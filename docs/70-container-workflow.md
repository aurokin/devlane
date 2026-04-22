# Container workflow

This is the **opt-in containerized pattern** in `devlane`. Repos use it by declaring `compose_files` in the adapter.

The default pattern is bare-metal: see `75-baremetal-workflow.md`. The two patterns can coexist on the same machine, and the same adapter can declare both for hybrid mode.

## Why `devlane up` runs compose for you

Compose is a supervisor: it owns PIDs, handles logs, supports `ps` / `logs` / `restart`, and survives your shell exiting. Running `docker compose up` for you is safe because the substrate already does the supervision work.

By contrast, devlane **prints** bare-metal commands declared under `runtime.run.commands` and never spawns them. If an adapter declares both, see "Hybrid pattern" below.

## Baseline pattern

1. each lane gets its own Compose project name
2. services talk to each other by Compose service name on the lane network
3. only ports the app actually needs on the host are bound
4. hostname-based discovery is available when the adapter declares `host_patterns`

When the host has an ingress proxy and a mechanism for `*.localhost` resolution, the adapter can declare `host_patterns` and lanes become reachable by name like `feature-x.agentchat.localhost`.

When the host has no proxy, containerized adapters still work. They just bind the necessary ports on the host and rely on port-based discovery via the manifest.

## Lifecycle commands

- `devlane up` runs `docker compose -p <project> -f <files> --env-file .devlane/compose.env --profile <profiles> up`
- `devlane down` runs `docker compose -p <project> ... down`
- `devlane status` runs `docker compose ps`
- `devlane up --dry-run` prints the command instead of running it
- if the adapter declares `ports` and any declared service is still `allocated: false`, `up` fails before running compose and points the caller at `prepare`
- compose-backed `up` also verifies the current prepare-owned inputs and fails clearly if `.devlane/compose.env` or declared generated outputs are stale

## When this pattern declares ports

Declare `ports` whenever a containerized service needs to bind a host port. Common cases:

- no ingress proxy is in use, and the app publishes its HTTP port directly to the host
- a database or other service needs to be reachable from outside the compose network
- a non-HTTP service needs a host port

If the host has an ingress proxy and only the proxy binds ports, the adapter typically omits `ports` entirely.

## Hostnames are optional and orthogonal

Hostnames are declarative. The adapter declares `host_patterns` when the user has a proxy or DNS mechanism that can resolve them. Devlane does not inspect proxy config or talk to proxy APIs directly.

Bare-metal apps can also have hostnames. Containerized apps can omit them. The runtime pattern and the discovery surface are independent.

## What `devlane` generates

When the compose pattern is in use, `prepare` writes `.devlane/compose.env` with:

- `DEVLANE_COMPOSE_PROJECT`
- `DEVLANE_PUBLIC_HOST`
- `DEVLANE_PUBLIC_URL`
- `DEVLANE_PORT_<NAME>` for allocated declared ports
- any entries from `runtime.env`

Compose reads this file via `env_file` or the `--env-file` flag.

## Recommended Compose pattern

- keep app services on fixed container ports
- do not publish those ports to the host unless needed
- use proxy labels or proxy config that reference `DEVLANE_PUBLIC_HOST`
- use the lane-specific Compose project name to isolate networks and service names

## Minimal Traefik-style example

```yaml
services:
  web:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.${DEVLANE_COMPOSE_PROJECT}-web.rule=Host(`${DEVLANE_PUBLIC_HOST}`)"
      - "traefik.http.services.${DEVLANE_COMPOSE_PROJECT}-web.loadbalancer.server.port=3000"
```

This is env projection, not proxy integration. Compose substitutes the variables; the proxy consumes them.

## Hybrid pattern

An adapter can declare **both** `compose_files` and `runtime.run.commands`. Common shapes:

- a Rails app that runs natively but depends on a Redis sidecar in compose
- a Node app with a Postgres container and a native dev server
- a CLI tool with an optional cache-service sidecar

`devlane up` behavior in hybrid mode:

1. Print the rendered `runtime.run.commands` first.
2. Run `docker compose up` for the supervised services.

If the adapter declares `ports`, the same allocation gate applies here: `up` fails before printing commands or running compose while any declared service is still `allocated: false`.

`devlane down` in hybrid mode runs `docker compose down`. The bare-metal processes are the user's to stop.

## Rule of thumb

Read the manifest first. If the adapter declares `host_patterns` and the host resolves them, use `publicHost` / `publicUrl`. Otherwise, use `manifest.ports.<service>.port` on localhost. Never guess.
