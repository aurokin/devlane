# CLI contract

The shared tool should own **lifecycle**, not product-specific business logic.

## Lifecycle commands

- `init` — scaffold a starter `devlane.yaml`. Scans for app roots (cwd and up to depth 3 below) and detects runtime pattern from signals at each candidate: `compose*.yaml` → containerized; `package.json` / `pyproject.toml` / `Cargo.toml` / `go.mod` / `Gemfile` / `*.csproj` without compose → bare-metal; neither → CLI. Outcomes:
  - **single** — one candidate (at cwd). Scaffold in place. Today's default path.
  - **monorepo** — multiple candidates. Print the list with inferred kind per candidate, prompt the user to pick one or all, and scaffold `devlane.yaml` in each chosen subtree. `--all` skips the prompt.
  - **ambiguous** — no confident signal. Scaffold a CLI template and print a notice pointing at `--template baremetal-web` or `--template containerized-web`.

  Flags: `--template <name>` uses a named starter template (`containerized-web`, `baremetal-web`, `cli`), `--from <path>` copies from any existing adapter, `--app <path>` targets a specific subtree and skips scanning, `--list` prints detected candidates without writing anything, `--yes` / `--all` skip interactive prompts (also skipped when stdin is not a TTY), `--force` overwrites an existing file.
- `inspect` — derive and print the manifest. Always recomputes from the adapter and the current catalog; never reads `.devlane/manifest.json` off disk. Works before `prepare` has ever run (emits `allocated: false` for unallocated ports).
- `prepare` — write the manifest, render generated files, and allocate ports via the host catalog. If no `devlane.yaml` is found, points the user at `devlane init`. If the compose pattern is in use, also writes `.devlane/compose.env`.
- `up` — start the lane.
  - **Containerized** (adapter declares `runtime.compose_files`): runs lane-aware `docker compose up`.
  - **Bare-metal with `runtime.run` declared**: prints rendered commands (`mode: suggest`, default) or runs them fire-and-forget (`mode: execute`).
  - **Bare-metal without `runtime.run`**: no-op. Prints a one-line hint pointing at `docs/75-baremetal-workflow.md`.
- `down` — stop the lane.
  - **Containerized**: runs lane-aware `docker compose down`. Does **not** release catalog ports.
  - **Bare-metal**: no-op. Devlane does not manage bare-metal processes.
- `status` — print lane state without mutating anything. For containerized, runs `docker compose ps`. For bare-metal, prints the manifest-derived summary.
- `doctor` — validate obvious prerequisites.

## Host catalog commands

- `port <service>` — print the currently assigned port for a service. Plain number by default; `--verbose` for metadata; `--probe` to verify bindability via exit code.
- `reassign <service>` — idempotent repair. Probes the current port and only moves it if actually blocked, otherwise no-op. `--lane <name>` operates on a specific lane by name without requiring a cd (the catalog has enough to find the right repo).
- `host status` — list all allocations across the host.
- `host doctor` — probe every allocation and report live conflicts, missing repos, or other drift.
- `host gc` — remove catalog entries whose repos or services no longer exist. Staleness = `repoPath` missing OR adapter no longer declares the service. Supports `--app`, `--dry-run`, `--yes`.

See `65-host-catalog.md` for the catalog contract, allocation model, and fixture semantics for stable lanes.

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
- bare-metal run commands (optional `runtime.run`)

The repo itself owns:

- application code
- service definitions
- product-specific wrapper semantics
- stable deployment policy
- bare-metal process supervision (devlane does not manage processes)

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

Because `inspect` recomputes from the adapter plus the current catalog, the command is always safe to run — it does not mutate state, and it works before `prepare` has ever run.
