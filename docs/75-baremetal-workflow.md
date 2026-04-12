# Bare-metal workflow

This is the **default runtime pattern** in `devlane`. It is the shape for repos whose services run directly on the host — no containers, no shared ingress proxy, just processes binding real host ports.

The opt-in alternative is containerized: see `70-container-workflow.md`. The two patterns can coexist on the same machine.

## What this pattern covers

- apps that are not containerized, or where the container is optional
- native dev servers (`npm run dev`, `cargo run`, `python manage.py runserver`) as the primary way the repo is run
- users who want to attach debuggers, hot-reloaders, or profilers directly to the process
- toolchains that expect fixed-port conventions (a framework that hard-codes `localhost:3000` in generated code, for example)

## Baseline pattern

1. the adapter declares every port the repo's services will bind via `ports`
2. the host catalog allocates real port numbers and remembers them across runs
3. generated templates render the allocated ports into whatever config the app actually reads
4. humans and agents discover ports via the manifest, not by remembering numbers

The catalog is what makes this work across projects. Without it, every repo would fight over `3000`.

## What the adapter declares

```yaml
ports:
  - name: web
    default: 3000
  - name: api
    default: 4000
  - name: ws
    default: 4001
```

Each entry names a service and a preferred port. The default is tried first; if unavailable the catalog walks `port_range` to find something free. See `65-host-catalog.md` for the full allocation model.

## What `prepare` produces

- `manifest.ports.web`, `manifest.ports.api`, etc. — integers, the resolved ports
- `DEVLANE_PORT_WEB=3100`, `DEVLANE_PORT_API=4000` in `.devlane/compose.env` — strings, same values
- any template can reference `{{ports.web}}` to render a real number into generated config

## Typical template

A framework like Next.js reads `PORT` from `.env.local`. The adapter generates that file from a template:

```
# templates/web.env.tmpl
PORT={{ports.web}}
NEXT_PUBLIC_API_URL=http://localhost:{{ports.api}}
```

And declares it as a generated output:

```yaml
outputs:
  generated:
    - template: "templates/web.env.tmpl"
      destination: ".env.local"
```

Now `devlane prepare` produces a `.env.local` whose port numbers are coordinated with every other `devlane`-managed repo on the host.

## Agent workflow

1. `devlane inspect --json` — read the lane
2. `devlane prepare` — allocate ports, render templates
3. start the service (the app reads its port from the rendered config)
4. on conflict: check the manifest first, probe, then `reassign` only if needed (see `80-agent-playbook.md`)

The agent never hard-codes port numbers. It reads them from the manifest every time.

## Stable versus dev in the bare-metal pattern

Stable typically gets the default ports because it is the canonical lane and usually prepared first. Dev lanes for the same app land on the next available ports in `port_range`.

That asymmetry is intentional. Stable owns the "friendly" numbers the same way it owns friendly hostnames. External tools that cache a stable URL with a stable port keep working across lane churn.

## Rule of thumb

For containerized web apps, prefer **hostname discovery** via `70-container-workflow.md`.

For bare-metal apps, prefer **catalog-allocated ports** via this pattern.

For mixed repos, use both. Declare `ports` for the host-port services and let the container pattern handle the rest.
