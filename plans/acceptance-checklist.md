# Acceptance checklist

This file is a planning/acceptance artifact, not a statement that the current repo has already met every item. Read `docs/` first for current behavior, and use this checklist only when you need the target acceptance bar for planned work.

Use this as the practical done bar. It is a **target acceptance bar**, not a statement that the current repo has already met every item. Read it in three layers:

- global invariants that must stay true in every phase
- phase-specific acceptance gates
- the final repo-adoption bar

## Global invariants

### Principles and design discipline

- `docs/00-principles.md` states the six principles and is read before any other design doc
- `docs/10-when-to-use-this.md` gates adoption: multiple agents in parallel, many worktrees, or many repos with overlapping port conventions
- `AGENTS.md` non-negotiable #1 matches principle #1 (state vs processes + supervised substrate)
- `AGENTS.md` non-negotiable #11 (no application framework) is present and references `docs/00-principles.md`
- proxy integration and stable deploy mechanics do not appear as planned phases in `plans/phase-roadmap.md`

### Agent experience

- `AGENTS.md` points agents to the correct docs, including `docs/00-principles.md` and `docs/10-when-to-use-this.md`
- docs and schemas agree
- examples still reflect current contracts
- prompt templates remain usable
- the manifest contract contains everything an agent needs for discovery for the active phase: Phase 1 covers lane identity, paths, network fields, compose data, and generated file paths; Phase 2 adds ports with allocation state and top-level `ready`
- agents consistently read `inspect --json` rather than the on-disk `.devlane/manifest.json`
- the agent playbook tells agents to run `prepare` when generated outputs or compose env are needed, not only when Phase 2 `ready` is false
- docs consistently treat dev-lane catalog identity as `(app, repoPath, service)` with `lane` / `mode` / `branch` as metadata

## Phase 1 acceptance

Phase 1 acceptance is intentionally limited to the contract/lifecycle subset. Host-catalog-backed `ports`, `ready`, provisional allocation, `port`, `reassign`, and `host *` commands are accepted in Phase 2 below.

### Init

