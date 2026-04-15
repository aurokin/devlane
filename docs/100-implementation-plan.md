# Implementation plan

This scaffold is intentionally phased.

## Phase 1 — contract first

Goal: make the shared manifest and lifecycle shape real before taking over host-wide coordination.

Deliverables:

- stable adapter schema (including optional `host_patterns`, `runtime.run.commands`, and the shape of future `ports` declarations)
- stable manifest schema (including the planned `ports` object shape with `allocated` / `healthUrl` and the top-level `ready` flag, even before host-catalog-backed allocations are authoritative)
- working `inspect` that always recomputes from adapter + catalog, never reads manifest.json from disk
- working `prepare` with strict validation for adapter load, template existence, destination containment, undefined template variables, and compose-file presence, plus sidecar-hash detection for hand-edited generated files
- explicit failure semantics for `prepare` and the write half of `reassign`: validate everything that can fail before catalog work begins, compute catalog mutations under lock, stage every repo-local write to a temp file in the destination directory, then promote staged files in deterministic order via atomic rename where possible. Publish `catalog.json` only after all required promotions succeed. On failure, release the lock without publishing the mutation and report which repo-local outputs were promoted and which remained untouched. Preserve the current meaning of `manifest.ready`: it remains an allocation-state signal only, not a claim that every repo-local write succeeded. Rerunning `prepare` remains the repair path for any partial repo-local update
- `devlane init` for zero-friction adoption (deterministic lexical scan from cwd to depth 3; skip common non-app trees; no symlink traversal; `--template`, `--from`, `--app`, `--list`, `--yes`, `--all`, `--force`; fail rather than guess in non-interactive monorepo mode)
- add an explicit `hybrid-web` starter template. Auto-detection remains conservative: `init` detects containerized, bare-metal, or CLI from filesystem signals, but it does not silently infer hybrid mode from overlapping signals. When the repo looks mixed, `init` should say so and point at `--template hybrid-web`
- `devlane init --from <path>` copies an existing adapter as a literal starting point. It does not re-root or rewrite relative paths, does not silently change `app` or hostname patterns, and prints a review checklist for source-relative fields (`compose_files`, template paths, `worktree.seed`, and similar repo-coupled settings) that may not make sense in the target repo
- lane-aware `up`, `down`, `status`, and `doctor`:
  - **Containerized** (`compose_files` declared): `up` runs `docker compose up`, `down` runs `docker compose down`, `status` runs `docker compose ps`. The compose substrate is the supervisor.
  - **Bare-metal** (`runtime.run.commands` declared): `up` prints the rendered commands and exits. `down` is a no-op. `status` prints the manifest-derived summary and reports declared ports as `bound`, `free`, or `unallocated`; provisional pre-prepare ports are never probed. Devlane never spawns bare processes.
  - **Hybrid** (both declared): `up` prints the bare-metal commands first, then runs compose. Exit code follows compose. `status` reports compose `ps` output plus `bound` / `free` / `unallocated` results for every declared host port; it does not try to infer which declared port "belongs" to which substrate unless the adapter grows that metadata later
  - **Doctor**: read-only preflight for tool prerequisites and adapter sanity; no app-health claims, no process supervision, no catalog mutation.
- example adapters
- acceptance tests

### Phase 1 is done when

- `init`, `inspect`, `prepare`, `up`, `down`, `status`, and `doctor` all have explicit behavior documented and tested for containerized, bare-metal, hybrid, and no-lifecycle adapters
- pre-prepare `inspect --json` and pre-prepare `status` are both unambiguous: provisional ports are surfaced as provisional, and unallocated services are not probed as though they were reserved
- the template / bare-metal-command variable scope is described once and repeated consistently across the adapter, manifest, and workflow docs
- the acceptance checklist can point to this phase without relying on unnamed checklist "groups"

## Phase 2 — host catalog and port allocation

Goal: make the host-scoped coordination layer authoritative across projects on the same machine.

This phase is a prerequisite for worktree lifecycle automation because `worktree create` should register allocations into the catalog when a new lane is spun up.

