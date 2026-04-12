# CLI contract

The shared tool should own **lifecycle**, not product-specific business logic.

## Current commands in this scaffold

- `inspect` — derive and print the manifest
- `prepare` — write the manifest, compose env file, and generated files
- `up` — run lane-aware `docker compose up`
- `down` — run lane-aware `docker compose down`
- `status` — run lane-aware `docker compose ps`
- `doctor` — validate obvious prerequisites

## Ownership boundaries

The shared tool owns:

- lane naming
- manifest generation
- path derivation
- compose project naming
- compose env generation
- template rendering
- common health and diagnostic output

The repo adapter owns:

- which files are generated
- which Compose files exist
- which profiles are default
- how repo-specific env/config files map from the manifest

The repo itself owns:

- application code
- service definitions
- product-specific wrapper semantics
- stable deployment policy

## Future commands that belong here

Once phase 1 is stable, these commands likely belong in the shared tool:

- `worktree create`
- `worktree list`
- `worktree remove`
- `gc`
- `stable deploy`
- `stable rollback`
- `proxy register`
- `proxy unregister`

## Why `inspect --json` matters

`inspect --json` is the contract that lets agents avoid repo-specific heuristics.

Example uses:

- a coding agent finds the public URL without guessing a port
- a shell wrapper reads the state root without re-deriving it
- a proxy integration learns the lane hostname and project name
- a test harness discovers generated output paths
