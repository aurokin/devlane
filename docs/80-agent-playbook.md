# Agent playbook

This document explains how coding agents should use `devlane`.

## Default sequence

When asked to work inside a repo that uses `devlane`, agents should prefer this sequence:

1. `inspect --json`
2. check `manifest.ready`
3. `prepare` when `ready: false`, and also whenever the task depends on current generated outputs or `.devlane/compose.env`. `up` never promotes provisional ports for you
4. `up` or `status`

That order keeps discovery explicit and reproducible.

`inspect --json` always works — it recomputes the manifest from the adapter plus the current catalog and never reads stale state off disk. Before `prepare` has ever run, `manifest.ready` is `false` and ports appear with `allocated: false` plus a provisional `port` computed against the live catalog. That is the signal that `prepare` has not been run yet. Treat that number as "what `prepare` would pick right now," not as a committed reservation.

## Always read `inspect --json`, not the file on disk

`.devlane/manifest.json` on disk is a snapshot from the last `prepare`. It can drift if another process has run `reassign` or `host gc` since. `inspect --json` is always fresh.

Agents should call `inspect --json` every time they need authoritative information. The file on disk is for wrappers and humans who want a static file to eyeball.

See principle #3 in `docs/00-principles.md` and the "Freshness surfaces" section in `docs/60-manifest-contract.md`.

## What `ready` does (and doesn't) tell you

`manifest.ready: true` means **the catalog has entries for every declared port**. That is it.

It does not mean:

- the port is bindable right now (another process might be squatting)
- the lane is currently running
- the health endpoint responds

For "is the port actually bindable": `devlane port <svc> --probe`. For "is the lane running": `devlane status`. For health: hit `manifest.ports.<svc>.healthUrl` yourself.

If your task depends on generated files or `.devlane/compose.env`, do not use `ready` as a shortcut for "prepare definitely ran." Run `prepare`.

The same rule applies before `up`: if the adapter declares `ports`, do not rely on provisional `inspect` values. `up` does not commit them; `prepare` does.

For compose-backed adapters, `up` also verifies that the current prepare-owned compose inputs still match the live manifest/template state. If `.devlane/compose.env` or any declared `outputs.generated` file is stale or missing, `up` fails and points back at `prepare` instead of starting containers against mixed state.

## What `devlane status` tells you (and how bare-metal differs from compose)

`status` is read-only. What it can tell you depends on whether the lane has a supervisor:

- **Containerized and hybrid compose side**: runs `docker compose ps`. You get container state, health, uptime, ports — the supervisor knows.
- **Bare-metal and hybrid bare-metal side**: for allocated services, probes the reserved port and prints `bound` or `free`; for pre-prepare services (`allocated: false`), prints `unallocated` instead of probing the provisional candidate. Devlane does not know which process holds a bound port or whether it is your app.

A `bound` bare-metal service is not proof the app is running — only that *something* is listening on the port devlane reserved. An orphan process from a previous run, an unrelated app, or a stale container all look identical. Treat `bound` as "probably up"; use `healthUrl` to confirm when correctness matters. Treat `unallocated` as "run `prepare` first," not as a probe failure.

## What agents should not do first

Avoid leading with:

- guessing ports
- editing generated env files directly
- scraping random shell scripts
- reading `.devlane/manifest.json` directly when `inspect --json` is available
- assuming stable and dev use the same naming rules

## If a repo has no adapter yet

Agents should:

1. run `devlane init` to scaffold a starter `devlane.yaml`. Use `--from examples/<example>/devlane.yaml` if a specific example is a closer fit than auto-detection. Pass `--yes` if you do not have an interactive TTY.
2. read `docs/50-adapter-schema.md` and the relevant workflow doc (`70-container-workflow.md` for containerized, `75-baremetal-workflow.md` for bare-metal)
3. wire the repo's current generated files into `outputs.generated`
4. make `inspect --json` and `prepare` produce the current local state deterministically

