# devlane agent kit

`devlane` is a docs-first starter kit for building a shared local-development control plane around **stable** and **dev** lanes.

It is designed for the case where you have many repos, many worktrees, some stable host-managed install or hostname, and a growing amount of parallel work performed by humans and coding agents.

The kit contains three things at once:

1. a **small Python CLI scaffold**
2. a **progressive-disclosure documentation set**
3. **example adapters** for a minimal web app, `agentchat`, and `wowhead_cli`

## Is this for you?

Use devlane if you have **multiple agents working in parallel** on the same machine, or if you run many worktrees / many repos that keep fighting over the same host ports.

If you are a single developer with one repo and one worktree, devlane is likely more machinery than you need — reach for a lighter per-directory env tool and a small task runner instead. See `docs/10-when-to-use-this.md` for the full adoption gate.

## The core idea

Standardize a **lane contract**, not a universal pile of env var names.

Agents should think in terms of:

- `stable`
- `dev/<lane>`
- `inspect`
- `prepare`
- `up`
- `down`
- `status`

Repos can still generate whatever app-specific files they need, but those files should be derived from one shared manifest.

## What is already implemented in this kit

The scaffold is intentionally small but useful:

- reads a declarative `devlane.yaml`
- derives a lane manifest from the current checkout
- writes `.devlane/manifest.json`
- writes `.devlane/compose.env` (when `compose_files` is declared)
- renders repo-local generated files from templates
- builds lane-aware `docker compose` commands for containerized adapters; prints (never runs) bare-metal commands from `runtime.run.commands`
- exposes `inspect`, `prepare`, `up`, `down`, `status`, `doctor`, and `init`

## What Phase 2 adds

Phase 2 is the **host catalog** — a shared, tool-owned file at `~/.config/devlane/catalog.json` that coordinates host-port allocations across every `devlane`-managed repo on the machine. It adds:

- `ports` declarations in the adapter (with optional `health_path`)
- `ports` (as `{port, allocated, healthUrl?}` objects) and a top-level `ready` flag in the manifest, plus `DEVLANE_PORT_*` env
- sticky, per-lane allocation with stable ports treated as fixtures (strict-fail on collision — see `docs/65-host-catalog.md`)
- concurrent-safe catalog writes via `fcntl.flock` + atomic rename (POSIX-first)
- `devlane port <service>` with `--verbose` and `--probe` (TCP probing on both `0.0.0.0` and `::`)
- `devlane reassign <service>` (idempotent, scoped, supports `--lane`)
- `devlane host status`, `host doctor`, `host gc`

The docs and schemas describe the Phase 2 target state. See `docs/65-host-catalog.md` and `docs/100-implementation-plan.md`.

Phase 3 adds minimal worktree lifecycle (`create` + `remove`, with adapter-declared `worktree.seed` copying credentials). That is the last planned phase — devlane stops there rather than drifting into proxy integration or deploy mechanics. See `docs/00-principles.md` for why.

## Start here

- Humans: read `docs/README.md`
- Coding agents: read `AGENTS.md` first, then `docs/README.md`

## Quickstart

```bash
python -m venv .venv
source .venv/bin/activate
pip install -e '.[dev]'
pytest
python -m devlane inspect --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --json
python -m devlane prepare --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web
```

## Progressive disclosure map

- `docs/README.md` — the reading map
- `docs/00-principles.md` — the design rules that govern every other choice in the tool
- `docs/10-when-to-use-this.md` — whether devlane is the right fit for your setup
- `docs/20-concepts.md` — lane, stable vs dev, runtime patterns, adapter, manifest, host catalog, generated outputs
- `docs/30-quickstart.md` — fastest path to a first success
- `docs/40-cli-contract.md` — what the shared tool owns
- `docs/50-adapter-schema.md` — what each repo declares
- `docs/60-manifest-contract.md` — what agents consume
- `docs/65-host-catalog.md` — host-wide port and lane coordination
- `docs/70-container-workflow.md` — containerized pattern
- `docs/75-baremetal-workflow.md` — bare-metal pattern
- `docs/90-example-integrations.md` — how this maps onto real repos
- `docs/100-implementation-plan.md` — phased roadmap
- `docs/110-acceptance-checklist.md` — done criteria

## Project layout

```text
devlane-agent-kit/
├── AGENTS.md
├── README.md
├── docs/
├── examples/
├── prompts/
├── schemas/
├── src/devlane/
└── tests/
```

## Suggested first milestone

Keep phase 1 narrow and dependable:

1. adopt `devlane.yaml` in one repo
2. make `inspect --json` authoritative
3. make `prepare` generate the files that repo currently hand-manages
4. make `up` and `down` lane-aware via Compose project names
5. delay worktree create/remove until the manifest and adapter contracts feel stable

## Why this is docs-first

This is meant to be handed to coding agents. The docs are not decoration; they are the control surface.

The design goal is that an agent can start at `AGENTS.md`, choose the right depth of detail, and change the system without rediscovering the architecture from scratch.
