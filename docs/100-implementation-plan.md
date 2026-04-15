# Implementation plan

This scaffold is intentionally phased.

## Phase 1 — contract first

Goal: make the shared contract real before taking over every lifecycle detail.

Deliverables:

- stable adapter schema (including optional `host_patterns`, `runtime.run.commands`, per-port `health_path`)
- stable manifest schema (ports-as-objects with `allocated` flag and optional `healthUrl`, plus the top-level `ready` flag)
- working `inspect` that always recomputes from adapter + catalog, never reads manifest.json from disk
- working `prepare` with strict validation (Groups A/B from `110-acceptance-checklist.md`) and sidecar-hash detection for hand-edited generated files
- explicit failure semantics for `prepare` and the write half of `reassign`: validate everything that can fail before catalog work begins, compute catalog mutations under lock, perform repo-local writes against the unpublished in-memory result, and publish `catalog.json` only after those writes succeed. On failure, release the lock without publishing the mutation. `manifest.ready` must not imply a successful local write when outputs are stale or missing.
- `devlane init` for zero-friction adoption (deterministic lexical scan from cwd to depth 3; skip common non-app trees; no symlink traversal; `--template`, `--from`, `--app`, `--list`, `--yes`, `--all`, `--force`; fail rather than guess in non-interactive monorepo mode)
- lane-aware `up`, `down`, `status`, and `doctor`:
  - **Containerized** (`compose_files` declared): `up` runs `docker compose up`, `down` runs `docker compose down`, `status` runs `docker compose ps`. The compose substrate is the supervisor.
  - **Bare-metal** (`runtime.run.commands` declared): `up` prints the rendered commands and exits. `down` is a no-op. `status` prints the manifest-derived summary and probes declared ports as `bound` / `free`. Devlane never spawns bare processes.
  - **Hybrid** (both declared): `up` prints the bare-metal commands first, then runs compose. Exit code follows compose.
  - **Doctor**: read-only preflight for tool prerequisites and adapter sanity; no app-health claims, no process supervision, no catalog mutation.
- example adapters
- acceptance tests

## Phase 2 — host catalog and port allocation

Goal: let the shared tool coordinate across projects on the same host.

This phase is a prerequisite for worktree lifecycle automation because `worktree create` should register allocations into the catalog when a new lane is spun up.

Deliverables:

- host config at `~/.config/devlane/config.yaml` (user-owned)
- config schema + tests for `~/.config/devlane/config.yaml`, including malformed-config behavior and defaults when the file is absent
- host catalog at `~/.config/devlane/catalog.json` (tool-owned)
- concurrent-safe catalog writes: `fcntl.flock` on a sidecar lockfile + atomic `os.rename`, 30-second acquire timeout, POSIX-first (Windows deferred)
- adapter `ports` field
- manifest `ports` section with `allocated` flag, top-level `ready` flag, and `DEVLANE_PORT_*` env vars
- sticky allocation with probing only for first-time allocation and explicit repair / audit commands; existing allocations are never re-probed by `prepare`. Stable lanes treat `stable_port` as the fixture when declared, otherwise `default`, and fail loudly on collision (three scenarios documented in `65-host-catalog.md`)
- TCP probing on both `0.0.0.0` and `::` (IPv6 `V6ONLY=1`)
- `devlane port <service>` with `--verbose` and `--probe`
- `devlane reassign <service>` — idempotent, scoped, supports `--lane <name>` for same-app lane targeting; when repo context is unavailable and lookup falls back to the catalog, ambiguity across apps fails loudly
- `devlane host status`, `host doctor`, `host gc` (staleness = missing repoPath OR missing service declaration)
- repo-identity drift handling in host audits and cleanup: treat a row as drifted when the adapter currently loaded from `repoPath` no longer derives the same `(app, lane)` pair the catalog row claims, so stale allocations do not survive indefinitely after branch or repo-identity changes
- catalog schema at `schemas/catalog.schema.json`
- agent playbook section on conflict handling

## Phase 3 — worktree lifecycle (final phase)

Goal: let the shared tool create and retire lanes, not only operate inside them. This is the last planned phase.

Deliverables:

- `devlane worktree create <lane>` — `git worktree add` + seed copy + `prepare` in the new checkout. Uses the sibling path convention `<repo-root-parent>/<repo-root-base>-<lane-slug>`, creates a new branch named `<lane>` from the current `HEAD`, and fails rather than guessing if that branch or path already exists. Registers the dev lane's ports in the catalog before the user starts anything.
- `devlane worktree remove <lane>` — `git worktree remove` + dedicated scoped catalog cleanup so the catalog self-cleans. Scoped cleanup removes only the removed worktree's `(app, lane, repoPath)` allocations; it is not a host-wide `host gc`.
- adapter `worktree.seed` — explicit list of paths (files and directories) copied from the source checkout into a new worktree before `prepare`. No defaults. Paths that also appear in `outputs.generated` are skipped with a notice. Missing source files warn and continue. The full list of copied paths is printed on completion for security clarity.
- non-happy-path worktree semantics: define what is rolled back, what is left in place, and what remediation is printed when `git worktree add` succeeds but seed copy or `prepare` fails, or when `worktree remove` cannot complete both git removal and scoped catalog cleanup in one pass

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

## Design pressure to resist

The risk is turning the shared tool into an application framework. Non-negotiable #11 and `docs/00-principles.md` principle #6 exist to hold this line.

Keep the tool focused on:

- lane metadata
- lane lifecycle (create/remove, prepare, supervised up/down)
- orchestration of generated files
- host-wide coordination of ports and lanes

Do not let it become the place where product logic lives.
