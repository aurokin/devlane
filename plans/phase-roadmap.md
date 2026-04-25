# Implementation plan

This file is planning context, not a current-state product contract. Read `docs/` first for shipped behavior, and use Linear for execution state.

This scaffold is intentionally phased.

Current implementation note:

- the shipped CLI is `init`, `inspect`, `prepare`, `up`, `down`, `status`, and `doctor`
- host-catalog-backed `ready`, `ports`, sticky allocation, and host-port-aware `status` are already implemented
- the remaining unscheduled operator surface is mostly `port`, `reassign`, `host *`, plus Phase 3 worktree lifecycle

## Phase 1 — contract first

Goal: make the shared manifest, generated-output flow, and lifecycle shape real before taking over host-wide coordination.

Deliverables:

- stable adapter schema for the initial surface: optional `lane.host_patterns`, optional `runtime.run.commands`, generated outputs, and zero-friction adoption via `init`
- stable manifest schema for the initial surface: lane identity, derived paths, network, compose, generated-output metadata, and the current host-catalog-backed `ports` / `ready` fields
- one path-anchor glossary shared across docs and schemas: `repoRoot` = Git worktree root, `adapterRoot` = directory containing `devlane.yaml`, `repoPath` = catalog identity path for the checkout root. Relative adapter paths resolve from `adapterRoot` and must remain inside `repoRoot`
- working `inspect` that always recomputes from adapter + current checkout state, never reads manifest.json from disk. In Phase 2 it extends that recomputation with the host catalog
- working `prepare` with strict validation for adapter load, template existence, destination containment, undefined template variables, and compose-file presence, plus sidecar-hash detection for hand-edited generated files
- explicit Phase 1 failure semantics for `prepare`: validate everything that can fail before writing, stage repo-local writes to temp files in the destination directories, promote them in deterministic order via atomic rename where possible, and roll back any already-promoted outputs if a late promotion fails. Catalog-coupled publish semantics for `prepare` / `reassign` land in Phase 2
- `devlane init` for zero-friction adoption (deterministic lexical scan from cwd to depth 3; skip common non-app trees; no symlink traversal; `--template`, `--from`, `--app`, `--list`, `--yes`, `--all`, `--force`; fail rather than guess in non-interactive monorepo mode)
- add an explicit `hybrid-web` starter template. Auto-detection remains conservative: `init` detects containerized, bare-metal, or CLI from filesystem signals, but it does not silently infer hybrid mode from overlapping signals. When the repo looks mixed, `init` should say so and point at `--template hybrid-web`
- `devlane init --from <path>` copies an existing adapter as a literal starting point. It validates the source adapter against the current schema before writing anything, does not re-root or rewrite relative paths, does not silently change `app` or hostname patterns, and prints a review checklist for repo-coupled fields (`runtime.compose_files`, template paths, output paths, `worktree.seed`, and similar settings) that may not make sense in the target repo
- lane-aware `up`, `down`, `status`, and `doctor`:
  - **Containerized** (`runtime.compose_files` declared): `up` runs `docker compose up`, `down` runs `docker compose down`, `status` runs `docker compose ps`. The compose substrate is the supervisor.
  - **Bare-metal** (`runtime.run.commands` declared): `up` prints the rendered commands and exits. It never implicitly runs `prepare`. Once Phase 2 lands, if the adapter declares `ports` and any declared service is still `allocated: false`, `up` fails before printing anything and points the caller at `prepare`. Port-only bare-metal adapters without `runtime.run.commands` stay no-op `up` paths. `down` is a no-op. In Phase 1, `status` prints the manifest-derived summary only; host-port probing lands in Phase 2. Devlane never spawns bare processes.
  - **Hybrid** (both declared): `up` prints the bare-metal commands first, then runs compose. Exit code follows compose. Once Phase 2 lands, if the adapter declares `ports` and any declared service is still `allocated: false`, `up` fails before printing commands or running compose and points the caller at `prepare`. In Phase 1, `status` reports compose `ps` output plus the manifest-derived bare-metal summary. Phase 2 extends that with `bound` / `free` / `unallocated` host-port results for declared services
  - **Doctor**: read-only preflight for tool prerequisites and adapter sanity; no app-health claims, no process supervision, no catalog mutation.
- example adapters
- acceptance tests

### Phase 1 is done when

- `init`, `inspect`, `prepare`, `up`, `down`, `status`, and `doctor` all have explicit behavior documented and tested for containerized, bare-metal, hybrid, and no-lifecycle adapters without depending on host-catalog-backed coordination
- `up` never implicitly mutates state, and the docs are explicit that once Phase 2 lands it fails whenever the adapter declares `ports` and any declared service is still unallocated
- the template / bare-metal-command variable scope is described once and repeated consistently across the adapter, manifest, and workflow docs
- the docs are explicit about the current template scope, including top-level `ready` and flattened `ports.<name>` values
- the acceptance checklist can point to this phase without relying on unnamed checklist "groups"

## Phase 2 — host catalog operator commands

Goal: ship the operator command surface for the host catalog so port queries, repairs, audits, and cleanup are first-class CLI flows instead of manual catalog edits.

