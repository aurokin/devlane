# Prompt: extend the devlane core

You are working on the shared `devlane` tool.

## Read first

1. `AGENTS.md`
2. `README.md`
3. `docs/README.md`
4. `docs/40-cli-contract.md`
5. `docs/50-adapter-schema.md`
6. `docs/60-manifest-contract.md`
7. `docs/100-implementation-plan.md`
8. `docs/110-acceptance-checklist.md`

## Goal

Implement the next feature without breaking the lane contract or making the core repo-specific.

## Rules

- treat the manifest as contract surface
- update docs, schemas, examples, and tests together
- do not push product-specific naming into the core
- prefer explicit behavior and deterministic output

## Deliverables

- implementation
- docs updates
- schema updates if the contract changed
- example updates if relevant
- tests

## Preferred order

1. change the contract
2. update docs and schema
3. implement
4. update examples
5. update tests
