# Adapter schema

Each repo contributes a `devlane.yaml`.

The adapter should stay small and declarative.

## Scaffolding a new adapter

`devlane init` writes a starter `devlane.yaml` based on what it finds in the repo — compose files present (containerized), framework manifest without compose (bare-metal), or neither (CLI). Use `--template <name>` to force a shape, or `--from <path>` to copy an existing example. Most adoptions start here and then customize the result.

## Example

```yaml
schema: 1
app: demoapp
kind: web

lane:
  stable_name: stable
  stable_branches: [main, master]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"

  # host_patterns is optional. Uncomment if you have a proxy or DNS
  # resolving these hostnames (Caddy, Traefik, /etc/hosts, etc.).
  # host_patterns:
  #   stable: "{app}.localhost"
  #   dev: "{lane}.{app}.localhost"

runtime:
  compose_files:
    - compose.yaml
    - compose.devlane.yaml
  default_profiles: [web]
  optional_profiles: [db]
  env:
    APP_MODE: development

ports:
  - name: web
    default: 3000
    health_path: /healthz
  - name: api
    default: 4000

outputs:
  manifest_path: ".devlane/manifest.json"
  compose_env_path: ".devlane/compose.env"
  generated:
    - template: "templates/app.env.tmpl"
      destination: ".devlane/generated/app.env"
```

## Fields

### Top-level

- `schema` — adapter schema version
- `app` — stable app identifier
- `kind` — `web`, `cli`, or `hybrid`

### `lane`

- `stable_name` — canonical stable lane name
- `stable_branches` — branches that should default to stable mode
- `host_patterns` — optional. Format strings for stable and dev hostnames. Omit entirely to stay on port-based discovery (the common bare-metal case).
- `project_pattern` — format string for the Compose project name
- `path_roots` — base directories for state, cache, and runtime roots

When `host_patterns` is declared:

- `host_patterns.dev` must contain `{lane}` so dev lanes produce distinct hostnames. Enforced at schema load.
- `host_patterns.stable` and `host_patterns.dev` must render to different strings. Enforced at schema load.

When `host_patterns` is omitted:

- The manifest emits `network.publicHost: null` and `network.publicUrl: null`.
- Discovery is port-based via `manifest.ports.<name>.port` on localhost.

### `runtime`

- `compose_files` — Compose files relative to repo root
- `default_profiles` — profiles enabled by default
- `optional_profiles` — known optional profiles
- `env` — extra env values that should be available to templates and Compose
- `run` — optional bare-metal command declarations (see below)

All fields are optional. Pure bare-metal repos that do not use Docker Compose can omit `compose_files` and the profile fields; the default runtime pattern is bare-metal (see `75-baremetal-workflow.md`). Declaring `compose_files` is what opts an adapter into the containerized pattern (see `70-container-workflow.md`).

### `runtime.run`

Optional. Tells `devlane up` what to do on bare-metal. Without it, `up` is a no-op.

```yaml
runtime:
  run:
    mode: suggest   # suggest | execute   (default: suggest)
    commands:
      - name: web
        description: "Start the Rails API"
        command: "bin/rails server -p {{ports.web}}"
      - name: worker
        command: "bin/sidekiq"
```

- `mode: suggest` (default) — `devlane up` prints the rendered commands and exits. Safe to copy-paste. No process spawning.
- `mode: execute` — `devlane up` runs each command as a fire-and-forget child process. No supervision, no restart, no log collection. `devlane down` is still a no-op; users stop their own processes.

`devlane init` never scaffolds `mode: execute`. Users opt into execution consciously by editing the field.

Commands accept `{{...}}` templating. The scope is the same as `outputs.generated` templates: `ports.<name>`, `lane.*`, `app`, `runtime.env.*`. New variables are added to both scopes together.

### `ports`

Optional. A list of named port needs, each with a preferred `default` and an optional HTTP `health_path`.

- `name` — service identity, referenced from the manifest (`ports.<name>`) and env (`DEVLANE_PORT_<NAME>`)
- `default` — preferred port, tried first during allocation
- `health_path` — optional HTTP path. When declared, the manifest emits `ports.<name>.healthUrl` as `http://localhost:<port><health_path>`. Devlane itself does not probe this URL; it is for agents and tooling.

The adapter declares what the app needs. The shared tool resolves real numbers via the host catalog. Once allocated, ports are sticky — they do not move unless `devlane reassign` or `devlane host gc` is run. See `65-host-catalog.md` for the allocation model, including the fixture semantics that apply to stable lanes.

If `ports` is omitted, no ports are allocated. This is appropriate for pure-CLI repos that do not bind host ports.

### `outputs`

- `manifest_path` — where to write the manifest
- `compose_env_path` — where to write the Compose env file. Required when `runtime.compose_files` is declared; omit otherwise.
- `generated` — files rendered from templates. `destination` must resolve inside the repo root; absolute paths outside the repo are refused at prepare time.

Generated files are tool-owned. `prepare` tracks a sidecar hash under `.devlane/` for each generated destination. If the on-disk file has been hand-edited since the last `prepare`, the tool prints a one-line warning and writes anyway. On first `prepare` (no sidecar hash yet), existing files are quietly overwritten with a notice.

## Design rule

If you find yourself adding repo-specific imperative behavior to the adapter, stop and ask whether it belongs in:

- core lifecycle logic, or
- a repo-owned wrapper outside the adapter

The adapter should describe, not orchestrate.
