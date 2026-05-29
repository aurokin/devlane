# CLI contract

> **Reference** · Read this when you're changing or calling a `devlane` command and need its exact flags, output, and exit codes. Onward: `65-host-catalog.md` (catalog + drift model), `50-adapter-schema.md` (what a repo declares), `60-manifest-contract.md` (what it produces).

The shared tool owns **lane lifecycle and machine-readable state**, not product-specific business logic. It mutates only the state it owns, reads process state it can observe safely, and refuses to fire-and-forget unsupervised user processes.

## Shipped surface

The current CLI is:

- `init`
- `inspect`
- `prepare`
- `port`
- `up`
- `down`
- `status`
- `doctor`
- `reassign`

Host commands:

- `host status`
- `host doctor`
- `host gc`

Worktree commands:

- `worktree create`
- `worktree remove`

## Lifecycle commands

- `init` — scaffold a starter `devlane.yaml`. It scans for app roots (cwd and up to depth 3 below) and detects runtime pattern from signals at each candidate: `compose*.yml|yaml` or `docker-compose*.yml|yaml` -> containerized; `package.json` / `Cargo.toml` / `go.mod` / `Gemfile` / `*.csproj` without compose -> bare-metal; neither -> CLI. The scan walks descendants in lexical order, does not follow symlinks, skips nested Git repository roots, and skips common non-app trees: `.git/`, `.devlane/`, `.direnv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, and `tmp/`. Overlapping signals do not silently infer hybrid mode; `init` stays conservative there and points at an explicit hybrid template instead. For containerized detections, the scaffold preserves the matched Compose filename list instead of rewriting it to `compose.yaml`.

  Outcomes:
  `single` means one candidate, `monorepo` means multiple candidates, `ambiguous` means no confident signal. `--all` scaffolds every candidate, `--app <path>` targets one subtree, and non-interactive multi-candidate runs fail rather than guessing.

  Flags: `--template <name>`, `--from <path>`, `--app <path>`, `--list`, `--yes`, `--all`, `--force`.

- `inspect` — derive and print the manifest. It always recomputes from the adapter plus live inputs and never reads `.devlane/manifest.json` from disk. Plain `inspect` prints a short human summary; `inspect --json` emits the full manifest JSON, which is what agents should consume.
  - When `ports` are declared, `inspect` emits catalog-backed `ports` plus top-level `ready`.
  - Before the first `prepare`, unallocated services emit `allocated: false` plus a provisional `port` computed against the live catalog.
  - For dev lanes, the provisional value is the current bindable candidate `prepare` would pick right now.
  - For stable lanes, `inspect` only emits the fixture when it is currently usable; otherwise it fails with the same unavailability condition `prepare` would surface.

- `prepare` — allocate ports when needed, write the manifest, write `.devlane/compose.env` when compose is declared, and render generated files. If no `devlane.yaml` is found, it points the caller at `devlane init` or an explicit `--config`.

- `port <service>` — print the assigned port for one declared service in the current adapter. It uses the same repo-context flags as `inspect` and `prepare`: `--cwd`, `--config`, `--lane`, `--mode`, and `--profile`.
  - Default output is a single integer on stdout, newline-terminated. Successful lookup exits `0`.
  - `--verbose` prints one human-readable line with `service`, `port`, `allocated`, `mode`, `lane`, and `repoPath`.
  - `--probe` probes the assigned port on both `0.0.0.0` and `[::]` using the same TCP probe as allocation/status. The command still prints the port to stdout. It exits `0` when the port is bindable on both supported families and exits `1` when the probe fails.
  - `port` reports assigned catalog rows only. It does not print provisional candidates. Before the first `prepare` for a service, it exits `1` with guidance to use `inspect --json` for the current provisional candidate or `prepare` to commit an allocation.
  - Unknown services, adapter/config errors, malformed catalog state, and missing assigned rows exit non-zero with a message on stderr.

- `up` — start the lane without implicitly mutating state.
  - **Containerized** (`runtime.compose_files`): verifies that the current prepare-owned inputs still match the live manifest/template state, then runs lane-aware `docker compose up`.
  - **Bare-metal with `runtime.run.commands`**: prints the rendered commands and exits. Devlane does not spawn bare processes.
  - **Bare-metal without `runtime.run.commands`**: no-op with a one-line hint.
  - **Hybrid** (both declared): prints the bare-metal commands first, then verifies the current prepare-owned compose inputs and runs compose.
  - When the adapter declares `ports` and any declared service is still `allocated: false`, `up` fails before printing commands or running compose and points the caller at `prepare`. This gating applies to runtime shapes that actually start something — containerized, bare-metal with `runtime.run.commands`, and hybrid. Pure ports-only bare-metal adapters without `runtime.run.commands` keep the no-op `up` behavior described above.
  - `--dry-run` (containerized and hybrid) prints the `docker compose up` command and exits `0` without running it.

- `down` — stop the lane.
  - **Containerized**: runs lane-aware `docker compose down`. It does not release catalog ports.
  - **Bare-metal**: no-op. Devlane does not track bare-metal processes.
  - **Hybrid**: runs `docker compose down`. Bare-metal processes remain the user's responsibility.
  - `--dry-run` (containerized and hybrid) prints the `docker compose down` command and exits `0` without running it.

- `status` — print lane state without mutating anything.
  - **Containerized**: runs `docker compose ps`.
  - **Bare-metal**: for each declared service, reports `bound`, `free`, or `unallocated`. Devlane probes only allocated ports. If `inspect` says a service is still `allocated: false`, `status` prints `unallocated` and does not probe the provisional candidate.
  - **Hybrid**: compose `ps` output plus `bound` / `free` / `unallocated` for every declared host port.
  - `--dry-run` (containerized and hybrid) prints the `docker compose ps` command and exits `0` without running it.
  - Successful reads exit `0`. Non-zero is reserved for invocation, config, or subprocess errors.

- `doctor` — read-only preflight for the current repo. It checks obvious prerequisites and adapter sanity for the current lane context: readable adapter/config, required external tools, and compose-file presence when compose is declared. It does not claim app health, process ownership, or runtime readiness.

- `host status` — list every allocation in the host catalog, sorted by `(app, repoPath, service)`. Output columns: `app`, `mode`, `lane`, `service`, `port`, `repoPath`. Empty catalog prints `no allocations` and exits `0`. The read does not acquire the catalog lock, so it is safe to run during an in-flight `prepare`; the lock-then-rename write discipline guarantees the read sees either the pre-write or post-write file, never a partial one. Non-zero exit only on read or invocation failure.

- `host doctor` — read-only audit of the host catalog. A row's `repoPath` is the Git worktree root, which in a monorepo may host several subtree adapters, so `doctor` discovers every `devlane.yaml` under the worktree root — skipping the same trees as `init` (`.git`, `node_modules`, build output, etc.) and stopping at nested Git roots, since a nested checkout is a different worktree with its own rows — and matches each row to the adapter that declares its `app`. Discovery is unbounded in depth because `prepare` records `repoPath` as the worktree root regardless of how deep the adapter lives. It classifies drift into the five categories defined in the drift model in `65-host-catalog.md` — `missing-repoPath`, `missing-service`, and `app-mismatch` (the three `host gc` treats as removable), plus `duplicate-claim` and `bad-adapter` (surfaced for the operator but requiring a human decision). Discovery is conservative about uncertainty: an unreadable directory, an unparseable adapter, or a worktree root whose own parent is missing (so the absence may be an offline volume rather than a deletion) never demotes a row into a removable category, so a transient failure cannot make a healthy allocation look safe to delete. Loader errors are surfaced as classified findings, never panics; a single malformed subtree adapter never hides a healthy sibling. Each reported allocation's port is probed for `bound` / `free` context; probing is informational only and never creates a finding, so a bound-but-singly-claimed row is not flagged on its own. Like `host status`, the read does not acquire the catalog lock and never mutates the catalog. A clean catalog prints `no drift detected` and exits `0`; any finding exits `1`.

- `host gc` — remove host-catalog rows that `host doctor` would classify as safe to delete. It reuses the same drift detector and adapter-discovery loader as `host doctor`, so the two commands can never disagree about what is removable. It removes only the three removable categories — `missing-repoPath`, `missing-service`, and `app-mismatch`; `duplicate-claim` and `bad-adapter` are surfaced for the operator but never removed, because each needs a human to decide which claimant or adapter is authoritative. These categories are surfaced per finding, not per row: `duplicate-claim` and `bad-adapter` never *drive* removal and never *protect* a row that independently qualifies as removable. A row whose worktree, app, or service is provably gone is removed on that basis even when it also appears under `duplicate-claim`, and removing it resolves the duplicate as a side effect by leaving the healthy claimant the sole owner of the port; only a row whose findings are *entirely* surfaced-only is preserved, because that is the only case where which claimant is authoritative is still genuinely undecided. Detection runs over the whole catalog so cross-app `duplicate-claim` is computed correctly; `--app <name>` then narrows both the displayed and removed findings to one app. Removal is gated: `--dry-run` prints the removable (and surfaced) findings and exits `0` without touching the catalog; otherwise the command requires either `--yes` or an interactive confirmation. With neither `--yes` nor a TTY it refuses and exits `1` without mutating, so an unattended run can never delete rows silently; an interactive operator who declines the prompt also exits `1` with the catalog unchanged. All removals go through the catalog `Mutate` primitive (lock-then-rename) in a single atomic write. Under that lock gc re-runs drift detection against the freshly locked snapshot and removes a row only when it is *still* classified removable at that moment **and** was shown to and confirmed by the operator (matched by its entire value — every field, including port and `lastPrepared`). So a row that was repaired between the unlocked scan, or while an interactive prompt was waiting, and the locked write — whether its adapter was fixed, its worktree restored, or the row refreshed by a concurrent `prepare` — is re-validated and left untouched rather than removed on a stale classification, and a row that drifted only after the operator saw the preview is never removed without confirmation. The re-detection runs the same adapter discovery as `host doctor`; it touches the filesystem but not the catalog, so holding the lock across it cannot deadlock, and it happens after the prompt, never during it. Limitation (offline volumes): because the re-detection re-runs that same discovery, it only protects against a volume that comes back online between the scan and the locked write — a worktree whose volume is offline for the *entire* run is re-classified `missing-repoPath` again and removed, and under bare `--yes` the confirmation does not apply. Reliably distinguishing a deleted worktree from an empty, persistently-parked mountpoint is not possible from the local filesystem, so this is a known, accepted limitation. gc removes a catalog row, not the worktree's code; a wrongly-removed row is reconstructed by re-running `prepare` (a stable lane reclaims its fixed port unless it was taken in the interim; a dev lane re-picks from the pool). Operators who keep worktrees on removable or network volumes should not wire `host gc --yes` into cron without ensuring those volumes are mounted, or should scope it with `--app`. A clean catalog, or one whose only findings are surfaced-only, prints `no removable drift` and exits `0` — `host gc` is cleanup, not an audit, so surfaced-only drift does not make it fail; `host doctor` remains the command that exits non-zero on any finding. Successful removal prints how many allocations were removed and exits `0`.

- `reassign <service>` — move a single allocation onto a fresh port using the same sticky resolution rules as `prepare`. Mutation scope is the requested service only — every other catalog row is left untouched.
  - Idempotent on a bindable port: when the current allocation's port is bindable, `reassign` exits 0 and writes nothing.
  - `--force` always invokes the allocator. For dev lanes the current port is treated as held so the row is displaced onto a different bindable port; for stable lanes the fixture is reclaimed (so `--force` is effectively a no-op when the fixture is already held).
  - `--lane <name>` selects the target row by lane name within the current adapter's `app`. Resolution scopes to repo context (no cross-app scanning); worktrees of the same app match because their `repoPath` values resolve under symlink evaluation. When more than one checkout in the same app shares the lane name, the resolver's tiebreak prefers the caller's own `repoPath` so an operator inside a worktree can target their local lane unambiguously. If no match wins the tiebreak, the command exits 1 and enumerates the colliding checkouts.
  - All mutations go through the catalog `Mutate` primitive (lock-then-rename) so concurrent `prepare` and `reassign` invocations serialize correctly.

- `worktree create <lane>` — add a new dev-lane checkout and register it. It runs `git worktree add` at the sibling path `<repo-root-parent>/<repo-root-base>-<lane-slug>` on a new branch named raw `<lane>` created from the current `HEAD`, copies the adapter's `worktree.seed` paths into the new checkout, then runs `prepare` there so the catalog records the new lane's ports before anyone starts it. It only operates when the active adapter lives at the Git worktree root (`adapterRoot == repoRoot`); a subtree adapter in a monorepo fails clearly and stays manual `git worktree` territory.
  - `<lane>` must be a valid new local Git branch name and must slugify to a non-empty `<lane-slug>`. The command fails rather than guessing when the target path already exists, the branch already exists, or a distinct raw lane name would collide on the same slug (caught by the path-exists check). `<lane>` equal to the adapter's `stable_name` is rejected — the command is for new dev lanes only.
  - Seed copying happens before `prepare`. Entries are `adapterRoot`-relative; absolute paths and paths that escape the repo root are rejected up front, before any checkout is created. Directories are copied recursively, symlinks are recreated as symlinks (never dereferenced), and regular-file mode bits are preserved best-effort. Existing destinations are overwritten, except entries that match an `outputs.generated[].destination` — those are skipped with a notice because `prepare` renders them. Missing sources warn and continue. The full list of copied paths is printed for security clarity.
  - Failure is non-destructive: if seed copy or `prepare` fails after the checkout is created, the checkout (and any copied seeds) is left in place, the catalog mutation is not published, and the command prints the exact recovery action — fix the issue and run `devlane prepare` in the new checkout, or `git worktree remove --force` to abandon it. A successfully created checkout is never auto-removed.

- `worktree remove <lane>` — retire a lane checkout and clean up its catalog rows. By default it resolves `<lane>` to the conventional sibling path; if that path does not exist it fails rather than guessing, and `--path <worktree>` targets a manually moved or renamed checkout. It captures the target's `app` and `repoPath` before removal, runs `git worktree remove`, then runs scoped catalog cleanup that deletes only allocations matching the removed worktree's `(app, repoPath)` — this is not `host gc` and never scans unrelated repos.
  - Without `--force`, `git worktree remove` refuses a worktree with uncommitted or untracked changes (which includes `prepare`'s generated outputs); `--force` discards them. If `git worktree remove` fails, scoped cleanup does not run. If removal succeeds but cleanup fails, the command reports the partial state and points at `devlane host gc --app <app>` as the deterministic recovery.

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
- worktree seed declarations (`worktree.seed`), copied into new checkouts by `worktree create`

The repo itself owns:

- application code
- service definitions
- product-specific wrapper semantics
- stable deployment policy
- bare-metal process supervision
- branch content and any git worktree flows beyond `worktree create` / `worktree remove`

## Not in scope

No proxy integration, no deploy mechanics, no process supervision, no log collection, no `worktree list`.

## Why `inspect --json` matters

`inspect --json` is the contract that lets agents avoid repo-specific heuristics. Agents should prefer it over reading `.devlane/manifest.json` from disk because the file is only a snapshot from the last `prepare`; `inspect` is always fresh.
