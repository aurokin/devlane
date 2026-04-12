# Adapter schema

Each repo contributes a `devlane.yaml`.

The adapter should stay small and declarative.

## Example

```yaml
schema: 1
app: demoapp
kind: web

lane:
  stable_name: stable
  stable_branches: [main, master]
  host_patterns:
    stable: "{app}.localhost"
    dev: "{lane}.{app}.localhost"
  project_pattern: "{app}_{lane}"
  path_roots:
    state: ".devlane/state"
    cache: ".devlane/cache"
    runtime: ".devlane/runtime"

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
  - name: api
    default: 4000

outputs:
  manifest_path: ".devlane/manifest.json"
  compose_env_path: ".devlane/compose.env"
  generated:
    - template: "templates/app.env.tmpl"
      destination: ".devlane/generated/app.env"

health:
  http_url: "http://{public_host}/health"
```

## Fields

### Top-level

- `schema` — adapter schema version
- `app` — stable app identifier
- `kind` — `web`, `cli`, or `hybrid`

### `lane`

- `stable_name` — canonical stable lane name
- `stable_branches` — branches that should default to stable mode
- `host_patterns` — format strings for stable and dev hostnames
- `project_pattern` — format string for the Compose project name
- `path_roots` — base directories for state, cache, and runtime roots

### `runtime`

- `compose_files` — Compose files relative to repo root
- `default_profiles` — profiles enabled by default
- `optional_profiles` — known optional profiles
- `env` — extra env values that should be available to templates and Compose

All four fields are optional. Pure bare-metal repos that do not use Docker Compose can omit `compose_files` and the profile fields; the default runtime pattern is bare-metal (see `75-baremetal-workflow.md`). Declaring `compose_files` is what opts an adapter into the containerized pattern (see `70-container-workflow.md`).

### `ports`

Optional. A list of named port needs, each with a preferred `default`.

- `name` — service identity, referenced from the manifest (`ports.<name>`) and env (`DEVLANE_PORT_<NAME>`)
- `default` — preferred port, tried first during allocation

The adapter declares what the app needs. The shared tool resolves real numbers via the host catalog. Once allocated, ports are sticky — they do not move unless `devlane reassign` or `devlane host gc` is run. See `65-host-catalog.md` for the allocation model.

If `ports` is omitted, no ports are allocated. This is appropriate for pure-CLI repos that do not bind host ports.

### `outputs`

- `manifest_path` — where to write the manifest
- `compose_env_path` — where to write the Compose env file
- `generated` — files rendered from templates

### `health`

- `http_url` — optional rendered HTTP health URL

## Design rule

If you find yourself adding repo-specific imperative behavior to the adapter, stop and ask whether it belongs in:

- core lifecycle logic, or
- a repo-owned wrapper outside the adapter

The adapter should describe, not orchestrate.
