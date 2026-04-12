# AGENTS.md

Read this file first. It is the entrypoint for coding agents working on this kit.

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
- **Container or reverse-proxy work:** `docs/70-container-workflow.md`, then `examples/minimal-web/` and `examples/agentchat/`
- **Repo adoption work:** `docs/90-example-integrations.md`, then `examples/agentchat/` or `examples/wowhead_cli/`
- **Planning / acceptance work:** `docs/100-implementation-plan.md` and `docs/110-acceptance-checklist.md`
- **Prompt handoff work:** `prompts/README.md`

## Non-negotiables

These rules are the design center. Do not casually violate them.

1. **Shared tool owns lifecycle.** Per-repo files stay declarative.
2. **Stable owns global names.** Stable may own friendly hostnames, global wrappers, or global service names. Dev lanes do not silently take them.
3. **`inspect --json` is the source of truth for agents.** Agents should not scrape ad hoc env files when a manifest exists.
4. **Generated files are tool-owned.** Repos may read generated files, but humans and agents should avoid manual edits except for explicit adoption flows.
5. **Only ingress binds host ports for HTTP apps.** Internal services should communicate by Compose service name on the lane network.
6. **Compose project names include the lane slug.** That is the baseline container namespace.
7. **Keep core repo-agnostic.** App-specific env var names, wrapper names, and product rules live in adapters and examples, not in the core library.
8. **Prefer additive, machine-readable contracts.** If the behavior changes, update docs, schemas, examples, and tests together.

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
3. Improve Compose lifecycle support.
4. Add worktree lifecycle support only after the manifest contract is stable.
5. Add proxy integration after lane naming and compose env generation are stable.

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
