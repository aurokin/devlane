# Example integrations

This kit includes three example adapters at increasing realism.

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

## 3. `examples/wowhead_cli/`

This example is shaped like a CLI-heavy repo with stable wrappers and branch-local editable environments.

It demonstrates:

- stable-vs-dev ownership boundaries
- a generated shell activation file
- XDG and runtime roots
- optional sidecar usage for cache services

## How to use these examples

- Start with `minimal-web` if you are implementing core behavior.
- Use `agentchat` when adapting a web app with generated `.env.local` and config files.
- Use `wowhead_cli` when adapting a CLI repo with stable wrappers and local activation scripts.

The examples are deliberately illustrative. They should guide design and implementation, not lock you into one product's variable names.
