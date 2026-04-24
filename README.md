# devlane agent kit

`devlane` is a docs-first starter kit for building a shared local-development control plane around **stable** and **dev** lanes.

It is designed for the case where you have many repos, many worktrees, some stable host-managed install or hostname, and a growing amount of parallel work performed by humans and coding agents.

The kit contains three things at once:

1. a **small Go CLI scaffold**
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

The scaffold is intentionally small but already includes part of the host-catalog model:

- reads a declarative `devlane.yaml`
- derives a lane manifest from the current checkout
- computes host-catalog-backed `ready` plus `ports.<service> = {port, allocated, healthUrl}` when the adapter declares `ports`
- writes `.devlane/manifest.json`
- writes `.devlane/compose.env` (when `compose_files` is declared)
- renders repo-local generated files from templates
- projects `DEVLANE_PORT_*` into generated env when ports have been allocated
- allocates sticky per-lane ports during `prepare`, with stable fixtures treated strictly
- builds lane-aware `docker compose` commands for containerized adapters; prints (never runs) bare-metal commands from `runtime.run.commands`
- exposes `init`, `inspect`, `prepare`, `up`, `down`, `status`, and `doctor`

The host catalog itself lives under the OS user config directory: `os.UserConfigDir()/devlane`, with an explicit `XDG_CONFIG_HOME` taking precedence when set. In practice that is typically `~/.config/devlane` on Linux and `~/Library/Application Support/devlane` on macOS.

## What is not implemented yet

The remaining unshipped surface is mostly operator and lifecycle work around the catalog:

- `devlane port <service>`
- `devlane reassign <service>`
- `devlane host status`, `host doctor`, `host gc`
- `devlane worktree create` / `worktree remove`

Those commands are not part of the shipped CLI. `docs/` describes current behavior; planning detail lives under `plans/`.

## Start here

- Humans: read `docs/README.md`
- Coding agents: read `AGENTS.md` first, then `docs/README.md`

## Quickstart

```bash
go mod download
go tool gofumpt -w .
go tool goimports -w ./cmd ./internal
go tool golangci-lint run
go tool gotestsum -- ./...
go run ./cmd/devlane inspect --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --mode dev --json
go run ./cmd/devlane prepare --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --mode dev
```

## Go Tooling

The scaffold uses Go's module-pinned tool directives, so formatting, import cleanup, linting, and test output come from `go tool` rather than ad hoc local installs:

```bash
go tool gofumpt -w .
go tool goimports -w ./cmd ./internal
go tool golangci-lint run
go tool gotestsum -- ./...
```

## Progressive disclosure map

- `docs/README.md` — the reading map
- `docs/00-principles.md` — the design rules that govern every other choice in the tool
- `docs/10-when-to-use-this.md` — whether devlane is the right fit for your setup
- `docs/15-tech-stack.md` — implementation language, tooling, and repository policy choices
- `docs/20-concepts.md` — lane, stable vs dev, runtime patterns, adapter, manifest, host catalog, generated outputs
- `docs/30-quickstart.md` — fastest path to a first success
- `docs/40-cli-contract.md` — what the shared tool owns
- `docs/50-adapter-schema.md` — what each repo declares
- `docs/60-manifest-contract.md` — what agents consume
- `docs/65-host-catalog.md` — host-wide port and lane coordination
- `docs/70-container-workflow.md` — containerized pattern
- `docs/75-baremetal-workflow.md` — bare-metal pattern
- `docs/90-example-integrations.md` — how this maps onto real repos
- `plans/README.md` — planning and acceptance artifacts outside the primary docs path

## Project layout

```text
devlane-agent-kit/
├── AGENTS.md
├── README.md
├── cmd/devlane/
├── docs/
├── examples/
├── internal/
├── plans/
├── prompts/
├── schemas/
└── .golangci.yml
```

## Suggested next milestone

Keep the remaining implementation work narrow and dependable:

1. adopt `devlane.yaml` in one repo
2. make `inspect --json` authoritative
3. make `prepare` generate the files that repo currently hand-manages
4. make `up` and `down` lane-aware via Compose project names
5. finish the remaining operator surface around the already-shipped host catalog (`port`, `reassign`, `host *`)
6. delay worktree create/remove until the manifest and host-catalog contracts feel stable

## Why this is docs-first

This is meant to be handed to coding agents. The docs are not decoration; they are the control surface.

The design goal is that an agent can start at `AGENTS.md`, choose the right depth of detail, and change the system without rediscovering the architecture from scratch.
