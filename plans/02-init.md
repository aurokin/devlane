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
- deterministic scan behavior, `--list`, and no-guess non-interactive monorepo behavior
- non-interactive safe behavior for CI and agent contexts

## Work breakdown

1. Build a scanner that walks cwd to depth 3 in lexical order, skips the documented non-app trees, and never follows symlinks.
2. Implement candidate detection and runtime classification rules from the CLI contract, including ambiguous-to-CLI fallback.
3. Create starter template assets that already satisfy schema validation.
4. Implement copy-from-existing-adapter flow for `--from`.
5. Implement `--list`, single-app, and monorepo write flows, including overwrite protection.
6. Make prompts conditional on TTY presence and bypassable with `--yes` or `--all`, while failing rather than guessing in non-interactive monorepo mode unless `--all` or `--app` is provided.

## Tests

- detection tests for compose, bare-metal, CLI, and ambiguous repos
- monorepo scan tests, including lexical ordering, skipped trees, and symlink avoidance
- `--list` no-write tests
- template output tests
- overwrite protection tests
- non-interactive tests for `--yes` / non-TTY cases and no-guess failure paths

## Out of scope

- host catalog behavior
- worktree lifecycle

## Exit criteria

- acceptance checklist section: Init
- generated starter adapters validate and align with the documented schema
