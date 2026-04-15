# Milestone 4: Host Catalog Core

## Goal

Build the host-wide coordination subsystem that makes sticky port allocation real across repos, lanes, and runtime patterns.

## Primary references

- `docs/60-manifest-contract.md`
- `docs/65-host-catalog.md`
- `docs/80-agent-playbook.md`
- `docs/110-acceptance-checklist.md`

## Scope

- host config load from `~/.config/devlane/config.yaml`
- host catalog load/store from `~/.config/devlane/catalog.json`
- lockfile + atomic rename write discipline
- TCP probing on IPv4 and IPv6
- stable fixture handling
- dev-lane pool allocation
- `prepare` integration with catalog-backed allocation

## Deliverables

- catalog schema-aligned persistence layer
- allocation engine that preserves existing allocations and only probes when allocating
- merged reserved-port handling
- collision detection and user-facing stable error messages
- manifest population from catalog state, including `allocated` and top-level `ready`

## Work breakdown

1. Implement host config defaults and parser.
2. Implement catalog model and storage layer with lock acquisition and atomic writes.
3. Implement probing utilities for `0.0.0.0` and `::` with `V6ONLY=1`.
4. Implement allocation rules for existing entries, stable fixtures, dev defaults, `pool_hint`, and global pool fallback.
5. Implement stable collision scenarios exactly as documented.
6. Integrate catalog-backed allocation into `prepare`.
7. Update manifest derivation so fresh `inspect` reflects current catalog state.

## Tests

- catalog read/write tests
- lock discipline tests where practical
- allocation tests for stable, dev, reserved ports, `pool_hint`, and exhaustion
- collision scenario tests
- manifest readiness tests backed by catalog state
- integration tests for `prepare` creating and reusing allocations

## Out of scope

- user-facing `port`, `reassign`, and `host *` commands
- worktree lifecycle

## Exit criteria

- acceptance checklist section: Host catalog, excluding the host-facing command entrypoints delivered next
- `inspect --json` and `prepare` are backed by real host state rather than example-only defaults
