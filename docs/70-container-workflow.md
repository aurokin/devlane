# Container workflow

This is the **opt-in containerized pattern** in `devlane`. It is the recommended shape for repos whose services all run in containers and speak HTTP — declare `compose_files` in the adapter to use it.

The default pattern is bare-metal: see `75-baremetal-workflow.md`. The two patterns can coexist on the same machine — the host catalog keeps their ports from colliding.

## Baseline pattern

1. each lane gets its own Compose project name
2. services talk to each other by Compose service name on the lane network
3. only the ingress proxy binds host ports
4. humans and agents use hostnames, not random host ports

When this pattern is in use, the adapter typically does not declare `ports`. Host-port allocation is unnecessary because nothing other than the shared proxy listens on the host. If a containerized service *does* need to bind a host port (a database for external tools, for example), declare it in the adapter like any other port and the catalog will allocate it.

## Why this is simpler

Host ports are a weak discovery mechanism for parallel work because they force people and agents to remember which app is on which number.

A hostname like `feature-x.agentchat.localhost` is a much better discovery surface than “the frontend is on 31847 today”.

## What `devlane` should generate

The shared tool should generate enough information for Compose and the proxy to agree on:

- lane slug
- Compose project name
- public hostname
- public URL

In this scaffold, that data is written to `.devlane/compose.env`.

## Recommended Compose pattern

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

## CLI-heavy repos

CLI repos may still use the lane model even when they do not expose HTTP apps.

In that case, the lane contract is still useful for:

- stable wrapper ownership
- XDG roots
- cache isolation
- optional sidecars such as Redis
- shell activation scripts

## Rule of thumb

For containerized web apps, prefer **hostname discovery** via this pattern.

For bare-metal apps, prefer **catalog-allocated ports** via `75-baremetal-workflow.md`.

For CLI repos, prefer **wrapper or activation discovery**.

For all of the above, prefer one manifest over many hand-maintained conventions.
