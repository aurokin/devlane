# Manifest contract

The manifest is the shared language between humans, agents, wrappers, and automation.

## Example shape

```json
{
  "schema": 1,
  "app": "demoapp",
  "kind": "web",
  "lane": {
    "name": "feature-x",
    "slug": "feature-x",
    "mode": "dev",
    "stable": false,
    "branch": "feature-x",
    "repoRoot": "/repo/path",
    "configPath": "/repo/path/devlane.yaml"
  },
  "paths": {
    "manifest": "/repo/path/.devlane/manifest.json",
    "composeEnv": "/repo/path/.devlane/compose.env",
    "stateRoot": "/repo/path/.devlane/state/feature-x",
    "cacheRoot": "/repo/path/.devlane/cache/feature-x",
    "runtimeRoot": "/repo/path/.devlane/runtime/feature-x"
  },
  "network": {
    "projectName": "demoapp_feature-x",
    "publicHost": "feature-x.demoapp.localhost",
    "publicUrl": "http://feature-x.demoapp.localhost"
  },
  "ports": {
    "web": {
      "port": 3100,
      "allocated": true,
      "healthUrl": "http://localhost:3100/healthz"
    },
    "api": {
      "port": 4000,
      "allocated": true,
      "healthUrl": null
    }
  },
  "compose": {
    "files": ["/repo/path/compose.yaml"],
    "profiles": ["web"]
  },
  "outputs": {
    "generated": [
      {
        "template": "/repo/path/templates/app.env.tmpl",
        "destination": "/repo/path/.devlane/generated/app.env"
      }
    ]
  }
}
```

## Top-level shape

Nine groups, no duplicates:

- `schema`, `app`, `kind` — identity primitives
- `lane` — everything that identifies *this lane* (name/slug/mode/stable plus branch, repoRoot, configPath)
- `paths` — where devlane writes things
- `network` — project name + optional public hostname/URL
- `ports` — per-service allocation state
- `compose` — resolved compose files and profiles
- `outputs` — generated template destinations

There is no top-level `env` or `repo` block. Env is a *projection* computed at write time for `.devlane/compose.env` and template rendering (see below). Repo identity lives inside `lane`.

## Lane

`lane.branch`, `lane.repoRoot`, and `lane.configPath` were the old top-level `repo` block; they now live under `lane` so every identity field is in one place.

## Ports

Each entry in `manifest.ports` is an object:

- `port` — integer, the assigned port
- `allocated` — boolean, `true` when the `(app, lane, service)` tuple has an entry in the host catalog
- `healthUrl` — string or null. Rendered from the adapter's `health_path` as `http://localhost:<port><health_path>`. Null when `health_path` is not declared.

Ports are resolved from the host catalog at `prepare` time. Once allocated they are sticky — see `65-host-catalog.md`.

Stable lanes use `stable_port` as a fixture when declared; otherwise the adapter's `default` plays both roles (dev-lane hint + stable fixture). Dev lanes allocate from the pool, preferring any `pool_hint` range before falling back to the host-wide `port_range`. Both write catalog entries and render this manifest shape the same way; the distinction shows up only in collision handling at `prepare`.

When the adapter declares no `ports`, the manifest still emits `ports: {}` so the shape stays stable for consumers.

### `allocated: false`

`inspect --json` always recomputes the manifest in memory from the adapter and the current catalog. It never reads `.devlane/manifest.json` off disk, so it works before `prepare` has ever run.

Before the first `prepare`, no catalog entry exists for `(app, lane, service)`. The manifest emits:

```json
"ports": {
  "web": {"port": 3000, "allocated": false, "healthUrl": "http://localhost:3000/healthz"}
}
```

`port` is the adapter's declared default (or `stable_port` on a stable lane, when declared). `allocated: false` tells the consumer "this is what devlane would allocate; run `prepare` to make it real." Agents should check `allocated` before relying on a port being bindable.

## Paths

`paths.composeEnv` is present only when the adapter declares `compose_files` (and `outputs.compose_env_path`). It is fully omitted otherwise — the key does not appear at all. All other `paths.*` fields are always present.

## Network

- `projectName` — rendered Compose project name
- `publicHost` — rendered hostname for the current lane mode, or `null` when `host_patterns` is not declared
- `publicUrl` — `http://<publicHost>` when `publicHost` is set, `null` otherwise. Convenience for consumers that want a URL without concatenating.

Hostname-based discovery is optional. Bare-metal adapters that do not declare `host_patterns` emit `publicHost: null` and rely on port-based discovery via `ports.<name>.port`.

## Env projection (not stored in the manifest)

Two places consume an env projection:

- `.devlane/compose.env` — written by `prepare` when compose is in use
- template rendering — `{{env.DEVLANE_*}}` is available in any generated template

The projection is computed at write time from the manifest plus the adapter's `runtime.env` block. Keys include:

```
DEVLANE_APP, DEVLANE_APP_SLUG, DEVLANE_KIND
DEVLANE_BRANCH, DEVLANE_MODE, DEVLANE_LANE, DEVLANE_LANE_SLUG, DEVLANE_STABLE
DEVLANE_REPO_ROOT, DEVLANE_CONFIG, DEVLANE_MANIFEST, DEVLANE_COMPOSE_ENV
DEVLANE_STATE_ROOT, DEVLANE_CACHE_ROOT, DEVLANE_RUNTIME_ROOT
DEVLANE_COMPOSE_PROJECT, DEVLANE_PUBLIC_HOST, DEVLANE_PUBLIC_URL
DEVLANE_PORT_<NAME>  (one per declared port)
```

Plus any `runtime.env` keys declared in the adapter, with `{public_host}`, `{public_url}`, `{lane_name}`, `{lane_slug}`, `{app}`, `{mode}`, `{branch}`, `{project_name}`, `{state_root}`, `{cache_root}`, `{runtime_root}` expanded.

The projection is not stored in `manifest.json` because it is 1:1 derivable from the other fields. Consumers that want env should read `.devlane/compose.env` or pass manifest + adapter through `compute_env()`.

## Template scope

Templates see every top-level manifest group (`app`, `kind`, `lane`, `paths`, `network`, `compose`, `outputs`) plus:

- `ports.<name>` — flattened to the integer port number (not the object). Use `{{ports.web}}` to get `3100`, not the `{port, allocated, healthUrl}` object.
- `env.<KEY>` — the env projection described above.

New variables are added to the template scope and the `runtime.run` command scope together.

## Required qualities

The manifest should be:

- deterministic
- JSON-serializable
- easy to diff
- safe for agents to consume
- broad enough to drive template rendering and Compose lifecycle

## Why agents should consume the manifest

If agents read the manifest, they do not need to know:

- which repo-specific env file exists
- which stable/worktree variable names the repo chose
- which hostname pattern the repo uses
- where the runtime, state, or cache directories live

The manifest centralizes those answers.

## Stability policy

Treat manifest fields as contract surface.

- adding fields is usually safe
- renaming or removing fields is a breaking change
- changing semantics without documentation is not acceptable

Keep `schemas/manifest.schema.json` current when the contract changes.
