# CLI contract

The shared tool owns **lane lifecycle and machine-readable state**, not product-specific business logic. It mutates only the state it owns, reads process state it can observe safely, and refuses to fire-and-forget unsupervised user processes.

## Shipped surface

The current CLI is:

- `init`
- `inspect`
- `prepare`
- `up`
- `down`
- `status`
- `doctor`

Not shipped:

- `port`
- `reassign`
- `host status`
- `host doctor`
- `host gc`
- `worktree create`
- `worktree remove`

Planning detail for those commands lives in `../plans/phase-roadmap.md`, not in this contract doc.

## Lifecycle commands

- `init` — scaffold a starter `devlane.yaml`. It scans for app roots (cwd and up to depth 3 below) and detects runtime pattern from signals at each candidate: `compose*.yml|yaml` or `docker-compose*.yml|yaml` -> containerized; `package.json` / `Cargo.toml` / `go.mod` / `Gemfile` / `*.csproj` without compose -> bare-metal; neither -> CLI. The scan walks descendants in lexical order, does not follow symlinks, skips nested Git repository roots, and skips common non-app trees: `.git/`, `.devlane/`, `.direnv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, and `tmp/`. Overlapping signals do not silently infer hybrid mode; `init` stays conservative there and points at an explicit hybrid template instead. For containerized detections, the scaffold preserves the matched Compose filename list instead of rewriting it to `compose.yaml`.

  Outcomes:
  `single` means one candidate, `monorepo` means multiple candidates, `ambiguous` means no confident signal. `--all` scaffolds every candidate, `--app <path>` targets one subtree, and non-interactive multi-candidate runs fail rather than guessing.

  Flags: `--template <name>`, `--from <path>`, `--app <path>`, `--list`, `--yes`, `--all`, `--force`.

- `inspect` — derive and print the manifest. It always recomputes from the adapter plus live inputs and never reads `.devlane/manifest.json` from disk.
  - When `ports` are declared, `inspect` emits catalog-backed `ports` plus top-level `ready`.
  - Before the first `prepare`, unallocated services emit `allocated: false` plus a provisional `port` computed against the live catalog.
  - For dev lanes, the provisional value is the current bindable candidate `prepare` would pick right now.
  - For stable lanes, `inspect` only emits the fixture when it is currently usable; otherwise it fails with the same unavailability condition `prepare` would surface.

- `prepare` — allocate ports when needed, write the manifest, write `.devlane/compose.env` when compose is declared, and render generated files. If no `devlane.yaml` is found, it points the caller at `devlane init` or an explicit `--config`.

- `up` — start the lane without implicitly mutating state.
  - **Containerized** (`runtime.compose_files`): verifies that the current prepare-owned inputs still match the live manifest/template state, then runs lane-aware `docker compose up`.
  - **Bare-metal with `runtime.run.commands`**: prints the rendered commands and exits. Devlane does not spawn bare processes.
  - **Bare-metal without `runtime.run.commands`**: no-op with a one-line hint.
  - **Hybrid** (both declared): prints the bare-metal commands first, then verifies the current prepare-owned compose inputs and runs compose.
  - When the adapter declares `ports` and any declared service is still `allocated: false`, `up` fails before printing commands or running compose and points the caller at `prepare`.

- `down` — stop the lane.
  - **Containerized**: runs lane-aware `docker compose down`. It does not release catalog ports.
  - **Bare-metal**: no-op. Devlane does not track bare-metal processes.
  - **Hybrid**: runs `docker compose down`. Bare-metal processes remain the user's responsibility.

- `status` — print lane state without mutating anything.
  - **Containerized**: runs `docker compose ps`.
  - **Bare-metal**: for each declared service, reports `bound`, `free`, or `unallocated`. Devlane probes only allocated ports. If `inspect` says a service is still `allocated: false`, `status` prints `unallocated` and does not probe the provisional candidate.
  - **Hybrid**: compose `ps` output plus `bound` / `free` / `unallocated` for every declared host port.
  - Successful reads exit `0`. Non-zero is reserved for invocation, config, or subprocess errors.

- `doctor` — read-only preflight for the current repo. It checks obvious prerequisites and adapter sanity for the current lane context: readable adapter/config, required external tools, and compose-file presence when compose is declared. It does not claim app health, process ownership, or runtime readiness.

The bare-metal asymmetry is deliberate: with compose, the supervisor can answer whether a service is up. Without a supervisor, the best devlane can do is say whether the reserved port is bound.

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
- `os.UserConfigDir()/devlane/catalog.json`

For dev lanes, the durable host-catalog identity is the checkout path. The lane label, branch, and mode remain important manifest metadata and operator-facing display fields, but they do not make a row become a different lane when the user changes branches in place. The stable exception is fixture enforcement for the current checkout.

The repo adapter owns:

- which files are generated
- which Compose files exist
- which profiles are default
- how repo-specific env/config files map from the manifest
- bare-metal run commands (`runtime.run.commands`, always printed, never executed by devlane)
- future worktree seed declarations (`worktree.seed`)

The repo itself owns:

- application code
- service definitions
- product-specific wrapper semantics
- stable deployment policy
- bare-metal process supervision
- manual git worktree flows until worktree lifecycle lands

## Not in scope

No proxy integration, no deploy mechanics, no process supervision, no log collection, no `worktree list`.

## Why `inspect --json` matters

`inspect --json` is the contract that lets agents avoid repo-specific heuristics. Agents should prefer it over reading `.devlane/manifest.json` from disk because the file is only a snapshot from the last `prepare`; `inspect` is always fresh.
