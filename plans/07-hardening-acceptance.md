# Milestone 7: Hardening and Acceptance

## Goal

Close the rewrite by driving the full acceptance checklist to green and replacing example-only scaffolding with a maintainable product baseline.

## Primary references

- `docs/00-principles.md`
- `docs/100-implementation-plan.md`
- `docs/110-acceptance-checklist.md`

## Scope

- test coverage completion
- schema and docs synchronization
- example adapter refresh
- output polish and error-message review
- acceptance checklist audit

## Deliverables

- complete acceptance pass across all implemented milestones
- synced docs, schemas, and examples
- cleaned package boundaries and dead code removal
- stable integration test harness for future work

## Work breakdown

1. Audit the implementation against every checklist group and fill remaining gaps.
2. Refresh examples so they exercise current contracts rather than old scaffolding assumptions.
3. Align schemas with final implementation details and verify doc consistency.
4. Remove obsolete code paths from the pre-rewrite scaffold.
5. Improve command output where the spec requires clear operator guidance.
6. Capture residual risks and explicitly defer only what the roadmap already defers.

## Tests

- full unit and integration suite
- golden output coverage for command UX that the acceptance checklist depends on
- example-adapter end-to-end tests

## Out of scope

- roadmap items explicitly cut from scope
- deep roadmap items that are documented as deferred

## Exit criteria

- acceptance checklist is satisfied end to end
- examples, docs, and schemas describe the implementation that now exists
- no milestone depends on preserving the current example code structure
