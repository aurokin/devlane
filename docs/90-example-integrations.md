# Example integrations

> **Playbook** · Read this when you're picking an example adapter to model your repo on. Onward: `50-adapter-schema.md`, then the chosen adapter under `examples/`.

This kit includes four example adapters at increasing realism.

## 1. `examples/minimal-web/`

The smallest useful example.

Use this when you want to understand the lane contract without product-specific noise.

It demonstrates:

- lane naming
- hostname derivation
- Compose project naming
- generated `.devlane/compose.env`
- one simple generated env file

## 2. `examples/agentchat/`

This example is shaped like a multi-service web app with generated local files for web and server runtimes.

It demonstrates:

- multiple generated outputs
- service-oriented Compose usage
- proxy label patterns
- checkout-local state roots and URLs

## 3. `examples/hybrid-web/`

This example demonstrates the **hybrid pattern**: a bare-metal web server plus a compose-managed Redis sidecar. It is the right reference when the repo's primary dev server runs natively (for debugger and hot-reload ergonomics) but depends on a supporting service that is easier to run in a container.

It demonstrates:

- `kind: hybrid` with both `runtime.compose_files` and `runtime.run.commands`
- `devlane up` printing the bare-metal commands first, then running `docker compose up`
- host-catalog-backed ports with `pool_hint` subranges, rendered into the bare-metal commands and templates via `{{ports.web}}` / `{{ports.redis}}`
- a compose sidecar that stays lane-aware through both the compose project name and its catalog-allocated host port

## 4. `examples/wowhead_cli/`

This example is shaped like a CLI-heavy repo with stable wrappers and branch-local editable environments.

It demonstrates:

- stable-vs-dev ownership boundaries
- a generated shell activation file
- XDG and runtime roots
- optional sidecar usage for cache services

## How to use these examples

- Start with `minimal-web` if you are implementing core behavior.
- Use `agentchat` when adapting a web app with generated `.env.local` and config files.
- Use `hybrid-web` when adapting a repo with a native dev server and a containerized sidecar (Rails + Redis, Django + Postgres, Node + anything).
- Use `wowhead_cli` when adapting a CLI repo with stable wrappers and local activation scripts.

The examples are deliberately illustrative. They should guide design and implementation, not lock you into one product's variable names.
