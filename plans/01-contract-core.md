# Milestone 1: Contract Core

## Goal

Rebuild the core domain engine so `inspect --json` and `prepare` are authoritative, deterministic, and driven by the documented contracts rather than the current example implementation.

## Primary references

- `docs/40-cli-contract.md`
- `docs/50-adapter-schema.md`
- `docs/60-manifest-contract.md`
- `plans/acceptance-checklist.md`

## Scope

- adapter loading and validation
- adapter discovery from cwd, `--cwd`, and `--config`
- lane resolution and slug generation
- manifest derivation
- env projection for templates and compose env
- generated-file rendering
- sidecar hash tracking for generated outputs
- `inspect`
- `prepare`

## Deliverables

- pure packages for adapter, lane, manifest, env, and rendering concerns
- nearest-`devlane.yaml` resolution for `inspect` / `prepare` with clear missing-adapter UX
- `inspect --json` that always recomputes from adapter + current catalog snapshot and never reads `.devlane/manifest.json`
- `prepare` that writes the manifest, writes `.devlane/compose.env` when compose is declared, and renders generated outputs
- strict validation and deterministic JSON output
- generated output overwrite warnings and sidecar hash handling

## Work breakdown

1. Define internal domain types that map cleanly to the adapter schema and manifest contract.
2. Separate pure derivation logic from CLI and filesystem mutation logic.
3. Implement adapter resolution from explicit flags and nearest-parent walk-up from cwd.
4. Implement adapter validation to match documented schema-load failures.
5. Implement manifest building so top-level shape, `ready`, omitted fields, and template scope match the spec exactly.
6. Implement env projection from manifest + `runtime.env`.
7. Implement generated file rendering with repo-root safety checks and sidecar hash tracking.
8. Wire `inspect` and `prepare` to the new core, including the `devlane init` pointer when no adapter is found.

## Tests

- adapter validation table tests
- adapter resolution tests for explicit path, cwd walk-up, and missing-adapter cases
- manifest golden tests for stable, dev, bare-metal, containerized, and no-hostname cases
- env projection tests
- template scope tests
- generated file overwrite / sidecar hash tests
- CLI integration tests for `inspect` and `prepare`

## Out of scope

- `init`
- host catalog allocation
- host-level commands
- worktree lifecycle

## Exit criteria

- acceptance checklist sections: Core contract, Manifest shape, Generated outputs
- no command behavior depends on reading current example-generated files
