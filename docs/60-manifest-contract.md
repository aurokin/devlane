# Manifest contract

The manifest is the shared language between humans, agents, wrappers, and automation.

## Example shape

```json
{
  "schema": 1,
  "app": "demoapp",
  "kind": "web",
  "repo": {
    "root": "/repo/path",
    "config": "/repo/path/devlane.yaml",
    "branch": "feature-x"
  },
  "lane": {
    "name": "feature-x",
    "slug": "feature-x",
    "mode": "dev",
    "stable": false
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
  },
  "env": {
    "DEVLANE_APP": "demoapp",
    "DEVLANE_LANE": "feature-x",
    "DEVLANE_PORT_WEB": "3100",
    "DEVLANE_PORT_API": "4000"
  }
}
```

## Ports

Each entry in `manifest.ports` is an object:

- `port` — integer, the assigned port
- `allocated` — boolean, `true` when the `(app, lane, service)` tuple has an entry in the host catalog
- `healthUrl` — string or null. Rendered from the adapter's `health_path` as `http://localhost:<port><health_path>`. Null when `health_path` is not declared.

Ports are resolved from the host catalog at `prepare` time. Once allocated they are sticky — see `65-host-catalog.md`.

Stable lanes treat their declared `default` as a fixture: `prepare` either claims the default or fails loudly. Dev lanes allocate from the pool. Both write catalog entries and render this manifest shape the same way; the distinction shows up only in collision handling at `prepare`.

When the adapter declares no `ports`, the manifest still emits `ports: {}` so the shape stays stable for consumers. No `DEVLANE_PORT_*` env vars are emitted.

### `allocated: false`

`inspect --json` always recomputes the manifest in memory from the adapter and the current catalog. It never reads `.devlane/manifest.json` off disk, so it works before `prepare` has ever run.

Before the first `prepare`, no catalog entry exists for `(app, lane, service)`. The manifest emits:

```json
"ports": {
  "web": {"port": 3000, "allocated": false, "healthUrl": "http://localhost:3000/healthz"}
}
```

`port` is the adapter's declared default. `allocated: false` tells the consumer "this is what devlane would allocate; run `prepare` to make it real." Agents should check `allocated` before relying on a port being bindable.

### Env exports

Templates and compose see ports as env:

```
DEVLANE_PORT_WEB=3100
```

Templates can also reference ports via the dot-path mechanism:

```
PORT={{ports.web}}
```

Agents should read `manifest.ports.<name>.port` rather than querying the catalog directly. The catalog is an implementation detail; the manifest is the contract.

## Network

- `projectName` — rendered Compose project name
- `publicHost` — rendered hostname for the current lane mode, or `null` when `host_patterns` is not declared
- `publicUrl` — full URL composed from `publicHost`, or `null` when `publicHost` is null

Hostname-based discovery is optional. Bare-metal adapters that do not declare `host_patterns` emit `publicHost: null` and rely on port-based discovery via `ports.<name>.port`.

## Paths

`paths.composeEnv` is `null` when the adapter does not declare `compose_files`. All other `paths.*` fields are always present.

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
