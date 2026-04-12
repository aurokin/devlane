# CLI contract

The shared tool should own **lifecycle**, not product-specific business logic.

## Lifecycle commands

- `init` — scaffold a starter `devlane.yaml`. Detects runtime pattern from repo signals (compose files → containerized; framework manifest without compose → bare-metal; neither → CLI). Flags: `--template <name>` uses a named starter template, `--from <path>` copies from any existing adapter, `--force` overwrites an existing file.
- `inspect` — derive and print the manifest
- `prepare` — write the manifest, compose env file, and generated files (allocates ports via the host catalog). If no `devlane.yaml` is found, points the user at `devlane init`.
- `up` — run lane-aware `docker compose up`
- `down` — run lane-aware `docker compose down` (does **not** release catalog ports)
- `status` — run lane-aware `docker compose ps`
- `doctor` — validate obvious prerequisites

## Host catalog commands

- `port <service>` — print the currently assigned port for a service (plain number by default, `--verbose` for metadata, `--probe` to verify bindability via exit code)
- `reassign <service>` — idempotent repair; probes the current port and only moves it if actually blocked, otherwise no-op
- `host status` — list all allocations across the host
- `host doctor` — probe every allocation and report live conflicts, missing repos, or other drift
- `host gc` — remove catalog entries whose repos or lanes no longer exist (supports `--older-than`, `--app`, `--yes`)

See `65-host-catalog.md` for the catalog contract and allocation model.

## Ownership boundaries

The shared tool owns:

- lane naming
- manifest generation
- path derivation
- compose project naming
- compose env generation
- template rendering
- common health and diagnostic output
- the host catalog and port allocation
- `~/.config/devlane/catalog.json` (state, tool-written)

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

Once the current phases are stable, these commands likely belong in the shared tool:

- `worktree create`
- `worktree list`
- `worktree remove`
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