- `devlane init` creates a valid `devlane.yaml` that passes schema validation
- `devlane init` auto-detects runtime pattern from signals (compose files present → containerized; framework manifest without compose → bare-metal; neither → CLI)
- `devlane init` scans cwd and up to depth 3 below for candidate app roots using `compose*.yaml`, `package.json`, `Cargo.toml`, `go.mod`, `Gemfile`, `*.csproj` as signals
- `devlane init` scans in deterministic lexical order, does not follow symlinks, and skips common non-app trees: `.git/`, `.devlane/`, `.direnv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, `tmp/`
- `devlane init` treats nested Git repository roots as traversal boundaries during parent scans
- `devlane init` in a single-candidate tree scaffolds in place when the candidate is `cwd`; if the only candidate is a descendant, it scaffolds there and prints the selected path
- `devlane init` in a multi-candidate tree enters monorepo mode: lists candidates with inferred kinds and prompts for one or all
- `devlane init --all` in monorepo mode scaffolds an adapter in every candidate without prompting
- `devlane init --app <path>` targets a specific subtree and skips scanning
- `devlane init --list` prints detected candidates without writing anything
- `devlane init --template <name>` overrides detection and uses a named starter template (`containerized-web`, `baremetal-web`, `hybrid-web`, `cli`)
- `devlane init --from <path>` copies an existing adapter as the starting point
- `devlane init --from <path>` validates the source adapter against the current schema before writing anything
- `devlane init --from <path>` copies the source adapter literally; it does not re-root relative paths or rewrite `app`, `lane.host_patterns`, `runtime.compose_files`, template paths, or `worktree.seed`
- `devlane init --from <path>` fails before writing when any copied relative path would escape the target repo root
- `devlane init --from <path>` prints a post-copy review checklist for repo-coupled fields and warns when referenced source-relative inputs such as compose files or templates do not exist in the target repo
- `devlane init` refuses to overwrite an existing `devlane.yaml` unless `--force` is passed
- `devlane init` prompts for confirmation when stdin is a TTY; `--yes` / `--all` / a non-TTY stdin skips the prompt
- when multiple candidates are found and prompting is unavailable, `devlane init` fails unless `--all` or `--app <path>` is provided; it never guesses
- `devlane init` prints its detection reasoning (e.g., `Detected: containerized (found compose.yaml)`) so the user knows why it picked what it picked
- `devlane init` scaffolds `lane.host_patterns` as a commented-out block in every starter template; users opt in by uncommenting
- `devlane init` scaffolds `worktree.seed` as a commented-out block with placeholder examples; users add their own paths explicitly
- `devlane init` never scaffolds `runtime.run.commands` entries that would be executed — there is no execute mode; all bare-metal commands are print-only
- Ambiguous detection defaults to CLI with a clear notice pointing at `--template baremetal-web` or `--template containerized-web`
- hybrid mode is never auto-detected from overlapping filesystem signals alone; when `init` sees both compose and bare-metal hints, it prints a notice pointing at `--template hybrid-web`
- `devlane prepare` on a directory with no `devlane.yaml` prints a pointer to `devlane init`
- any future proxy-signal detection in `init` is suggestion-only; it never silently infers `lane.host_patterns` or hostname ownership into the adapter contract

### Core contract

- `devlane.yaml` can be loaded from cwd or an explicit path
- `prepare`/`inspect`/`up` walk up from cwd to find the nearest `devlane.yaml`
- `inspect --json` always recomputes from live inputs; it never reads `.devlane/manifest.json`
- `inspect --json` emits deterministic JSON
- docs define the path anchors once and use them consistently: `repoRoot` = Git worktree root, `adapterRoot` = directory containing `devlane.yaml`, `repoPath` = catalog identity path for the checkout root
- lane names are stable and slugified according to the documented slug algorithm
- stable vs dev mode is explicit or reproducible
- paths, hostnames, and project names derive from the adapter
- `lane.host_patterns` is optional; the manifest emits `publicHost: null` when omitted

### Manifest shape

- Phase 1 manifest shape is explicit for the non-catalog-backed surface: `schema`, `app`, `kind`, `lane`, `paths`, `network`, `compose`, `outputs` (no top-level `env`, `repo`, or `health`)
- `lane` carries `name`, `slug`, `mode`, `stable`, `branch`, `repoRoot`, `configPath`
- `paths.composeEnv` is present only when the adapter declares `runtime.compose_files` — the key is omitted entirely otherwise, not set to `null`
- `network.publicUrl` is `http://<publicHost>` when `publicHost` is set, `null` otherwise
- env is a *projection* computed at write time, not stored in the manifest; consumers read it from `.devlane/compose.env` or the template `env.*` scope
- env projection uses empty strings, not missing keys, for unavailable optional values such as `DEVLANE_COMPOSE_ENV`, `DEVLANE_PUBLIC_HOST`, and `DEVLANE_PUBLIC_URL`
- Phase 1 template / printed-command scope excludes `ready` and `ports.<name>`; referencing either before Phase 2 is a render error

### Generated outputs

- `prepare` writes the manifest
- `prepare` writes `.devlane/compose.env` when the adapter declares `runtime.compose_files`, omits it otherwise
- `prepare` renders declared templates
- generated directories are created automatically
- missing template fields fail loudly
- templates with undefined variables fail loudly (no silent empty string)
- relative adapter paths resolve from `adapterRoot` and must remain inside `repoRoot`
- generated destinations must resolve inside `repoRoot`; absolute paths outside the checkout are refused
- `prepare` tracks a sidecar hash per generated file under `.devlane/`
- when an on-disk generated file has been hand-edited, `prepare` prints a one-line warning and writes anyway
- first `prepare` with no sidecar hash quietly overwrites existing files with a one-line notice
- `prepare` preserves existing regular-file mode bits for repo-local write targets and writes through existing symlinked file targets without replacing the symlink itself
- `prepare` validates all repo-local writes that can fail before writing, stages repo-local writes to temp files, promotes them in deterministic order, and restores any already-promoted outputs if a late promotion fails

### Compose lifecycle

