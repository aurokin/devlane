# Milestone 6: Worktree Lifecycle

## Goal

Let devlane create and retire lanes end to end through `git worktree` integration and explicit seed-file copying.

## Primary references

- `docs/40-cli-contract.md`
- `docs/50-adapter-schema.md`
- `docs/80-agent-playbook.md`
- `plans/phase-roadmap.md`
- `plans/acceptance-checklist.md`

## Scope

- `worktree create <lane>`
- `worktree remove <lane>`
- `worktree.seed`
- seed-file and seed-directory copying
- generated-output skip behavior
- scoped cleanup after removal

## Deliverables

- worktree creation flow: add checkout, copy seeds, run `prepare`
- worktree removal flow: remove checkout, run dedicated scoped catalog cleanup
- clear reporting of copied, skipped, and missing seed paths

## Work breakdown

1. Define the conventional path strategy for new worktrees.
2. Wrap `git worktree add` and `git worktree remove` with explicit error handling.
3. Implement seed copying for files and directories relative to repo root.
4. Skip seed entries that overlap generated destinations and report that clearly.
5. Warn and continue on missing seed sources.
6. Run `prepare` in the new worktree so catalog state is registered before use.
7. Run dedicated scoped cleanup after removal so only the removed worktree's `(app, lane, repoPath)` allocations are deleted.

## Tests

- worktree create integration tests
- seed file and directory copy tests
- overlap skip tests for generated destinations
- missing source warning tests
- worktree remove + dedicated scoped cleanup tests

## Out of scope

- `worktree list`
- automatic git config changes
- default seed inference

## Exit criteria

- acceptance checklist section: Worktree lifecycle
- agents can create a new lane without manual secret copying when the adapter declares `worktree.seed`