### Monorepos

If `devlane init` detects multiple app candidates, it enters **monorepo** mode. Agents should:

1. run `devlane init --list` first to see the detected candidates and inferred kinds
2. decide whether to scaffold one adapter per app (`--all`) or target a specific subtree (`--app <path>`)
3. each resulting `devlane.yaml` is self-contained — the catalog keys off `(app, repoPath, service)`, so siblings in the same monorepo share a host catalog but not a manifest

Devlane does not ship a monorepo workspace file. If you need to enumerate or operate across all apps, loop over the discovered `devlane.yaml` paths yourself. `devlane host status` works across all of them automatically because the catalog is already host-scoped.

If `init` finds multiple candidates and you do not have a TTY, do **not** rely on `--yes` to pick for you. Use `--all` to scaffold every candidate or `--app <path>` to choose one subtree. Otherwise `init` fails after printing the candidate list.

## What to ask from a repo

A repo adopting `devlane` only needs to answer a small number of questions:

- what is the app identifier?
- is it `web`, `cli`, or `hybrid`?
- what does stable own?
- which files should be generated?
- which Compose files and profiles exist (if containerized)?
- for bare-metal, what commands start the services (optional, powers `devlane up`)?
- if the host has a proxy or DNS, what hostname pattern should lanes use (optional)?
- which files (credentials, `.env.local`, etc.) should be copied into a new worktree (optional, powers `devlane worktree create`)?

If you use `devlane init --from <path>`, treat the copied adapter as a draft. Relative paths and repo-specific identifiers are preserved literally; review `app`, `lane.host_patterns`, `compose_files`, `outputs.manifest_path`, `outputs.compose_env_path`, `outputs.generated[].template`, `outputs.generated[].destination`, and `worktree.seed` before assuming the target repo is correctly wired.

## Discovery: hostnames vs ports

Two discovery surfaces are first-class:

- **Hostname-based** — when the adapter declares `host_patterns` and the host has a proxy or DNS resolving them. `manifest.network.publicHost` and `publicUrl` are populated.
- **Port-based** — this becomes first-class in Phase 2 once host-catalog-backed `manifest.ports.<service>.port` lands. In Phase 1, discovery is usually via generated files plus the manifest's path and network fields.

Agents should check both. If `publicHost` is non-null, it is the preferred way to reach the app (stable across port changes). Once Phase 2 lands, otherwise use `manifest.ports.<service>.port` on localhost. Always read the manifest; never guess.

## Handling port conflicts

The manifest is the answer, not the agent's memory. An agent that has been running for a while may be acting on a stale port. Another process — another agent, the user, a hook — might have reassigned in the meantime.

When an agent encounters a port conflict, the order is:

1. **Re-read `inspect --json`.** Get a fresh manifest, including a fresh `ready` and fresh `ports.<service>.port`.
2. If the manifest value differs from what the agent was using, the agent was stale. Use the manifest value. Done.
3. If the manifest value matches what the agent was using, verify the port is actually free for us: `devlane port <service> --probe`. A non-zero exit confirms a real conflict.
4. Only then: `devlane reassign <service>`. This is idempotent and scoped — it will no-op if the conflict resolved itself, and it will only move the one service. Use `--force` only when you intentionally need to move an offline checkout aside, such as stable reclaiming its fixture from an unbound dev lane.
5. Re-read `inspect --json` for the new port and continue.

Reassign should be the last step, not the first. Most "conflicts" are staleness, and the stickiness guarantee of the host catalog is only valuable if agents avoid unnecessary reassigns.

Before calling `reassign`, check whether the process holding the port is actually yours. Orphan processes from a previous run look identical to external collisions from devlane's perspective.

When using `reassign --lane <name>`, prefer staying in the intended repo or passing `--config` / `--cwd` for it so the app context stays explicit. Lane names are selector metadata, not the durable identity key; a repo-less catalog lookup should only succeed when exactly one matching row exists across the host.

