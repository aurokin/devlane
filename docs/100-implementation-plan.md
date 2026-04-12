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

## Phase 2 — worktree lifecycle

Goal: let the shared tool create and retire lanes, not only operate inside them.

Candidate deliverables:

- `worktree create`
- `worktree list`
- `worktree remove`
- `gc`
- optional per-worktree Git config wiring

## Phase 3 — proxy integration

Goal: make hostname discovery first-class.

Candidate deliverables:

- proxy registration / unregistration
- lane DNS or local hostname helpers
- health-aware proxy routing
- stable lane cutover helpers

## Phase 4 — stable deploy policy

Goal: help repos formalize the boundary between dev lanes and stable ownership.

Candidate deliverables:

- stable deploy hooks
- stable rollback hooks
- global wrapper ownership helpers
- lane cutover docs and checks

## Design pressure to resist

The risk is turning the shared tool into an application framework.

Keep the tool focused on:

- lane metadata
- lifecycle
- orchestration
- generated files

Do not let it become the place where product logic lives.
