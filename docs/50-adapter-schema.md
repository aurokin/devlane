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
