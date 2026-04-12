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
3. `docs/30-quickstart.md`

Then branch by task:

- **Core CLI or manifest work:** `docs/40-cli-contract.md`, `docs/50-adapter-schema.md`, `docs/60-manifest-contract.md`, then `src/devlane/`
- **Port or host catalog work:** `docs/65-host-catalog.md`, then `docs/80-agent-playbook.md` for the conflict-handling protocol
- **Runtime patterns (containerized or bare-metal):** `docs/70-container-workflow.md` and `docs/75-baremetal-workflow.md`, then the matching examples under `examples/`
- **Repo adoption work:** `docs/90-example-integrations.md`, then `examples/agentchat/` or `examples/wowhead_cli/`
- **Planning / acceptance work:** `docs/100-implementation-plan.md` and `docs/110-acceptance-checklist.md`
- **Prompt handoff work:** `prompts/README.md`

## Non-negotiables

These rules are the design center. Do not casually violate them.

1. **Shared tool owns lifecycle.** Per-repo files stay declarative.
2. **Stable owns global names.** Stable may own friendly hostnames, global wrappers, or global service names. Dev lanes do not silently take them.
3. **`inspect --json` is the source of truth for agents.** Agents should not scrape ad hoc env files when a manifest exists.
4. **Generated files are tool-owned.** Repos may read generated files, but humans and agents should avoid manual edits except for explicit adoption flows.
5. **Bare-metal is the default runtime pattern.** Most apps bind host ports directly and use the host catalog for coordinated allocation. Containerized apps are an opt-in alternative: declare `compose_files` to use them. For HTTP apps behind an ingress proxy, hostname discovery is preferred; for everything else, port-based discovery via `manifest.ports.<service>.port` on localhost is first-class. Hostnames are optional and orthogonal to the runtime pattern — a bare-metal app with Caddy can have hostnames, a containerized app with plain port-publish can skip them. The pattern is signaled declaratively by the adapter (presence of `ports`, `compose_files`, `host_patterns`, `runtime.run`) rather than inferred from the environment.
6. **Compose project names include the lane slug.** That is the baseline container namespace.
7. **Keep core repo-agnostic.** App-specific env var names, wrapper names, and product rules live in adapters and examples, not in the core library.
8. **Prefer additive, machine-readable contracts.** If the behavior changes, update docs, schemas, examples, and tests together.
9. **The host catalog is tool-owned.** `~/.config/devlane/catalog.json` is written by the tool and read by everyone else. Humans and agents should not hand-edit it. User configuration lives in `~/.config/devlane/config.yaml`, which the tool only reads.
10. **Port allocations are sticky.** Once a `(app, lane, service)` tuple has a port, it does not move except via explicit `reassign` or `gc`. Do not introduce code paths that re-probe existing allocations silently.

## Working style

- Start from the acceptance checklist before adding features.
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
6. Add worktree lifecycle support only after the manifest contract and host catalog are stable.
7. Add proxy integration after lane naming, compose env generation, and the catalog are stable.

## Commands

From the repo root:

```bash
python -m venv .venv
source .venv/bin/activate
pip install -e '.[dev]'
pytest
python -m devlane inspect --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --json
python -m devlane prepare --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web
```

## What to avoid

- Do not bake product-specific stable/worktree variable names into the core.
- Do not make agents guess ports when a hostname or manifest can be authoritative.
- Do not require every repo to reimplement orchestration logic.
- Do not let dev lanes seize global wrappers or global hostnames by default.
