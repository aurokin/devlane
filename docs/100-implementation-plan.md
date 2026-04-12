# Implementation plan

This scaffold is intentionally phased.

## Phase 1 — contract first

Goal: make the shared contract real before taking over every lifecycle detail.

Deliverables:

- stable adapter schema
- stable manifest schema
- working `inspect`
- working `prepare`
- lane-aware `up`, `down`, and `status`
- example adapters
- acceptance tests

## Phase 2 — host catalog and port allocation

Goal: let the shared tool coordinate across projects on the same host.

This phase is a prerequisite for worktree lifecycle automation because `worktree create` should register allocations into the catalog when a new lane is spun up.

Deliverables:

- host config at `~/.config/devlane/config.yaml` (user-owned)
- host catalog at `~/.config/devlane/catalog.json` (tool-owned)
- adapter `ports` field
- manifest `ports` section and `DEVLANE_PORT_*` env vars
- sticky allocation with live probing during `prepare`
- `devlane port <service>` with `--verbose` and `--probe`
- `devlane reassign <service>` — idempotent, scoped
- `devlane host status`, `host doctor`, `host gc`
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

## Design pressure to resist

The risk is turning the shared tool into an application framework.

Keep the tool focused on:

- lane metadata
- lifecycle
- orchestration
- generated files
- host-wide coordination of ports and lanes

Do not let it become the place where product logic lives.