### When `prepare` fails with a collision error

If `prepare` itself fails with a stable-fixture collision (see `65-host-catalog.md`), the error message prints the exact resolution. Agents should parse the message and follow its instructions:

- **Stable vs another app's stable** — no automatic resolution. The agent should report the error to the user and not pick a winner. Which app moves is a human decision.
- **Stable vs offline dev lane** — the error includes a ready-to-paste `devlane reassign web --lane <name> --force && devlane prepare` command. The agent can run it.
- **Stable vs bound dev lane** — the dev lane must be stopped first. The agent should report the error and not attempt to kill foreign processes.

## Worktree lifecycle (Phase 3)

When Phase 3 lands, agents should prefer `devlane worktree create <lane>` over `git worktree add` directly. The devlane command does three things in one:

This applies only when the active adapter lives at the Git worktree root (`adapterRoot == repoRoot`). Subtree adapters in monorepos remain supported for in-place commands, but `worktree create` / `worktree remove` are out of scope for them and should fail clearly.

1. `git worktree add` at the conventional sibling path `<repo-root-parent>/<repo-root-base>-<lane-slug>`, creating a new branch named `<lane>` from the current `HEAD`
2. copy every path listed in the adapter's `worktree.seed` from the source checkout (credentials, `.env.local`, secret keys) — skipping any that are also in `outputs.generated`, because `prepare` will render those
3. run `prepare` in the new checkout so the catalog registers the new dev lane's ports

After `worktree create` returns, the new worktree has everything it needs to run: its own lane identity, its own allocated ports, its own generated files, and whatever seed files the adapter declared. No manual copying.

`devlane worktree remove <lane>` is the inverse: `git worktree remove` plus scoped catalog cleanup so the catalog does not accumulate stale entries. By default it resolves `<lane>` through the conventional sibling-path rule. If the worktree was manually moved or renamed, pass `--path <worktree>`; the command does not guess from lane metadata alone. "Scoped" means deleting only the removed worktree's `(app, repoPath)` allocations, not doing a host-wide sweep.

Changing branches in place inside an existing checkout is acceptable. For dev lanes, the checkout path is the durable identity anchor; branch, lane label, and mode are refreshed metadata. Agents should not treat an in-place branch switch as grounds to reassign or garbage-collect a lane by itself.

Agents should **not**:

- copy files between worktrees manually if `worktree.seed` covers them
- run `host gc` as a cleanup step after removing a worktree — `worktree remove` already does it
- propose a `worktree list` command — use `git worktree list` + `devlane host status`

If the adapter has no `worktree.seed` block, the agent should surface that as adoption debt. Most adapters need at least `.env.local` or credentials seeded. See `docs/50-adapter-schema.md` for the schema.

## `up` behavior by adapter shape

- **Containerized** (`compose_files` only): `devlane up` runs `docker compose up`. Devlane owns the command; compose owns supervision.
- **Bare-metal** (`runtime.run.commands` only): `devlane up` prints the rendered commands and exits. The agent (or user) runs them in a terminal.
- **Hybrid** (both): `devlane up` prints the bare-metal commands first, then runs `docker compose up`. If compose fails, the printed commands stay visible in the terminal.
- **Neither** (pure CLI or no lifecycle needs): `devlane up` is a no-op with a one-line hint.

Agents should not assume `up` always runs something. Read the adapter to know which shape applies, or just trust `up`'s output — it is always self-describing.

Agents should also not expect bare-metal run commands to be present in the manifest. That guidance lives in the adapter and `up` output, not in `inspect --json`.

## Good agent behavior

Good agents leave behind:

- updated docs
- updated example adapters
- updated schemas
- deterministic generated outputs
- explicit acceptance notes

The goal is not only to implement the change, but to preserve the next agent's ability to reason about it quickly.
