# Milestone 3: Lifecycle Commands

## Goal

Finish runtime command behavior for containerized, bare-metal, and hybrid adapters while keeping the supervised-substrate rule intact.

## Primary references

- `docs/40-cli-contract.md`
- `docs/70-container-workflow.md`
- `docs/75-baremetal-workflow.md`
- `docs/110-acceptance-checklist.md`

## Scope

- `up`
- `down`
- `status`
- `doctor`
- compose command construction
- bare-metal command rendering
- hybrid output sequencing

## Deliverables

- `up` behavior that differs correctly by adapter shape
- `down` behavior that is no-op for bare-metal and compose-backed for containerized/hybrid
- `status` behavior that uses compose `ps` where available and port-bound evidence for bare-metal services
- `doctor` with basic prerequisite checks that do not overreach into app framework behavior

## Work breakdown

1. Refactor lifecycle command routing so adapter shape drives behavior instead of ad hoc branching.
2. Make compose command generation deterministic and lane-aware.
3. Implement bare-metal run command rendering using the same template scope as generated outputs.
4. Implement hybrid sequencing: print bare-metal commands first, then run compose.
5. Implement bare-metal `status` probing output as `bound` / `free` only.
6. Keep `doctor` limited to tool prerequisites and adapter sanity.

## Tests

- compose command generation tests
- lifecycle tests for containerized, bare-metal, and hybrid adapters
- dry-run tests
- bare-metal status probe output tests
- hybrid failure-path tests that preserve printed run commands

## Out of scope

- host catalog mutation beyond what `prepare` already needs later
- worktree commands

## Exit criteria

- acceptance checklist sections: Compose lifecycle, Bare-metal lifecycle, Hybrid lifecycle
- no command spawns unsupervised bare-metal processes
