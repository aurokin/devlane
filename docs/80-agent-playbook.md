# Agent playbook

This document explains how coding agents should use `devlane`.

## Default sequence

When asked to work inside a repo that uses `devlane`, prefer this sequence:

1. `inspect --json`
2. check `manifest.ready`
3. run `prepare` when `ready: false`, and also whenever the task depends on current generated outputs or `.devlane/compose.env`
4. use `up` or `status`

That order keeps discovery explicit and reproducible.

`inspect --json` always works. It recomputes the manifest from the adapter plus the current catalog and never reads stale state from disk. Before `prepare` has ever run, `manifest.ready` is `false` and ports appear with `allocated: false` plus a provisional `port` computed against the live catalog.

## Always read `inspect --json`, not the file on disk

`.devlane/manifest.json` on disk is only a snapshot from the last `prepare`. Agents should call `inspect --json` every time they need authoritative information.

## What `ready` does and does not tell you

`manifest.ready: true` means the catalog has entries for every declared port. It does not mean:

- the port is bindable right now
- the lane is currently running
- the health endpoint responds

If your task depends on generated files or `.devlane/compose.env`, do not use `ready` as a shortcut for "prepare definitely ran." Run `prepare`.

## What `devlane status` tells you

`status` is read-only. What it can tell you depends on whether the lane has a supervisor:

- **Containerized and hybrid compose side**: runs `docker compose ps`
- **Bare-metal and hybrid bare-metal side**: for allocated services, probes the reserved port and prints `bound` or `free`; for pre-prepare services, prints `unallocated`

A `bound` bare-metal service is not proof that the app is running. It only means that something is listening on the reserved port.

## What agents should not do first

Avoid leading with:

- guessing ports
- editing generated env files directly
- scraping random shell scripts
- reading `.devlane/manifest.json` directly when `inspect --json` is available
- assuming stable and dev use the same naming rules

## If a repo has no adapter yet

Agents should:

1. run `devlane init` to scaffold a starter `devlane.yaml`
2. read `docs/50-adapter-schema.md` and the relevant workflow doc
3. wire the repo's current generated files into `outputs.generated`
4. make `inspect --json` and `prepare` produce the current local state deterministically

### Monorepos

If `devlane init` detects multiple app candidates:

1. run `devlane init --list` first
2. decide whether to scaffold one adapter per app (`--all`) or target a specific subtree (`--app <path>`)
3. treat each resulting `devlane.yaml` as self-contained

If `init` finds multiple candidates and you do not have a TTY, do not rely on `--yes` to pick for you. Use `--all` or `--app <path>`. Otherwise `init` fails after printing the candidate list.

## What to ask from a repo

A repo adopting `devlane` only needs to answer:

- what is the app identifier?
- is it `web`, `cli`, or `hybrid`?
- what does stable own?
- which files should be generated?
- which Compose files and profiles exist, if containerized?
- for bare-metal, what commands start the services, if any?
- if the host has a proxy or DNS, what hostname pattern should lanes use?
- if the repo wants future worktree automation, which files should be declared in `worktree.seed`?

If you use `devlane init --from <path>`, treat the copied adapter as a draft. Relative paths and repo-specific identifiers are preserved literally.

## Discovery: hostnames vs ports

Two discovery surfaces are first-class:

- **Hostname-based** when the adapter declares `host_patterns`
- **Port-based** through `manifest.ports.<service>.port`

If `publicHost` is non-null, prefer it. Otherwise use localhost plus the manifest port. Always read the manifest; never guess.

## Handling port conflicts

When an agent encounters a port conflict, the order is:

1. re-read `inspect --json`
2. if the manifest value changed, use the manifest value
3. if it did not change, verify the current state with `devlane status` or an OS-level probe
4. if the port is still blocked, stop and surface the conflict
5. re-read `inspect --json` after the external conflict is resolved

There is no shipped `devlane reassign` command yet. Do not hand-edit the catalog as part of normal agent behavior.

### When `prepare` fails with a collision error

If `prepare` fails with a stable-fixture collision:

- **stable vs another app's stable**: report the error to the user; which adapter moves is a human decision
- **stable vs offline dev lane**: surface the conflict and stop; the repair workflow is still manual today
- **stable vs bound dev lane**: the conflicting process must be stopped first; do not attempt to kill foreign processes

## Worktrees today

`devlane` does not ship worktree lifecycle commands yet. Use ordinary `git worktree` flows when needed, then run `inspect` / `prepare` inside the resulting checkout.

If a repo wants future worktree automation, `worktree.seed` is the place to declare which credentials or local env files should move with a new worktree. No shipped CLI command consumes that field today.

## `up` behavior by adapter shape

- **Containerized** (`compose_files` only): `devlane up` runs `docker compose up`
- **Bare-metal** (`runtime.run.commands` only): `devlane up` prints the rendered commands and exits
- **Hybrid** (both): `devlane up` prints the bare-metal commands first, then runs `docker compose up`
- **Neither**: `devlane up` is a no-op with a one-line hint

Agents should not assume `up` always runs something. Read the adapter or trust `up`'s output.

## Good agent behavior

Good agents leave behind:

- updated docs
- updated example adapters
- updated schemas
- deterministic generated outputs
- explicit acceptance notes when behavior changes
