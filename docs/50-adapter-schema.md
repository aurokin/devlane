# Adapter schema

Each repo contributes a `devlane.yaml`.

The adapter should stay small and declarative. See principle #2 in `00-principles.md` — adapters describe, the tool orchestrates.

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
    stable_port: 3000
    pool_hint: [3100, 3199]
  - name: api
    default: 4000

reserved:
  - 5555

worktree:
  seed:
    - .env
    - .env.local
    - config/master.key

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

All fields are optional. Pure bare-metal repos that do not use Docker Compose can omit `compose_files` and the profile fields; the default runtime pattern is bare-metal (see `75-baremetal-workflow.md`). Declaring `compose_files` is what opts an adapter into the containerized pattern (see `70-container-workflow.md`). Declaring both gets the hybrid pattern.

### `runtime.run`

Optional. Declares bare-metal commands that `devlane up` should print.

```yaml
runtime:
  run:
    commands:
      - name: web
        description: "Start the Rails API"
        command: "bin/rails server -p {{ports.web}}"
      - name: worker
        command: "bin/sidekiq"
```

- `devlane up` **always prints** these commands and exits. Devlane never spawns bare-metal processes — nothing would supervise them. This is the supervised-substrate rule (principle #1).
- In a hybrid adapter (both `compose_files` and `runtime.run.commands`), `up` prints these commands first, then runs `docker compose up`. If compose fails, the bare-metal plan is still visible.
- `devlane down` is always a no-op for bare-metal. Users stop their own processes.

Commands accept `{{...}}` templating. The scope is the same as `outputs.generated` templates: `ports.<name>`, `lane.*`, `app`, `runtime.env.*`. New variables are added to both scopes together.

### `ports`

Optional. A list of named port needs.

- `name` — service identity, referenced from the manifest (`ports.<name>`) and env (`DEVLANE_PORT_<NAME>`)
- `default` — preferred port, tried first during dev-lane allocation. Plays the stable-fixture role too when `stable_port` is absent.
- `health_path` — optional HTTP path. When declared, the manifest emits `ports.<name>.healthUrl` as `http://localhost:<port><health_path>`. Devlane itself does not probe this URL; it is for agents and tooling.
- `stable_port` — optional. When declared, the stable lane asserts this port as a fixture at `prepare` time. Omit to let `default` play both roles. Declaring `stable_port` lets teams have a distinct dev-lane preference (via `default`) from the stable fixture.
- `pool_hint` — optional `[low, high]` pair. Dev-lane pool allocation walks this subrange first before falling back to the host-wide `port_range`. Must sit inside the host range; if not, the walk falls back immediately.

The adapter declares what the app needs. The shared tool resolves real numbers via the host catalog. Once allocated, ports are sticky — they do not move unless `devlane reassign` or `devlane host gc` is run. See `65-host-catalog.md` for the allocation model, including the fixture semantics that apply to stable lanes.

If `ports` is omitted, no ports are allocated. This is appropriate for pure-CLI repos that do not bind host ports.

### `reserved`

Optional. A list of port numbers this adapter should never allocate for dev lanes.

```yaml
reserved:
  - 5555      # load-test harness
```

Merged with the host-wide `reserved` in `~/.config/devlane/config.yaml` at allocation time. Additive only — adapter `reserved` cannot un-reserve a port the host has reserved. Use this when a specific port is off-limits for *this app* even though the host is fine with it (e.g., the app's CI uses it for load testing).

### `worktree`

Optional. Controls Phase 3 worktree lifecycle behavior.

```yaml
worktree:
  seed:
    - .env
    - .env.local
    - config/master.key
    - config/credentials/
```

- `seed` — explicit list of paths (relative to repo root) copied from the source checkout into a new worktree when `devlane worktree create` runs, **before `prepare`**. Directories are copied recursively. Missing source files warn and continue rather than failing.
- Paths that also appear in `outputs.generated[].destination` are **skipped** with a one-line notice — `prepare` will render them, so seeding would just be shadowed.
- The full list of copied paths is printed on completion, so the user can see exactly which credentials just moved.

There is no default seed list. Devlane does not guess which files are sensitive or which secrets should follow a worktree. Each adapter declares its own list, explicitly. See principle #6 in `00-principles.md`.

### `outputs`

- `manifest_path` — where to write the manifest
- `compose_env_path` — where to write the Compose env file. Required when `runtime.compose_files` is declared; omit otherwise.
- `generated` — files rendered from templates. `destination` must resolve inside the repo root; absolute paths outside the repo are refused at prepare time.

Generated files are tool-owned. `prepare` tracks a sidecar hash under `.devlane/` for each generated destination. If the on-disk file has been hand-edited since the last `prepare`, the tool prints a one-line warning and writes anyway. On first `prepare` (no sidecar hash yet), existing files are quietly overwritten with a notice.

## Design rule

If you find yourself adding repo-specific imperative behavior to the adapter, stop and ask whether it belongs in:

- core lifecycle logic, or
- a repo-owned wrapper outside the adapter

The adapter should describe, not orchestrate. See principle #2.
