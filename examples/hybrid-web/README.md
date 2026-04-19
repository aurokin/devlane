# hybrid-web example

Demonstrates the **hybrid pattern**: a bare-metal web server with a
compose-managed sidecar. Common shape for Rails/Django/Node apps where the dev
server runs natively (debugger, hot reload) and a supporting service like Redis
or Postgres runs in a container.

`kind: hybrid` and the adapter declares both:

- `runtime.compose_files` — the supervised substrate. `devlane up` runs
  `docker compose up` here.
- `runtime.run.commands` — unsupervised processes. `devlane up` **prints**
  these; it never spawns them. See principle #1 in `docs/00-principles.md`.

The host catalog allocates a port for every `ports[]` entry, so the
bare-metal `web` process and the containerized `redis` sidecar both get
coordinated port numbers.

In this example, the printed Rails and Sidekiq commands render `{{ports.*}}`,
`.env.local` gets the same values, and Compose publishes Redis through
`${DEVLANE_PORT_REDIS}` from `.devlane/compose.env`. Concurrent lanes therefore
do not share fixed host ports.

## Try it

```bash
go run ./cmd/devlane inspect --config examples/hybrid-web/devlane.yaml --cwd examples/hybrid-web --json
go run ./cmd/devlane prepare --config examples/hybrid-web/devlane.yaml --cwd examples/hybrid-web
go run ./cmd/devlane up --config examples/hybrid-web/devlane.yaml --cwd examples/hybrid-web --dry-run
```

`up` (without `--dry-run`) prints the bare-metal commands first, then runs
`docker compose up` for the Redis sidecar. If compose fails, the printed
commands stay visible above the error. Exit code follows compose.

`down` stops the compose side only. The bare-metal processes are the user's
to stop.
