# AGENTS.md

## Mission

Build **devlane**, a shared local-development control plane that lets humans and agents work on many repos with the same mental model:

- one **stable** lane owns global names
- many **dev** lanes can exist at the same time
- the shared tool owns lifecycle and machine-readable state
- each repo contributes only a small declarative adapter

The goal is to remove repo-specific guesswork around ports, worktrees, generated env files, Compose project names, hostnames, caches, and runtime roots.

## Read path

Read in this order unless you already know the area you are touching:

1. `README.md`
2. `docs/README.md`
3. `docs/00-principles.md`
4. `docs/10-when-to-use-this.md`
5. `docs/15-tech-stack.md`
6. `docs/30-quickstart.md`

Then branch by task:

- **Core CLI or manifest work:** `docs/40-cli-contract.md`, `docs/50-adapter-schema.md`, `docs/60-manifest-contract.md`, then `cmd/devlane/` and `internal/`
- **Port or host catalog work:** `docs/65-host-catalog.md`, then `docs/80-agent-playbook.md` for the conflict-handling protocol
- **Runtime patterns (containerized or bare-metal):** `docs/70-container-workflow.md` and `docs/75-baremetal-workflow.md`, then the matching examples under `examples/`
- **Repo adoption work:** `docs/90-example-integrations.md`, then `examples/agentchat/` or `examples/wowhead_cli/`
- **Planning / acceptance work:** `plans/README.md`, then `plans/phase-roadmap.md` and `plans/acceptance-checklist.md`
- **Prompt handoff work:** `prompts/README.md`

## Non-negotiables

These rules are the design center. Do not casually violate them. The full reasoning lives in `docs/00-principles.md`; this list is the agent-facing summary.

1. **Shared tool owns state and supervised substrates; users own unsupervised processes.** Devlane mutates its own state (catalog, manifest, generated files, compose env, worktree seed copies) and executes commands against supervised substrates (`docker compose up`). It prints commands for unsupervised processes (`runtime.run.commands`) and refuses to fire-and-forget them.
2. **Stable owns global names.** Stable may own friendly hostnames, global wrappers, or global service names. Dev lanes do not silently take them.
3. **`inspect --json` is the source of truth for agents.** Agents should not scrape ad hoc env files when a manifest exists. `.devlane/manifest.json` on disk is a snapshot; `inspect --json` is fresh.
4. **Generated files are tool-owned.** Repos may read generated files, but humans and agents should avoid manual edits except for explicit adoption flows.
5. **Bare-metal is the default runtime pattern.** Most apps bind host ports directly and use the host catalog for coordinated allocation. Containerized apps are an opt-in alternative: declare `compose_files` to use them. For HTTP apps behind an ingress proxy, hostname discovery is preferred; for everything else, port-based discovery via `manifest.ports.<service>.port` on localhost is first-class. Hostnames are optional and orthogonal to the runtime pattern. The pattern is signaled declaratively by the adapter (presence of `ports`, `compose_files`, `host_patterns`, `runtime.run`) rather than inferred from the environment.
6. **Compose project names include the lane slug.** That is the baseline container namespace.
7. **Keep core repo-agnostic.** App-specific env var names, wrapper names, and product rules live in adapters and examples, not in the core library.
8. **Prefer additive, machine-readable contracts.** If the behavior changes, update docs, schemas, examples, and tests together.
9. **The host catalog is tool-owned.** The catalog lives under `os.UserConfigDir()/devlane` (`~/.config/devlane` on Linux, `~/Library/Application Support/devlane` on macOS). Humans and agents should not hand-edit it. User configuration lives alongside it in `config.yaml`, which the tool only reads.
10. **Port allocations are sticky.** Once a `(app, repoPath, service)` allocation exists, it does not move during ordinary dev-lane churn except via explicit repair/cleanup flows. The stable-specific exception is reclaiming the current checkout's fixture when a same-checkout dev allocation would otherwise make stable report the wrong port. Do not introduce code paths that re-probe existing allocations silently.
11. **The tool does not become an application framework.** Proxy integration, deploy mechanics, process supervision, log collection, and credential management beyond the explicit `worktree.seed` copy are permanently out of scope. When a proposal feels useful but drifts into these territories, decline it and point at `docs/00-principles.md`.

## Working style

- Start from the current contract docs first, then use `plans/acceptance-checklist.md` when you need the active acceptance scope.
- Keep functions small and typed.
- Prefer deterministic outputs over clever inference.
- If you add a new adapter field, update:
  - `docs/50-adapter-schema.md`
  - `schemas/devlane.schema.json`
  - examples
  - tests
- If you add a new manifest field, update:
  - `docs/60-manifest-contract.md`
  - `schemas/manifest.schema.json`
  - tests
- If you change the host catalog shape, update:
  - `docs/65-host-catalog.md`
  - `schemas/catalog.schema.json`
  - tests
- If you change the user config shape, update:
  - `docs/65-host-catalog.md`
  - `schemas/config.schema.json`
  - tests

## Definition of done

A feature is done when:

- the contract is documented
- the implementation exists
- the example adapters still make sense
- the manifest stays stable and machine-readable
- tests cover the new behavior
- the change still respects the non-negotiables above

## Recommended implementation order

1. Keep `inspect` and `prepare` rock-solid.
2. Keep template rendering deterministic and easy to reason about.
3. Add `devlane init` as a zero-friction entry point for new adopters.
4. Improve Compose lifecycle support.
5. Land the host catalog and port allocation before anything that depends on cross-project coordination.
6. Add worktree lifecycle support (`create` + `remove`, with `worktree.seed` copying) only after the manifest contract and host catalog are stable.

Phase 4 (proxy integration) and Phase 5 (stable deploy) have been **cut from the roadmap** per non-negotiable #11. Do not propose them.

## Commands

From the repo root:

```bash
go mod download
go tool gofumpt -w .
go tool goimports -w ./cmd ./internal
go tool golangci-lint run
go tool gotestsum -- ./...
go run ./cmd/devlane inspect --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --mode dev --json
go run ./cmd/devlane prepare --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --mode dev
```

## What to avoid

- Do not bake product-specific stable/worktree variable names into the core.
- Do not make agents guess ports when a hostname or manifest can be authoritative.
- Do not require every repo to reimplement orchestration logic.
- Do not let dev lanes seize global wrappers or global hostnames by default.
- Do not propose features that belong in an application framework — see non-negotiable #11 and `docs/00-principles.md`.
