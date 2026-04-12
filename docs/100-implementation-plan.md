# Implementation plan

This scaffold is intentionally phased.

## Phase 1 — contract first

Goal: make the shared contract real before taking over every lifecycle detail.

Deliverables:

- stable adapter schema (including optional `host_patterns`, `runtime.run`, per-port `health_path`)
- stable manifest schema (ports-as-objects with `allocated` flag and optional `healthUrl`)
- working `inspect` that always recomputes from adapter + catalog, never reads manifest.json from disk
- working `prepare` with strict validation (Groups A/B from `110-acceptance-checklist.md`) and sidecar-hash detection for hand-edited generated files
- `devlane init` for zero-friction adoption (cwd-based detection; `--template`, `--from`, `--yes`, `--force`)
- lane-aware `up`, `down`, and `status` (containerized runs compose; bare-metal `up` is no-op without `runtime.run`, otherwise suggests or executes commands; bare-metal `down` is always a no-op)
- example adapters
- acceptance tests

## Phase 2 — host catalog and port allocation

Goal: let the shared tool coordinate across projects on the same host.

This phase is a prerequisite for worktree lifecycle automation because `worktree create` should register allocations into the catalog when a new lane is spun up.

Deliverables:

- host config at `~/.config/devlane/config.yaml` (user-owned)
- host catalog at `~/.config/devlane/catalog.json` (tool-owned)
- concurrent-safe catalog writes: `fcntl.flock` on a sidecar lockfile + atomic `os.rename`, 30-second acquire timeout, POSIX-first (Windows deferred)
- adapter `ports` field
- manifest `ports` section with `allocated` flag and `DEVLANE_PORT_*` env vars
- sticky allocation with live probing during `prepare`; stable lanes treat declared `default` as a fixture and fail loudly on collision (three scenarios documented in `65-host-catalog.md`)
- TCP probing on both `0.0.0.0` and `::` (IPv6 `V6ONLY=1`)
- `devlane port <service>` with `--verbose` and `--probe`
- `devlane reassign <service>` — idempotent, scoped, supports `--lane <name>` to operate by lane
- `devlane host status`, `host doctor`, `host gc` (staleness = missing repoPath OR missing service declaration)
- catalog schema at `schemas/catalog.schema.json`
- agent playbook section on conflict handling

## Phase 3 — worktree lifecycle

Goal: let the shared tool create and retire lanes, not only operate inside them.

Candidate deliverables:

- `worktree create` (with catalog registration)
- `worktree list`
- `worktree remove` (with catalog cleanup)
- optional per-worktree Git config wiring

## Phase 4 — proxy integration

Goal: make hostname discovery first-class for containerized and bare-metal apps alike.

Candidate deliverables:

- proxy registration / unregistration
- lane DNS or local hostname helpers
- health-aware proxy routing
- stable lane cutover helpers

## Phase 5 — stable deploy policy

Goal: help repos formalize the boundary between dev lanes and stable ownership.

Candidate deliverables:

- stable deploy hooks
- stable rollback hooks
- global wrapper ownership helpers
- lane cutover docs and checks

## Deep roadmap

Further out, still worth capturing:

- UDP port allocation in the host catalog (currently TCP-only)
- Windows support for catalog concurrency (Phase 2 is POSIX-first via `fcntl.flock`; Windows needs `msvcrt.locking` equivalent)
- `devlane up --wait` with health-probe integration (Phase 1 emits `healthUrl` in the manifest but does not probe it)
- Smarter `init` auto-detection that senses proxy signals (Traefik labels, Caddyfile, etc.) — deferred; the current rule is "adapter declares `host_patterns` explicitly or it's absent"

## Design pressure to resist

The risk is turning the shared tool into an application framework.

Keep the tool focused on:

- lane metadata
- lifecycle
- orchestration
- generated files
- host-wide coordination of ports and lanes

Do not let it become the place where product logic lives.