Deliverables:

- host config at `~/.config/devlane/config.yaml` (user-owned)
- config schema + tests for `~/.config/devlane/config.yaml`, including malformed-config behavior and explicit defaults when the file is absent: `port_range: 3000-9999`, `reserved: [22, 80, 443, 5432, 6379]`
- host catalog at `~/.config/devlane/catalog.json` (tool-owned)
- concurrent-safe catalog writes: `fcntl.flock` on a sidecar lockfile + atomic `os.rename`, 30-second acquire timeout, POSIX-first (Windows deferred)
- adapter `ports` field
- manifest `ports` section with `allocated` flag, top-level `ready` flag, and `DEVLANE_PORT_*` env vars
- sticky allocation with probing only for first-time allocation and explicit repair / audit commands; existing allocations are never re-probed by `prepare`. Stable lanes treat `stable_port` as the fixture when declared, otherwise `default`, and fail loudly on collision (three scenarios documented in `65-host-catalog.md`)
- catalog identity model: `(app, repoPath, service)` is the durable key; `mode`, `lane`, and `branch` are refreshed metadata. In-place branch switching updates metadata and is not drift by itself
- deterministic multi-service allocation: declaration-order walk with in-memory reservation during both `prepare` and provisional `inspect`
- TCP probing on both `0.0.0.0` and `::` (IPv6 `V6ONLY=1`)
- `devlane port <service>` with `--verbose` and `--probe`. The command remains about assigned ports, not provisional candidates: before first `prepare` for that service it fails clearly and points callers at `inspect --json` (for the current provisional candidate) or `prepare` (to commit one)
- `devlane reassign <service>` — idempotent, scoped, supports `--force` for intentional displacement of an offline lane and `--lane <name>` as a metadata selector; any multi-match on that selector fails loudly
- `devlane host status`, `host doctor`, `host gc`:
  `host doctor` is primarily a stale-entry and duplicate-claim audit. It reports probe results (`bound` / `free`) for operator context but does not treat a singly claimed bound port as an error by itself, because host-wide probing cannot prove process ownership for bare-metal lanes
  staleness = missing repoPath OR missing service declaration OR app mismatch at repoPath
  duplicate claims (multiple catalog rows for one port) are explicit failures
  `host gc` in non-interactive mode fails unless `--yes` or `--dry-run` is provided
- repo-identity drift handling in host audits and cleanup: treat a row as drifted when the adapter currently loaded from `repoPath` no longer identifies the same app. Branch or lane-label changes at that checkout are metadata refreshes, not drift
- collision remediation docs that distinguish runtime shape: recipes may tell compose-backed lanes to use `devlane down`, but pure bare-metal lanes must be told to stop their own processes outside devlane before `reassign` / `prepare`
- catalog schema at `schemas/catalog.schema.json`
- agent playbook section on conflict handling

### Phase 2 is done when

- the catalog is the sole durable authority for assigned ports, while `inspect --json` remains the fresh read surface and `.devlane/manifest.json` remains only a snapshot
- collision handling, `reassign --lane`, and `host doctor` / `host gc` all have one ambiguity rule and one stale-entry rule shared across docs, tests, and user-facing messages
- host-wide coordination is strict about stable fixtures and explicit about what it can and cannot infer from probes

## Phase 3 — worktree lifecycle (final phase)

Goal: let the shared tool create and retire lanes, not only operate inside them. This is the last planned phase.

Deliverables:

