# CLI contract

The shared tool should own **lifecycle**, not product-specific business logic. It owns state it writes and reads process state it can read safely; it does not spawn or stop user processes unless the substrate itself supervises them. See `00-principles.md` for the full rule.

## Lifecycle commands

- `init` — scaffold a starter `devlane.yaml`. Scans for app roots (cwd and up to depth 3 below) and detects runtime pattern from signals at each candidate: `compose*.yaml` → containerized; `package.json` / `Cargo.toml` / `go.mod` / `Gemfile` / `*.csproj` without compose → bare-metal; neither → CLI. The scan walks descendants in lexical order, does not follow symlinks, and skips common non-app trees: `.git/`, `.devlane/`, `.direnv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, and `tmp/`. Outcomes:
  - **single** — one candidate (at cwd). Scaffold in place. Today's default path.
  - **monorepo** — multiple candidates. Print the list with inferred kind per candidate, prompt the user to pick one or all, and scaffold `devlane.yaml` in each chosen subtree. `--all` skips the prompt.
  - **ambiguous** — no confident signal. Scaffold a CLI template and print a notice pointing at `--template baremetal-web` or `--template containerized-web`.

  Flags: `--template <name>` uses a named starter template (`containerized-web`, `baremetal-web`, `cli`), `--from <path>` copies from any existing adapter, `--app <path>` targets a specific subtree and skips scanning, `--list` prints detected candidates without writing anything, `--yes` / `--all` skip interactive prompts (also skipped when stdin is not a TTY), `--force` overwrites an existing file.

  If `init` finds multiple candidates and prompting is unavailable (non-TTY stdin, `--yes`, or an agent context), the command does **not** guess. `--all` means scaffold every candidate; `--app <path>` means scaffold just that subtree; otherwise `init` fails after printing the candidate list and tells the user to rerun with `--all` or `--app`.
- `inspect` — derive and print the manifest. Always recomputes from the adapter and the current catalog; never reads `.devlane/manifest.json` off disk. Works before `prepare` has ever run: for unallocated ports it emits `allocated: false`, `ready: false`, and a **provisional** `port` computed against the live catalog using the current allocation rules. That provisional value is "what `prepare` would pick if it ran right now," not a committed allocation, so it may still change if another writer publishes first.
- `prepare` — write the manifest, render generated files, and allocate ports via the host catalog. If no `devlane.yaml` is found, points the user at `devlane init`. If the compose pattern is in use, also writes `.devlane/compose.env`.
- `up` — start the lane. The semantics follow the supervised-substrate rule:
  - **Containerized** (adapter declares `compose_files`): runs lane-aware `docker compose up`. Compose is the supervisor; devlane is a thin shell over it.
  - **Bare-metal with `runtime.run.commands`**: prints the rendered commands and exits. Devlane does not spawn bare processes, because nothing would supervise them.
  - **Bare-metal without `runtime.run.commands`**: no-op. Prints a one-line hint pointing at `docs/75-baremetal-workflow.md`.
  - **Hybrid** (both `compose_files` and `runtime.run.commands`): prints the bare-metal commands first, then runs compose. If compose fails, the bare-metal plan is still visible above the error. Exit code follows compose.
- `down` — stop the lane.
  - **Containerized**: runs lane-aware `docker compose down`. Does **not** release catalog ports.
  - **Bare-metal**: no-op. Devlane does not track bare-metal processes, even if `up` printed commands for them.
  - **Hybrid**: runs `docker compose down`. Bare-metal processes are the user's to stop.
- `status` — print lane state without mutating anything. `status` is always safe because it only reads:
  - **Containerized**: runs `docker compose ps`. Compose is the authoritative source for which containers are up, their health, and their state.
  - **Bare-metal**: prints the manifest-derived summary and, for every declared service, **probes the allocated port** to report whether something is bound (`bound` / `free`). Devlane cannot say *which* process is bound — it only owns state, not processes — so it does not claim `running` or `ours`. A `bound` port is just evidence that *a* process is listening on the port devlane reserved for that service.
  - **Hybrid**: both. The compose side reports container state; the bare-metal side reports port-bound evidence for the declared services.

The bare-metal asymmetry is deliberate: with compose, the supervisor can answer "is my service up?" definitively; without a supervisor, the best devlane can do is ask the kernel "is this port bound?" and say so plainly.
- `doctor` — read-only preflight for the current repo. Checks obvious prerequisites and adapter sanity for the current lane context: readable adapter/config, required external tools, and compose-file presence when compose is declared. It reports missing prerequisites clearly and exits non-zero on failures. It does not claim app health, process ownership, or runtime readiness.

## Host catalog commands

- `port <service>` — print the currently assigned port for a service. Plain number by default; `--verbose` for metadata; `--probe` to verify bindability via exit code.
- `reassign <service>` — idempotent repair. Probes the current port and only moves it if actually blocked, otherwise no-op. `--lane <name>` changes the lane target while preserving app context:
  - when run inside a repo (or with `--config` / `--cwd` pointing at one), operate on `<service>` for that app and the requested lane
  - when repo context is unavailable and the implementation falls back to the host catalog, succeed only if exactly one catalog entry matches `(lane, service)`; zero matches fail clearly, and multiple matches across apps fail on ambiguity with the matching app/repo pairs printed
- `host status` — list all allocations across the host.
- `host doctor` — read-only host-wide audit. Probes every allocation and reports live conflicts, missing repos, missing service declarations, or repo-identity drift. Identity drift means the adapter currently loaded from `repoPath` no longer derives the same `(app, lane)` pair the catalog row claims. It exits non-zero when any allocation is stale, drifted, or conflicting. It does not delete anything; cleanup remains explicit via `host gc`.
- `host gc` — remove catalog entries whose repos or services no longer exist, or whose current `(app, lane)` at `repoPath` no longer matches the catalog row. Supports `--app`, `--dry-run`, `--yes`.

See `65-host-catalog.md` for the catalog contract, allocation model, and fixture semantics for stable lanes.

## Worktree commands

Worktree lifecycle is Phase 3 (see `100-implementation-plan.md`). The planned shape:

- `worktree create <lane>` — `git worktree add` + seed copy + `prepare` in the new checkout. The target path is a sibling of the source repo root: `<repo-root-parent>/<repo-root-base>-<lane-slug>`. By default the command creates a new branch named `<lane>` from the current `HEAD`; if that branch already exists, it fails rather than silently resetting or reusing a different ref. Seed copy reads the adapter's `worktree.seed` list. `prepare` then registers the dev lane's ports in the catalog before the user starts anything.
- `worktree remove <lane>` — `git worktree remove` + dedicated scoped catalog cleanup so the catalog self-cleans. "Scoped" means removing only allocations whose `(app, lane, repoPath)` match the worktree being removed; it is not a host-wide sweep and it does not run `host gc`.

`worktree list` is explicitly **not** planned. `git worktree list` plus `devlane host status` already tells you what's running where.

## Ownership boundaries

The shared tool owns:

- lane naming
- manifest generation
- path derivation
- compose project naming
- compose env generation
- template rendering
- worktree creation and seed-file copying (Phase 3)
- common health and diagnostic output
- the host catalog and port allocation
- `~/.config/devlane/catalog.json` (state, tool-written)

The repo adapter owns:

- which files are generated
- which Compose files exist
- which profiles are default
- how repo-specific env/config files map from the manifest
- bare-metal run commands (`runtime.run.commands`, always printed, never executed by devlane)
- which files are copied into a new worktree (`worktree.seed`)

The repo itself owns:

- application code
- service definitions
- product-specific wrapper semantics
- stable deployment policy (devlane does not ship deploy mechanics)
- bare-metal process supervision (devlane does not manage processes)

## Not in scope

See `00-principles.md` principle #6 and non-negotiable #11 in `AGENTS.md`. Short version: no proxy integration, no deploy mechanics, no process supervision, no log collection, no `worktree list`.

## Why `inspect --json` matters

`inspect --json` is the contract that lets agents avoid repo-specific heuristics.

Example uses:

- a coding agent finds the public URL without guessing a port
- a shell wrapper reads the state root without re-deriving it
- a proxy integration (user-owned, not devlane) learns the lane hostname and project name
- a test harness discovers generated output paths

Because `inspect` recomputes from the adapter plus the current catalog, the command is always safe to run — it does not mutate state, and it works before `prepare` has ever run.

Agents should prefer `inspect --json` over reading `.devlane/manifest.json` off disk. The file is a snapshot from the last `prepare`; `inspect` is always fresh. See `60-manifest-contract.md` for the three-axes freshness explanation.
