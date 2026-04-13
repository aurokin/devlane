# Bare-metal workflow

This is the **default runtime pattern** in `devlane`. It is the shape for repos whose services run directly on the host — no containers, no shared ingress proxy, just processes binding real host ports.

The opt-in alternative is containerized: see `70-container-workflow.md`. The two patterns can coexist on the same machine, and the same adapter can declare both (hybrid mode).

## Why `devlane up` never spawns bare processes

Nothing supervises a bare `bin/rails server` process. If devlane fire-and-forgot it, there would be no log collection, no restart on crash, no `ps` command to check whether it's running, and no clean way to stop it. Reimplementing any of that would turn devlane into a process manager — a place this tool explicitly does not go. See principle #1 in `00-principles.md`.

So `devlane up` **prints** the commands declared in `runtime.run.commands` and exits. The user (or agent) runs them in whatever shell, pane, or tab they prefer. Devlane stays out of the process lifecycle.

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
    health_path: /healthz
  - name: api
    default: 4000
  - name: ws
    default: 4001
```

Each entry names a service and a preferred port. `health_path` is optional and, when present, causes the manifest to emit a `healthUrl` for that service.

The allocation model distinguishes stable lanes from dev lanes:

- **Stable** lanes treat their declared `default` (or `stable_port`) as a fixture. `prepare` either claims the fixture or fails loudly. Stable ports are reserved in the catalog once stable has been prepared, and they survive `down`, reboots, and long periods of inactivity.
- **Dev** lanes allocate from the pool (`port_range` in `config.yaml`), skipping anything in `reserved` and anything already held in the catalog.

See `65-host-catalog.md` for the full allocation model, collision scenarios, and resolution commands.

## Optional: `runtime.run` for `devlane up` guidance

`devlane up` is a no-op for bare-metal unless the adapter declares `runtime.run.commands`. When declared, `up` prints the rendered commands and exits.

```yaml
runtime:
  run:
    commands:
      - name: web
        description: "Start the Rails server"
        command: "bin/rails server -p {{ports.web}}"
      - name: worker
        command: "bin/sidekiq"
```

### `devlane up` output

```
$ devlane up
Bare-metal commands for lane "feature-x":

  # Start the Rails server
  bin/rails server -p 3100

  bin/sidekiq
```

The user copies these into terminal tabs, or pipes them through `sh` if they want to, or hands them to a process supervisor they already use. Devlane does not run them.

### `devlane down` for bare-metal

Always a no-op. Devlane does not track or stop bare-metal processes. The catalog entry and manifest persist across stop/start cycles exactly as they do for containerized lanes.

### Template scope

`runtime.run.commands[].command` renders with the same variable scope as `outputs.generated` templates: `ports.<name>`, `lane.*`, `app`, `runtime.env.*`. New variables are added to both scopes together.

## What `prepare` produces

- `manifest.ports.web.port`, `manifest.ports.api.port`, etc. — integers, the resolved ports
- `manifest.ports.web.healthUrl` when `health_path` is declared on that port
- `manifest.ready: true` once every declared port has an allocation
- `DEVLANE_PORT_WEB=3100`, `DEVLANE_PORT_API=4000` in `.devlane/compose.env` when compose is also in use (otherwise compose env is omitted)
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

1. `devlane inspect --json` — read the lane (works even before `prepare`; check `ready` before relying on port numbers)
2. `devlane prepare` — allocate ports, render templates
3. start the service (the app reads its port from the rendered config, or the agent runs `devlane up` to print the suggested commands)
4. on conflict: check the manifest first, probe, then `reassign` only if needed (see `80-agent-playbook.md`)

The agent never hard-codes port numbers. It reads them from the manifest every time.

## Stable versus dev in the bare-metal pattern

Stable treats its declared `default` (or `stable_port`) as a fixture. Dev lanes for the same app land on the next available ports in `port_range`.

This is not a convention — it is enforced at `prepare` time. Stable either gets its fixture port or `prepare` fails with a message telling the user how to resolve. External tools and wrappers that cache a stable port keep working across dev-lane churn because the fixture is a promise the tool upholds.

## Hybrid: compose sidecars plus bare-metal dev server

Adapters can declare both `compose_files` and `runtime.run.commands`. In that case, `devlane up` prints the bare-metal commands first and then runs `docker compose up` for the supervised services. If compose fails, the bare-metal plan is still visible above the error. See `70-container-workflow.md` for the hybrid details.

## Rule of thumb

Bare-metal ports via the catalog are first-class. If the host also runs an ingress proxy (Caddy, Traefik), the adapter can declare `host_patterns` and expose `publicHost` / `publicUrl` in the manifest — hostnames are orthogonal to the bare-metal pattern, not exclusive to containers.

For mixed repos, declare `ports` for the host-port services and add `compose_files` for the containerized ones; the manifest carries both.

Always read the manifest. Never guess.