- `devlane worktree create <lane>` — `git worktree add` + seed copy + `prepare` in the new checkout. Uses the sibling path convention `<repo-root-parent>/<repo-root-base>-<lane-slug>`, creates a new branch named raw `<lane>` from the current `HEAD`, fails rather than guessing if that branch or path already exists, requires `<lane>` to be a valid new local Git branch name, requires `<lane>` to slugify to a non-empty `<lane-slug>`, rejects `<lane>` equal to the adapter's `stable_name`, and rejects distinct raw lane names that would collide on the same `<lane-slug>`. Registers the new dev lane's ports in the catalog before the user starts anything.
- `devlane worktree remove <lane>` — `git worktree remove` + dedicated scoped catalog cleanup so the catalog self-cleans. By default `<lane>` resolves to the conventional sibling path `<repo-root-parent>/<repo-root-base>-<lane-slug>`. If that path is missing or the user has renamed the worktree manually, the command fails rather than guessing and requires `--path <worktree>` for an explicit target. Scoped cleanup removes only the removed worktree's `(app, repoPath)` allocations; it is not a host-wide `host gc`. Capture `app` and `repoPath` from the target checkout before removal so cleanup still has a stable key after the directory is gone.
- adapter `worktree.seed` — explicit list of paths (files and directories) copied from the source checkout into a new worktree before `prepare`. No defaults. Paths that also appear in `outputs.generated` are skipped with a notice. Missing source files warn and continue. Symlinks are recreated as symlinks (never dereferenced or rewritten). Regular-file mode bits are preserved best-effort; ownership is not. Existing destination paths are overwritten from the source checkout for explicit seed entries. The full list of copied paths is printed on completion for security clarity.
- non-happy-path worktree semantics: define what is rolled back, what is left in place, and what remediation is printed when `git worktree add` succeeds but seed copy or `prepare` fails, or when `worktree remove` cannot complete both git removal and scoped catalog cleanup in one pass

### Phase 3 is done when

- lane validation rules are explicit enough that branch naming, slug generation, and sibling-path collisions are deterministic before any git mutation happens
- `worktree create` and `worktree remove` have a documented partial-failure matrix that matches implementation and tests
- the command output always leaves the user with one deterministic next step rather than silent partial state

Explicitly **not** in this phase:

- `worktree list` (redundant with `git worktree list` + `devlane host status`)
- per-worktree git config wiring (users configure their own git)
- any default seed list, glob-based seeding, or magic credential discovery

## Cut from the roadmap

The following phases were considered and cut. They will not be re-opened without a revision to `docs/00-principles.md`:

- **Proxy integration.** Devlane emits `publicHost` / `publicUrl` in the manifest when `host_patterns` is declared. The user's proxy config (Caddyfile, Traefik labels, `/etc/hosts`) reads those values. Devlane does not talk to the proxy directly — coupling devlane to every proxy's API is the opposite of keeping the core repo-agnostic.
- **Stable deploy policy.** Deploy hooks, rollback hooks, global wrapper installers, and lane cutover helpers are per-product concerns. They belong in each repo's deploy scripts, not in a lane-metadata tool. See non-negotiable #11.

## Deep roadmap (not yet scheduled)

Further out, still worth capturing:

- UDP port allocation in the host catalog (currently TCP-only)
- Windows support for catalog concurrency (Phase 2 is POSIX-first via `fcntl.flock`; Windows needs `msvcrt.locking` equivalent)
- `devlane up --wait` with health-probe integration (Phase 1 emits `healthUrl` in the manifest but does not probe it)
- Smarter `init` assistance around proxy signals (Traefik labels, Caddyfile, etc.) — deferred, and limited to suggestion-only scaffolding. The adapter still declares `host_patterns` explicitly; detection must never silently infer hostname ownership into the contract.

Phase 1 `init` should stay deterministic even before that future work lands: scan descendants in lexical order, skip common non-app trees (`.git/`, `.devlane/`, `.direnv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, `tmp/`), do not follow symlinks, and fail rather than guessing when monorepo mode meets a non-interactive caller without `--all` or `--app`.

When exactly one candidate is found below `cwd`, `init` should scaffold in that subtree and print the selected path explicitly. Monorepo mode is reserved for multiple candidates, not for the single-descendant case.

## Design pressure to resist

The risk is turning the shared tool into an application framework. Non-negotiable #11 and `docs/00-principles.md` principle #6 exist to hold this line.

Keep the tool focused on:

- lane metadata
- lane lifecycle (create/remove, prepare, supervised up/down)
- orchestration of generated files
- host-wide coordination of ports and lanes

Do not let it become the place where product logic lives.