- Compose commands include the lane-specific project name
- Compose files are resolved relative to `adapterRoot`
- default profiles are included
- `devlane up` in containerized mode runs `docker compose up`
- `devlane down` in containerized mode runs `docker compose down`
- `devlane status` in containerized mode runs `docker compose ps`
- `--dry-run` shows the exact command without running it
- `status` works without mutating state
- successful `status` reads exit `0`; non-zero is reserved for invocation, config, or subprocess errors

### Bare-metal lifecycle

- `devlane up` is a no-op for bare-metal adapters without `runtime.run.commands`, printing a one-line hint
- `devlane up` **prints** rendered commands when `runtime.run.commands` is declared — never spawns them
- `devlane up` never implicitly runs `prepare`
- there is no `runtime.run.mode` field; the schema rejects it
- `devlane down` is always a no-op for bare-metal (no process tracking)
- `devlane status` for bare-metal prints the manifest-derived summary in Phase 1; Phase 2 extends it with `bound` / `free` / `unallocated` host-port results when the adapter declares `ports`
- `runtime.run.commands[].command` renders with the same template scope as `outputs.generated`
- once Phase 2 lands, if `devlane up` would consume prepared state (`runtime.run.commands` or compose) and the adapter declares `ports` with any service still `allocated: false`, it fails before printing anything and points the user at `prepare`

### Hybrid lifecycle

- When an adapter declares both `runtime.compose_files` and `runtime.run.commands`, `devlane up` prints the bare-metal commands first, then runs `docker compose up`
- If compose fails in hybrid mode, the printed bare-metal commands remain visible in the terminal output above the error
- `devlane up` exit code in hybrid mode follows compose's exit code
- `devlane down` in hybrid mode runs `docker compose down`; bare-metal processes are the user's to stop
- `devlane status` in hybrid mode emits compose `ps` output plus the manifest-derived bare-metal summary in Phase 1; Phase 2 extends it with `bound` / `free` / `unallocated` results for declared host ports without inferring per-port substrate ownership
- `examples/hybrid-web/` exercises the pattern end to end (compose sidecar + `runtime.run.commands` + `kind: hybrid`)

### Doctor

- `devlane doctor` is read-only and does not mutate lane or catalog state
- `devlane doctor` checks only tool prerequisites and adapter/config sanity for the current repo
- `devlane doctor` checks required external tools and declared compose files when the current adapter uses compose
- for compose adapters, `devlane doctor` verifies the actual `docker compose` subcommand rather than only checking for a `docker` binary on `PATH`
- `devlane doctor` does not claim app health, process ownership, or lane readiness beyond reporting missing prerequisites or config errors
- `devlane doctor` exits non-zero when a prerequisite or adapter/config error is found
- `devlane doctor` output distinguishes actionable failures from informational notes so it is not confused with `status`

### Validation strictness

- schema-load errors fail before `prepare` logic runs: unknown schema version, invalid `kind`, duplicate `ports[].name`, `lane.host_patterns.dev` missing `{lane}`, `lane.host_patterns.stable == lane.host_patterns.dev`, missing `outputs.manifest_path`, presence of a `runtime.run.mode` field (removed; schema rejects it)
- `prepare`-time errors fail loudly: missing template file, absolute destination outside repo, undefined template variable, out-of-scope template variable, missing compose file
- warnings do not block: `kind: web` with no `ports` and no `runtime.compose_files`

## Phase 2 acceptance

Phase 2 acceptance is tracked in the Linear milestone "Phase 2: Host Catalog Operator Commands" (AUR-126 through AUR-133). Each issue carries its own acceptance criteria.

Most originally-planned Phase 2 acceptance items shipped during Phase 1 stabilization (host config persistence, catalog lock-then-rename, IPv4/IPv6 probing with `V6ONLY=1`, sticky allocation, catalog identity model, deterministic multi-service allocation, catalog-coupled `prepare`, manifest `ports` and top-level `ready`, host-port `status` reporting). Those invariants are exercised by the current portalloc and CLI test suites and described in `docs/65-host-catalog.md`.

Remaining Phase 2 acceptance — the operator command surface (`port`, `reassign`, `host status`, `host doctor`, `host gc`), the catalog-mutation primitive, drift detection, lane resolution, the three stable-port collision recipes, and the Windows error copy fix — is owned by the Linear tickets and indexed in `plans/05-host-catalog-commands.md`.

