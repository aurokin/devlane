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

The CLI is small but covers the full lane lifecycle and host-catalog model:

- reads a declarative `devlane.yaml`
- derives a lane manifest from the current checkout
- computes host-catalog-backed `ready` plus `ports.<service> = {port, allocated, healthUrl}` when the adapter declares `ports`
- writes `.devlane/manifest.json`
- writes `.devlane/compose.env` (when `compose_files` is declared)
- renders repo-local generated files from templates
- projects `DEVLANE_PORT_*` into generated env when ports have been allocated
- allocates sticky per-lane ports during `prepare`, with stable fixtures treated strictly
- repairs allocations explicitly with `devlane reassign`, and inspects/cleans the host catalog with `devlane host status` / `host doctor` / `host gc` (backed by catalog drift detection)
- creates and retires dev-lane worktrees with `devlane worktree create` / `worktree remove`, copying `worktree.seed` paths and registering the new lane's ports
- builds lane-aware `docker compose` commands for containerized adapters; prints (never runs) bare-metal commands from `runtime.run.commands`
- exposes `init`, `inspect`, `prepare`, `port`, `up`, `down`, `status`, `doctor`, `host` (`status` / `doctor` / `gc`), `reassign`, and `worktree` (`create` / `remove`)

The host catalog itself lives under the OS user config directory: `os.UserConfigDir()/devlane`, with an explicit `XDG_CONFIG_HOME` taking precedence when set. In practice that is typically `~/.config/devlane` on Linux and `~/Library/Application Support/devlane` on macOS.

## What is not implemented yet

Phases 1–3 are shipped. The remaining surface is unscheduled "deep roadmap" work:

- UDP port allocation (the catalog is TCP-only today)
- Windows support for catalog concurrency (the lock is a stub on non-Unix platforms)
- `devlane up --wait` with health-probe integration (the manifest already emits `healthUrl`; nothing consumes it yet)
- smarter `init` assistance around proxy signals (Traefik labels, Caddyfile, etc.), suggestion-only

`docs/` describes current behavior; planning detail lives under `plans/`.

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

Each doc carries a one-line tier + "read this when" header. Open only as deep as your task needs.

**Orientation** — understand the model:

- `docs/README.md` — the reading map / task router
- `docs/00-principles.md` — the design rules that govern every other choice in the tool
- `docs/10-when-to-use-this.md` — whether devlane is the right fit for your setup
- `docs/20-concepts.md` — lane, stable vs dev, runtime patterns, adapter, manifest, host catalog, drift
- `docs/15-tech-stack.md` — implementation language and tooling (situational: contributing to devlane)
- `docs/30-quickstart.md` — fastest path to a first success (situational: first run)

**Reference contracts** — open on demand:

- `docs/40-cli-contract.md` — what the shared tool owns (commands, flags, exit codes)
- `docs/50-adapter-schema.md` — what each repo declares
- `docs/60-manifest-contract.md` — what agents consume
- `docs/65-host-catalog.md` — host-wide port, catalog, and drift model

**Task playbooks** — open for a worked sequence:

- `docs/70-container-workflow.md` — containerized pattern
- `docs/75-baremetal-workflow.md` — bare-metal pattern
- `docs/80-agent-playbook.md` — how agents drive the tool (discovery, conflict handling)
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

The lane lifecycle, host catalog, operator commands, and worktree lifecycle have all shipped. Good next steps:

1. adopt `devlane.yaml` in one repo (`devlane init`)
2. make `inspect --json` authoritative for that repo's agents
3. make `prepare` generate the files that repo currently hand-manages
4. make `up` and `down` lane-aware via Compose project names
5. exercise `devlane worktree create` for parallel dev lanes, and `host doctor` / `host gc` to keep the catalog clean
6. pick up the deep-roadmap items above (UDP, Windows locking, `up --wait`, proxy-signal hints) as the need arises

## Why this is docs-first

This is meant to be handed to coding agents. The docs are not decoration; they are the control surface.

The design goal is that an agent can start at `AGENTS.md`, choose the right depth of detail, and change the system without rediscovering the architecture from scratch.
