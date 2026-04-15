# Milestone 2: Init

## Goal

Ship `devlane init` as the zero-friction adoption entry point for new repos, including monorepo scanning and starter template generation.

## Primary references

- `docs/40-cli-contract.md`
- `docs/50-adapter-schema.md`
- `docs/100-implementation-plan.md`
- `docs/110-acceptance-checklist.md`

## Scope

- repo scanning from cwd to depth 3
- candidate detection and runtime classification
- single-candidate, monorepo, and ambiguous flows
- starter templates
- `--template`, `--from`, `--app`, `--list`, `--yes`, `--all`, `--force`

## Deliverables

- `init` command with detection reasoning printed to the user
- starter templates for `containerized-web`, `baremetal-web`, and `cli`
- commented scaffold blocks for `host_patterns` and `worktree.seed`
- non-interactive safe behavior for CI and agent contexts

## Work breakdown

1. Build a scanner that finds likely app roots and records the signal used for detection.
2. Implement runtime classification rules from the CLI contract.
3. Create starter template assets that already satisfy schema validation.
4. Implement copy-from-existing-adapter flow for `--from`.
5. Implement single-app and monorepo write flows, including overwrite protection.
6. Make prompts conditional on TTY presence and bypassable with `--yes` or `--all`.

## Tests

- detection tests for compose, bare-metal, CLI, and ambiguous repos
- monorepo scan tests
- template output tests
- overwrite protection tests
- non-interactive tests for `--yes` / non-TTY cases

## Out of scope

- host catalog behavior
- worktree lifecycle

## Exit criteria

- acceptance checklist section: Init
- generated starter adapters validate and align with the documented schema