## Phase 3 acceptance

### Worktree lifecycle

- `devlane worktree create <lane>` runs `git worktree add` at the sibling path `<repo-root-parent>/<repo-root-base>-<lane-slug>`
- `worktree create` creates a new branch named raw `<lane>` from the source checkout's current `HEAD`
- `worktree create` requires `<lane>` to be a valid new local Git branch name and to slugify to a non-empty `<lane-slug>` according to the documented slug algorithm
- `worktree create` fails rather than guessing when the target path already exists, the target branch already exists, or a distinct raw lane name would collide on the same `<lane-slug>`
- `worktree create` rejects `<lane>` equal to the adapter's `stable_name`; the command is for new dev lanes only
- `worktree create` / `worktree remove` are supported only when the active adapter lives at the Git worktree root (`adapterRoot == repoRoot`); subtree adapters in monorepos fail clearly and remain manual `git worktree` territory
- `worktree create` copies every path listed in `worktree.seed` from the source checkout into the new worktree, **before** `prepare` runs
- `worktree.seed` directories are copied recursively
- `worktree.seed` entries are `adapterRoot`-relative; absolute paths are rejected
- normalized `worktree.seed` source paths may not escape the source `repoRoot`, and copy destinations may not escape the target worktree root
- `worktree.seed` entries that are missing in the source checkout warn and continue — they do not fail the command
- `worktree.seed` entries that also appear in `outputs.generated[].destination` are skipped with a one-line notice (prepare will render them)
- `worktree.seed` symlinks are recreated as symlinks; devlane does not dereference or rewrite their targets
- `worktree.seed` preserves regular-file mode bits best-effort and does not preserve ownership
- `worktree.seed` overwrites existing destination paths in the new worktree for explicit seed entries, except for destinations skipped because they are generated outputs
- `worktree create` prints the full list of copied paths on completion for security clarity
- `worktree create` runs `prepare` in the new checkout so the catalog registers the new dev lane's ports before the user starts anything
- if seed copy fails after `git worktree add` succeeds, the command leaves the new checkout on disk, does not publish catalog state, and prints the exact recovery action (`devlane prepare` in that checkout after fixing the issue, or `git worktree remove` to abandon it)
- if `prepare` fails after worktree creation, the command leaves the new checkout and any copied seed files in place, leaves the catalog unpublished for the failed mutation, and prints the exact recovery action
- `worktree create` never auto-removes a checkout that was already created successfully
- `devlane worktree remove <lane>` resolves `<lane>` to the conventional sibling path `<repo-root-parent>/<repo-root-base>-<lane-slug>` by default; if that path does not exist, the command fails clearly instead of guessing
- `devlane worktree remove <lane> --path <worktree>` targets a manually moved or renamed worktree explicitly
- `devlane worktree remove <lane>` runs `git worktree remove` and then dedicated scoped catalog cleanup so the catalog does not accumulate stale entries
- `worktree remove` captures the target checkout's `app` and `repoPath` before `git worktree remove` so scoped cleanup still has a stable identity key after the directory is gone
- scoped cleanup removes only allocations matching the removed worktree's `(app, repoPath)`
- worktree scoped cleanup is not `host gc` and does not scan unrelated repos
- if `git worktree remove` fails, scoped catalog cleanup does not run
- if `git worktree remove` succeeds but scoped cleanup fails, the command reports the partial state clearly and leaves a deterministic recovery path (`devlane host gc --app <app>`)
- there is no `devlane worktree list` command
- devlane does not touch per-worktree git config
- devlane does not ship any default seed list; every path is explicit in the adapter

## Real adoption bar

A repo can be considered adopted when:

- its generated local files come from `prepare`
- its lane runtime can be started with `up` (or, for bare-metal without `runtime.run.commands`, its run commands are documented elsewhere in the repo; they are not expected to be discoverable via the manifest)
- if the repo has credentials or per-developer env files, those are declared in `worktree.seed` so new worktrees get them automatically
- stable vs dev ownership is documented
- an agent can enter the repo, run `inspect --json`, check `ready`, and act without repo-specific port heuristics
