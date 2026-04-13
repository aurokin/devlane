# Container workflow

This is the **opt-in containerized pattern** in `devlane`. It is the recommended shape for repos whose services all run in containers — declare `compose_files` in the adapter to use it.

The default pattern is bare-metal: see `75-baremetal-workflow.md`. The two patterns can coexist on the same machine — the host catalog keeps their ports from colliding — and the same adapter can declare both (hybrid mode).

## Why `devlane up` runs compose for you

Compose is a supervisor: it owns PIDs, handles logs, supports `ps` / `logs` / `restart`, and survives your shell exiting. Running `docker compose up` for you is safe because the substrate already does the supervision work. This is the supervised-substrate rule (principle #1 in `00-principles.md`).

By contrast, devlane **prints** bare-metal commands declared under `runtime.run.commands` and never spawns them. If an adapter declares both, see "Hybrid pattern" below.

## Baseline pattern

1. each lane gets its own Compose project name (derived from `lane.project_pattern`)
2. services talk to each other by Compose service name on the lane network
3. only ports the app actually needs on the host are bound (typically an ingress proxy, sometimes a database)
4. hostname-based discovery is available when the adapter declares `host_patterns`

When the host has an ingress proxy (Caddy, Traefik, nginx) and a mechanism for `*.localhost` resolution, the adapter can declare `host_patterns` and lanes become reachable by name like `feature-x.agentchat.localhost`. This is the "polished" container setup.

When the host has no proxy, containerized adapters still work — they just bind the necessary ports on the host (same as bare-metal) and rely on port-based discovery via the manifest.

## Lifecycle commands

- `devlane up` — runs `docker compose -p <project> -f <files> --env-file .devlane/compose.env --profile <profiles> up`. Compose does the supervision.
- `devlane down` — runs `docker compose -p <project> ... down`. Does **not** release catalog ports (see `65-host-catalog.md` on stickiness).
- `devlane status` — runs `docker compose ps`.
- `devlane up --dry-run` — prints the command instead of running it.

## When this pattern declares ports

Declare `ports` in the adapter whenever a containerized service needs to bind a host port. Common cases:

- no ingress proxy is in use, and the app publishes its HTTP port directly to the host
- a database or other service needs to be reachable from outside the compose network (a GUI tool, a migration script)
- a non-HTTP service needs a host port (a gRPC server, a WebSocket endpoint)

If the host has an ingress proxy and only the proxy binds ports, the adapter typically omits `ports` entirely. The proxy handles port binding at the compose layer.

## Hostnames are optional and orthogonal

Hostnames are declarative — the adapter declares `host_patterns` when the user has a proxy or DNS mechanism that can resolve them. Devlane does not sniff the filesystem for Caddyfiles or Traefik labels; the adapter is the source of truth.

Bare-metal apps can also have hostnames (Caddy reverse-proxying to localhost works fine). Containerized apps can omit hostnames (plain port-publish is fine too). The two axes are independent.

Devlane does not talk to the proxy directly, ever. It emits `DEVLANE_PUBLIC_HOST` and `DEVLANE_PUBLIC_URL`; the user's compose labels or external proxy config consume those values. Direct proxy integration is cut from the roadmap (see `100-implementation-plan.md`).

## What `devlane` generates

When the compose pattern is in use, `prepare` writes `.devlane/compose.env` with:

- `DEVLANE_COMPOSE_PROJECT` — the rendered project name
- `DEVLANE_PUBLIC_HOST` — the rendered hostname (when `host_patterns` is declared; absent otherwise)
- `DEVLANE_PUBLIC_URL` — the full URL (when `host_patterns` is declared; absent otherwise)
- `DEVLANE_PORT_<NAME>` — allocated ports for any declared `ports[]` services
- any entries from `runtime.env`

Compose reads this file via `env_file` or the `--env-file` flag to pick up the lane-specific values.

## Recommended Compose pattern (with proxy)

- keep app services on fixed container ports
- do **not** publish those ports to the host unless needed
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

This is not "proxy integration" in the Phase 4 sense — it is the env projection doing its job. Compose substitutes the variables, Traefik reads the labels, devlane never talks to Traefik.

## Hybrid pattern

An adapter can declare **both** `compose_files` and `runtime.run.commands`. Common shapes:

- a Rails app that runs natively but depends on a Redis sidecar in compose
- a Node app with a Postgres container and a native dev server
- a CLI tool with an optional cache-service sidecar

`devlane up` behavior in hybrid mode:

1. Print the rendered `runtime.run.commands` first (copy-pasteable, clearly labeled as "start these yourself").
2. Run `docker compose up` for the supervised services.

If compose fails, the bare-metal plan is still visible above the error — the user can fix compose, scroll up, paste the commands, and move on. Exit code follows compose.

`devlane down` in hybrid mode runs `docker compose down`. The bare-metal processes are the user's to stop.

## CLI-heavy repos

CLI repos may still use the lane model even when they do not expose HTTP apps.

In that case, the lane contract is still useful for:

- stable wrapper ownership
- XDG roots
- cache isolation
- optional sidecars such as Redis (via compose)
- shell activation scripts

## Rule of thumb

Read the manifest first. If the adapter declares `host_patterns` and you are behind an ingress proxy (Caddy, Traefik, `/etc/hosts`), use the rendered `publicHost` / `publicUrl`. Otherwise, use `manifest.ports.<service>.port` on localhost. Hostnames are orthogonal to runtime pattern: a bare-metal adapter can declare `host_patterns` if the host has DNS or a proxy resolving them, and a containerized adapter can skip them entirely.

For CLI repos, prefer **wrapper or activation discovery**.

Never guess. The manifest tells you which discovery mode the adapter chose.
