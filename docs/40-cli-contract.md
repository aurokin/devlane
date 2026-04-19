# CLI contract

The shared tool should own **lifecycle**, not product-specific business logic. It owns state it writes and reads process state it can read safely; it does not spawn or stop user processes unless the substrate itself supervises them. See `00-principles.md` for the full rule.

This document mixes two surfaces:

- **Phase 1 current surface** — `init`, `inspect`, `prepare`, generated outputs, compose lifecycle, printed bare-metal commands, read-only `doctor`
- **Phase 2 target surface** — host-catalog-backed `ports`, top-level `ready`, `port`, `reassign`, `host *`, and host-port-aware `status`

When a behavior is Phase-2-only, it is called out explicitly below.

## Lifecycle commands

- `init` — scaffold a starter `devlane.yaml`. It scans for app roots (cwd and up to depth 3 below) and detects runtime pattern from signals at each candidate: `compose*.yml|yaml` or `docker-compose*.yml|yaml` → containerized; `package.json` / `Cargo.toml` / `go.mod` / `Gemfile` / `*.csproj` without compose → bare-metal; neither → CLI. The scan walks descendants in lexical order, does not follow symlinks, and skips common non-app trees: `.git/`, `.devlane/`, `.direnv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, and `tmp/`. Overlapping signals do not silently infer hybrid mode; `init` stays conservative there and points at an explicit hybrid template instead. For containerized detections, the scaffold preserves the matched Compose filename list instead of rewriting it to `compose.yaml`. Outcomes:
  - **single** — exactly one candidate. If it is `cwd`, scaffold in place. If it is a descendant, scaffold there and print that choice explicitly.
  - **monorepo** — multiple candidates. Print the list with inferred kind per candidate, prompt the user to pick one or all, and scaffold `devlane.yaml` in each chosen subtree. `--all` skips the prompt.
  - **ambiguous** — no confident signal. Scaffold a CLI template and print a notice pointing at `--template baremetal-web` or `--template containerized-web`.

  Flags: `--template <name>` uses a named starter template (`containerized-web`, `baremetal-web`, `hybrid-web`, `cli`), `--from <path>` copies from any existing adapter as a literal starting point, `--app <path>` targets a specific subtree and skips scanning, `--list` prints detected candidates without writing anything, `--yes` / `--all` skip interactive prompts (also skipped when stdin is not a TTY), `--force` overwrites an existing file.

  If `init` finds multiple candidates and prompting is unavailable (non-TTY stdin, `--yes`, or an agent context), the command does **not** guess. `--all` means scaffold every candidate; `--app <path>` means scaffold just that subtree; otherwise `init` fails after printing the candidate list and tells the user to rerun with `--all` or `--app`.
  `init --from <path>` validates the source adapter against the current schema before writing anything. It does **not** re-root relative paths or silently rewrite repo-coupled fields. `lane.path_roots`, `runtime.compose_files`, template paths, output paths (`outputs.manifest_path`, `outputs.compose_env_path`, `outputs.generated[].destination`), `worktree.seed`, `app`, and `lane.host_patterns` are copied as written. If any copied relative path would escape the target repo root, `init` fails before writing; otherwise it prints a review checklist and warns about referenced source-relative inputs that do not exist in the target repo.
- `inspect` — derive and print the manifest. Always recomputes from the adapter plus current live inputs; never reads `.devlane/manifest.json` off disk.
  - **Phase 1:** derives lane identity, paths, network fields, compose data, and outputs without host-catalog-backed `ports` or top-level `ready`.
  - **Phase 2:** extends `inspect` with catalog-backed `ports` and top-level `ready`. Before the first `prepare`, unallocated ports emit `allocated: false` plus a **provisional** `port` computed against the live catalog. For dev lanes, that provisional value is the current bindable candidate `prepare` would pick right now. For stable lanes, `inspect` only emits the fixture when it is currently usable; otherwise it fails with the same unavailability condition `prepare` would surface. Any provisional answer is not a committed allocation, so it may still change if another writer publishes first.
- `prepare` — write the manifest and render generated files. If no `devlane.yaml` is found, points the user at `devlane init` or an explicit `--config`. If the compose pattern is in use, also writes `.devlane/compose.env`.
  - **Phase 1:** no host catalog mutation.
  - **Phase 2:** allocates ports via the host catalog as part of the same command.
- `up` — start the lane. The semantics follow the supervised-substrate rule:
  - **Containerized** (adapter declares `runtime.compose_files`): verifies that the current prepare-owned compose inputs (`.devlane/compose.env` plus any declared `outputs.generated`) still match the live manifest/template state, then runs lane-aware `docker compose up`. Compose is the supervisor; devlane is a thin shell over it.
  - **Bare-metal with `runtime.run.commands`**: prints the rendered commands and exits. Devlane does not spawn bare processes, because nothing would supervise them. It never implicitly runs `prepare`. In Phase 2, if the adapter declares `ports` and any declared service is still `allocated: false`, `up` fails before printing anything and points the caller at `prepare`.
  - **Bare-metal without `runtime.run.commands`**: no-op. Prints a one-line hint pointing at `docs/75-baremetal-workflow.md`.
  - **Hybrid** (both `runtime.compose_files` and `runtime.run.commands`): prints the bare-metal commands first, then verifies the current prepare-owned compose inputs and runs compose. If compose fails, the bare-metal plan is still visible above the error. Exit code follows compose. In Phase 2, if the adapter declares `ports` and any declared service is still `allocated: false`, `up` fails before printing commands or running compose and points the caller at `prepare`.
- `down` — stop the lane.
  - **Containerized**: runs lane-aware `docker compose down`. Does **not** release catalog ports.
  - **Bare-metal**: no-op. Devlane does not track bare-metal processes, even if `up` printed commands for them.
  - **Hybrid**: runs `docker compose down`. Bare-metal processes are the user's to stop.
- `status` — print lane state without mutating anything. `status` is always safe because it only reads:
  - **Containerized**: runs `docker compose ps`. Compose is the authoritative source for which containers are up, their health, and their state.
  - **Bare-metal**:
    - **Phase 1:** prints the manifest-derived summary only.
    - **Phase 2:** for every declared service, also reports one of three states: `bound`, `free`, or `unallocated`. Devlane probes only **allocated** ports. When `inspect` says a service is still `allocated: false`, `status` reports `unallocated` and may show the current provisional candidate, but it does not probe that provisional port. Devlane cannot say *which* process is bound — it only owns state, not processes — so it does not claim `running` or `ours`. A `bound` port is just evidence that *a* process is listening on the port devlane reserved for that service.
  - **Hybrid**:
    - **Phase 1:** compose `ps` output plus the manifest-derived bare-metal summary.
    - **Phase 2:** compose `ps` output plus `bound` / `free` / `unallocated` for every declared host port. The current adapter contract does not try to classify individual declared ports as "compose-owned" or "bare-metal-owned."

The bare-metal asymmetry is deliberate: with compose, the supervisor can answer "is my service up?" definitively; without a supervisor, the best devlane can do in Phase 2 is ask the kernel "is this port bound?" and say so plainly.
- Successful `status` reads exit `0`. Non-zero is reserved for invocation, config, or subprocess errors rather than "a service is free" or "a service is unallocated." Per-service output remains deterministic: bare-metal service rows follow adapter declaration order.
- `doctor` — read-only preflight for the current repo. Checks obvious prerequisites and adapter sanity for the current lane context: readable adapter/config, required external tools, and compose-file presence when compose is declared. For compose adapters, the required external tool is the actual `docker compose` subcommand, not just a `docker` binary on `PATH`. It reports missing prerequisites clearly and exits non-zero on failures. It does not claim app health, process ownership, or runtime readiness.

## Host catalog commands (Phase 2)

- `port <service>` — print the currently assigned port for a service. Plain number by default; `--verbose` for metadata; `--probe` to verify bindability via exit code. If the service has not been allocated yet, fail clearly and point the caller at `inspect --json` for the current provisional candidate or `prepare` to commit one.
- `reassign <service>` — scoped repair for one service allocation. By default it resolves the current checkout's `(app, repoPath, service)` row, probes the current port, and only moves it if actually blocked. `--force` skips the bindability no-op check and moves the allocation even when the current port is free; this is the explicit tool for moving an offline dev lane aside so stable can reclaim its fixture. `--lane <name>` is a convenience selector, not part of the identity key:
  - when run inside a repo (or with `--config` / `--cwd` pointing at one), operate on `<service>` for the current checkout by default
  - with `--lane <name>`, look for another allocation of the same app whose latest prepared metadata reports that lane name, then operate on its `(app, repoPath, service)` row
  - if repo context is unavailable and the implementation falls back to the host catalog, succeed only if exactly one catalog row matches the selector; zero matches fail clearly, and **any** multiple match fails on ambiguity with the matching app/repo pairs printed
- `host status` — list all allocations across the host. Output is deterministic: rows are ordered by `app`, then `repoPath`, then `service`. Successful reads exit `0`; non-zero is reserved for invocation, config, or read failures.
- `host doctor` — read-only host-wide audit. Probes every allocation and reports `bound` / `free` state for operator context, missing repos, missing service declarations, app/repo-path mismatches, and duplicate catalog claims. Metadata changes within a checkout (branch switch, lane-label change, stable/dev mode flip) are not treated as drift; they are refreshed on the next `prepare`. A singly claimed bound port is not an error by itself because host-wide probing cannot prove process ownership for bare-metal lanes. The command exits non-zero when any allocation is stale or when duplicate catalog claims exist. It does not delete anything; cleanup remains explicit via `host gc`.
- `host gc` — remove catalog entries whose repos or services no longer exist, or whose current adapter at `repoPath` no longer identifies the same app. Supports `--app`, `--dry-run`, `--yes`. When stdin is not a TTY, it fails unless `--yes` or `--dry-run` is provided.

See `65-host-catalog.md` for the catalog contract, allocation model, and fixture semantics for stable lanes.

## Worktree commands

Worktree lifecycle is Phase 3 (see `100-implementation-plan.md`). The planned shape:

- `worktree create <lane>` — supported only when the active adapter lives at the Git worktree root (`adapterRoot == repoRoot`). Subtree adapters in monorepos are out of scope for Phase 3; the command fails clearly there and points users at manual `git worktree` flows. When supported, the command does `git worktree add` + seed copy + `prepare` in the new checkout. The target path is a sibling of the source repo root: `<repo-root-parent>/<repo-root-base>-<lane-slug>`. By default the command creates a new branch named `<lane>` from the current `HEAD`; if that branch already exists, it fails rather than silently resetting or reusing a different ref. `<lane>` is the raw lane label and branch name; it must be a valid new local Git branch name, must slugify to a non-empty `<lane-slug>` under the documented slug algorithm, and must not equal the adapter's `stable_name`. The branch collision check uses raw `<lane>`; the path collision check uses `<lane-slug>`. Distinct raw lane names that would slugify to the same `<lane-slug>` are rejected because they would target the same sibling path. This command creates dev lanes only. Seed copy reads the adapter's `worktree.seed` list. Seed entries are resolved relative to `adapterRoot`, absolute paths are rejected, normalized paths may not escape `repoRoot`, and copy destinations must remain inside the target worktree root. `prepare` then registers the dev lane's ports in the catalog before the user starts anything.
- `worktree remove <lane>` — supported only when the active adapter lives at the Git worktree root (`adapterRoot == repoRoot`). Subtree adapters in monorepos are out of scope for Phase 3; the command fails clearly there and points users at manual `git worktree` flows. When supported, the command does `git worktree remove` + dedicated scoped catalog cleanup so the catalog self-cleans. By default `<lane>` resolves to the conventional sibling path `<repo-root-parent>/<repo-root-base>-<lane-slug>`. If that path does not exist, the command fails rather than guessing from mutable lane metadata. `--path <worktree>` targets a manually renamed or moved worktree explicitly. "Scoped" means removing only allocations whose `(app, repoPath)` match the worktree being removed; it is not a host-wide sweep and it does not run `host gc`. Capture the target checkout's `app` and `repoPath` before removal so cleanup still has a stable key after the directory is gone.

`worktree list` is explicitly **not** planned. `git worktree list` plus `devlane host status` already tells you what's running where.

### Worktree failure semantics

`worktree create` is ordered as:

1. validate `<lane>` and target collisions
2. `git worktree add`
3. seed copy
4. `prepare` in the new checkout

Failure handling is explicit:

- if validation fails, nothing is created
- if `git worktree add` fails, nothing else runs
- if seed copy fails after `git worktree add`, devlane leaves the new worktree on disk, does **not** run `prepare`, does **not** publish any catalog mutation, and prints the exact worktree path plus the recovery choices: fix the seed issue and run `devlane prepare` in that checkout, or remove the checkout with `git worktree remove`
- if `prepare` fails after seed copy, devlane leaves the new worktree and any copied seed files in place, and the `prepare` failure rules apply: repo-local outputs are rolled back and the catalog mutation stays unpublished. The printed recovery path is to fix the reported issue and rerun `devlane prepare` in the new checkout, or manually remove the checkout if the lane is being abandoned

`worktree create` does **not** auto-remove a checkout that was already created successfully. Once files or credentials may have been copied into place, cleanup stays explicit.

`worktree remove` is ordered as:

1. resolve the target worktree path and capture its `(app, repoPath)` identity
2. run `git worktree remove`
3. delete only the captured `(app, repoPath)` allocations from the catalog

Failure handling is explicit here too:

- if path resolution or identity capture fails, nothing is removed
- if `git worktree remove` fails, catalog cleanup does not run
- if `git worktree remove` succeeds but scoped catalog cleanup fails, the worktree stays removed and the command prints that the remaining repair step is catalog cleanup; the deterministic recovery path is `devlane host gc --app <app>` because the removed `repoPath` now satisfies the normal stale-entry rules

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

For dev lanes, the durable host-catalog identity is the checkout path. The lane label, branch, and mode remain important manifest metadata and operator-facing display fields, but they do not make a row "become a different lane" when the user changes branches in place. The stable exception is fixture enforcement for the current checkout: if the user flips the checkout into stable mode, devlane may update that same row onto the stable fixture rather than reusing a dev-only port.

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
