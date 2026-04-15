# Milestone 5: Host Catalog Commands

## Goal

Expose the host catalog subsystem through the operational commands agents and humans need to inspect, repair, and clean host-wide state.

## Primary references

- `docs/40-cli-contract.md`
- `docs/65-host-catalog.md`
- `docs/80-agent-playbook.md`
- `docs/110-acceptance-checklist.md`

## Scope

- `port <service>`
- `reassign <service>`
- `host status`
- `host doctor`
- `host gc`

## Deliverables

- plain-number and verbose `port` output
- `--probe` support with exit-code semantics
- idempotent `reassign` with `--lane` and explicit app-context preservation rules
- host-wide status and read-only audit commands
- explicit, confirmation-driven GC for stale allocations with `--app`, `--dry-run`, and `--yes`

## Work breakdown

1. Implement lookup logic for current lane context and explicit `--lane` targeting, including repo-less ambiguity failures across apps.
2. Build `port` around catalog reads and probing helpers.
3. Build `reassign` on top of the allocator and write-half-of-prepare flow.
4. Implement `host status` as a host-wide allocation view.
5. Implement `host doctor` as a read-only audit of bindability conflicts, missing repos, and adapter drift, with non-zero exit semantics.
6. Implement `host gc` using the documented staleness rules, `--app` scoping, and confirmation flow.

## Tests

- `port` output and exit-code tests
- `reassign` idempotency, single-service scope, and `--lane` ambiguity tests
- `host status` listing tests
- `host doctor` drift and conflict tests
- `host gc` app-scope, dry-run, and confirmation tests

## Out of scope

- worktree lifecycle
- any automatic background cleanup

## Exit criteria

- full acceptance checklist coverage for host-facing commands
- agent conflict-handling flow from the playbook works end to end