This phase is a prerequisite for worktree lifecycle automation because `worktree create` should register allocations into the catalog when a new lane is spun up.

Phase 1 stabilization shipped most of the originally-planned Phase 2 plumbing. The host config parser, catalog persistence with lock-then-rename atomicity, sticky allocation engine, IPv4/IPv6 probing, catalog-coupled `prepare` orchestration with rollback, manifest `ports` and `ready`, `inspect --json` recompute from live catalog, `status` host-port reporting, the catalog identity model, and the catalog schema at `schemas/catalog.schema.json` are all in place. What remains is the operator surface plus targeted polish.

Execution state is tracked in the Linear milestone "Phase 2: Host Catalog Operator Commands" (AUR-126 through AUR-133). Per-issue acceptance bars live in Linear; the deliverables below describe the shape.

Deliverables:

- exported catalog API in the port-allocation package: an `Allocation` row type and a no-lock `List()` reader, used by every subsequent operator command
- exported `Mutate(fn)` callback-style mutation primitive in the port-allocation package wrapping the existing lock-then-rename discipline; this is the single contract every write-side host-state mutation builds on
- a lane resolver module: catalog + repo context (app + repoPath, symlink-evaluated) + lane name → single match / ambiguity / not-found. Repo context is mandatory; the resolver never scans across apps. Worktrees of the same app match
- a drift detection module: catalog snapshot + injected adapter loader → categorized findings (missing-repoPath, missing-service, app-mismatch, duplicate-claim). Pure logic, no I/O of its own. Shared by `host doctor` and `host gc`
- `devlane port <service>` with `--verbose` and `--probe`. The command remains about assigned ports, not provisional candidates: before first `prepare` for that service it fails clearly and points callers at `inspect --json` (for the current provisional candidate) or `prepare` (to commit one). `--probe` always prints the port to stdout regardless of probe outcome and signals success/failure via exit code only
- `devlane reassign <service>` — idempotent on a bindable port, supports `--force` for intentional displacement of an offline lane and `--lane <name>` as a metadata selector. `--lane` requires repo context (works from main repo or any worktree of the same app); any multi-match fails loudly. Mutation scope is the requested service only
- `devlane host status` listing every catalog row in deterministic `(app, repoPath, service)` order, read-only, no lock acquired
- `devlane host doctor` — read-only audit using the drift module. Reports probe results (`bound` / `free`) for operator context but does not treat a singly claimed bound port as an error by itself, because host-wide probing cannot prove process ownership for bare-metal lanes. Exits 1 on any finding
- `devlane host gc` — uses the drift module to identify removable rows (missing-repoPath, missing-service, app-mismatch). Supports `--app`, `--dry-run`, `--yes`; non-interactive mode fails unless `--yes` or `--dry-run` is provided. Mutates via `Mutate`. Duplicate-claim findings are surfaced for operator awareness but are not auto-removed. Bound-but-singly-claimed rows are never removed. `worktree.seed` is not considered (Phase 3 territory)
- collision messaging for the three documented stable-port scenarios via a small collision-message formatter module: scenario 1 (held by another app's stable) retains manual-resolution prose; scenarios 2 (held by an offline dev lane) and 3 (held by a bound dev lane) emit copy-pasteable `reassign --lane … --force` recipes. Bare-metal recipes still tell users to stop their own processes outside devlane before `reassign --force` / `prepare`, because plain `reassign` would no-op once the old port is free
- `docs/65-host-catalog.md` collision recovery section and `docs/80-agent-playbook.md` conflict-handling section updated to use the new commands
- Windows catalog-lock error message upgraded to point at the deferred-roadmap entry (no behavioral change to locking)
- `up` continues to fail before printing or running anything when an adapter declares `runtime.compose_files` or `runtime.run.commands` and any declared port is still `allocated: false`. Pure ports-only bare-metal adapters without `runtime.run.commands` remain a no-op `up` path; gating those is deferred and intentionally not part of Phase 2

### Phase 2 is done when

- the operator command surface (`port`, `reassign`, `host status`, `host doctor`, `host gc`) is shipped, documented, and tested
- the catalog mutation primitive is the single contract every write-side host-state mutation uses
- collision messages name the new commands and ship copy-pasteable recipes for scenarios 2 and 3
- the agent playbook conflict-handling section describes the same flow operators follow
- the catalog remains the sole durable authority for assigned ports, while `inspect --json` remains the fresh read surface and `.devlane/manifest.json` remains only a snapshot

## Phase 3 — worktree lifecycle (final phase)

Goal: let the shared tool create and retire lanes, not only operate inside them. This is the last planned phase.

Deliverables:

- `devlane worktree create <lane>` — supported only when the active adapter lives at the Git worktree root (`adapterRoot == repoRoot`). Subtree adapters in monorepos are out of scope for Phase 3 and fail clearly rather than guessing how a sibling worktree should map back to a nested adapter. When supported, the command does `git worktree add` + seed copy + `prepare` in the new checkout. It uses the sibling path convention `<repo-root-parent>/<repo-root-base>-<lane-slug>`, creates a new branch named raw `<lane>` from the current `HEAD`, fails rather than guessing if that branch or path already exists, requires `<lane>` to be a valid new local Git branch name, requires `<lane>` to slugify to a non-empty `<lane-slug>`, rejects `<lane>` equal to the adapter's `stable_name`, and rejects distinct raw lane names that would collide on the same `<lane-slug>`. Registers the new dev lane's ports in the catalog before the user starts anything.
- `devlane worktree remove <lane>` — supported only when the active adapter lives at the Git worktree root (`adapterRoot == repoRoot`). Subtree adapters in monorepos are out of scope for Phase 3. When supported, the command does `git worktree remove` + dedicated scoped catalog cleanup so the catalog self-cleans. By default `<lane>` resolves to the conventional sibling path `<repo-root-parent>/<repo-root-base>-<lane-slug>`. If that path is missing or the user has renamed the worktree manually, the command fails rather than guessing and requires `--path <worktree>` for an explicit target. Scoped cleanup removes only the removed worktree's `(app, repoPath)` allocations; it is not a host-wide `host gc`. Capture `app` and `repoPath` from the target checkout before removal so cleanup still has a stable key after the directory is gone.
- adapter `worktree.seed` — explicit list of paths (files and directories) copied from the source checkout into a new worktree before `prepare`. No defaults. Paths that also appear in `outputs.generated` are skipped with a notice. Missing source files warn and continue. Symlinks are recreated as symlinks (never dereferenced or rewritten). Regular-file mode bits are preserved best-effort; ownership is not. Existing destination paths are overwritten from the source checkout for explicit seed entries. Seed paths are resolved relative to `adapterRoot`, absolute paths are rejected, normalized paths may not escape `repoRoot`, and copy destinations must remain inside the target worktree root. The full list of copied paths is printed on completion for security clarity.
- non-happy-path worktree semantics: define what is rolled back, what is left in place, and what remediation is printed when `git worktree add` succeeds but seed copy or `prepare` fails, or when `worktree remove` cannot complete both git removal and scoped catalog cleanup in one pass

### Phase 3 is done when

- lane validation rules are explicit enough that branch naming, slug generation, and sibling-path collisions are deterministic before any git mutation happens. The slug algorithm itself is documented as contract surface rather than inferred from implementation
- `worktree create` and `worktree remove` have a documented partial-failure matrix that matches implementation and tests
- the command boundary for subtree adapters in monorepos is explicit: in-place commands work there, but Phase 3 worktree lifecycle does not
- the command output always leaves the user with one deterministic next step rather than silent partial state

Explicitly **not** in this phase:

- `worktree list` (redundant with `git worktree list` + `devlane host status`)
- per-worktree git config wiring (users configure their own git)
- any default seed list, glob-based seeding, or magic credential discovery

## Cut from the roadmap

The following phases were considered and cut. They will not be re-opened without a revision to `docs/00-principles.md`:

- **Proxy integration.** Devlane emits `publicHost` / `publicUrl` in the manifest when `lane.host_patterns` is declared. The user's proxy config (Caddyfile, Traefik labels, `/etc/hosts`) reads those values. Devlane does not talk to the proxy directly — coupling devlane to every proxy's API is the opposite of keeping the core repo-agnostic.
- **Stable deploy policy.** Deploy hooks, rollback hooks, global wrapper installers, and lane cutover helpers are per-product concerns. They belong in each repo's deploy scripts, not in a lane-metadata tool. See non-negotiable #11.

## Deep roadmap (not yet scheduled)

Further out, still worth capturing:

- UDP port allocation in the host catalog (currently TCP-only)
- Windows support for catalog concurrency (Phase 2 is POSIX-first via `fcntl.flock`; Windows needs `msvcrt.locking` equivalent)
- `devlane up --wait` with health-probe integration (once Phase 2 adds `ports.<service>.healthUrl` to the manifest, devlane may still choose to avoid probing it by default)
- Smarter `init` assistance around proxy signals (Traefik labels, Caddyfile, etc.) — deferred, and limited to suggestion-only scaffolding. The adapter still declares `lane.host_patterns` explicitly; detection must never silently infer hostname ownership into the contract.

Phase 1 `init` should stay deterministic even before that future work lands: scan descendants in lexical order, skip common non-app trees (`.git/`, `.devlane/`, `.direnv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, `tmp/`), do not follow symlinks, and fail rather than guessing when monorepo mode meets a non-interactive caller without `--all` or `--app`.

When exactly one candidate is found below `cwd`, `init` should scaffold in that subtree and print the selected path explicitly. Monorepo mode is reserved for multiple candidates, not for the single-descendant case.

## Design pressure to resist

The risk is turning the shared tool into an application framework. Non-negotiable #11 and `docs/00-principles.md` principle #6 exist to hold this line.

Keep the tool focused on:

- lane metadata
- lane lifecycle (create/remove, prepare, supervised up/down)
- orchestration of generated files
- host-wide coordination of ports and lanes

Do not let it become the place where product logic lives.
