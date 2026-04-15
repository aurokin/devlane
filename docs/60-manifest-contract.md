# Manifest contract

The manifest is the shared language between humans, agents, wrappers, and automation.

This document mixes two surfaces:

- **Phase 1 current surface** — top-level `schema`, `app`, `kind`, `lane`, `paths`, `network`, `compose`, `outputs`
- **Phase 2 target surface** — adds top-level `ready` plus `ports.<service> = {port, allocated, healthUrl}`

When a section describes Phase-2-only fields, it says so explicitly.

## Example shape

The JSON below is the **Phase 2 target shape**.

```json
{
  "schema": 1,
  "app": "demoapp",
  "kind": "web",
  "ready": true,
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

Phase 2 has ten fields at the top level. Phase 1 omits `ready` and `ports`.

Ten fields at the top level:

- `schema`, `app`, `kind` — identity primitives
- `ready` — boolean, true iff the catalog has entries for every declared port (see below)
- `lane` — everything that identifies *this lane* (name/slug/mode/stable plus branch, repoRoot, configPath)
- `paths` — where devlane writes things
- `network` — project name + optional public hostname/URL
- `ports` — per-service allocation state
- `compose` — resolved compose files and profiles
- `outputs` — generated template destinations

There is no top-level `env` or `repo` block. Env is a *projection* computed at write time for `.devlane/compose.env` and template rendering (see below). Repo identity lives inside `lane`.

There is also no `runtime.run` block in the manifest. Bare-metal run commands remain adapter-owned guidance, not manifest contract surface.

## `ready` (Phase 2)

`manifest.ready` is a top-level boolean that answers one specific question: **is the catalog consistent with the adapter for this lane right now?**

It is `true` iff every port the adapter declares has an allocation in the host catalog for this `(app, repoPath, service)` tuple. Equivalently, `ready` is `true` iff every entry in `manifest.ports.*.allocated` is `true`.

This is the field agents should check first. It is cheaper than iterating `ports.*.allocated` and it states the question at the top level where it belongs.

### What `ready` does *not* claim

`ready: true` does **not** mean:

- the ports are bindable right now (another process might be squatting on them)
- the lane is running (`down` doesn't affect `ready`)
- the health endpoints respond (devlane doesn't probe `healthUrl`)

For each of those, there are separate surfaces:

- **Bindability right now:** `devlane port <service> --probe` exits non-zero if something is on the port.
- **Is the lane running:** `devlane status` (runs `docker compose ps` for containerized; prints manifest summary for bare-metal).
- **Health endpoints:** your own code, hitting `manifest.ports.<svc>.healthUrl`.

### Freshness surfaces

There are actually three different "is this fresh" questions tangled up in the manifest. `ready` only answers the first one cleanly:

1. **Have all declared ports been allocated for this checkout's lane state?** → `ready`.
2. **Is the catalog state still what the last successful publish wrote?** → fresh `inspect --json`. On-disk `.devlane/manifest.json` is a snapshot and can drift if another process has run `reassign` or `host gc`.
3. **Are the repo-local generated outputs current right now?** → not represented as a separate manifest bit today; run `prepare` when generated files or `.devlane/compose.env` need to be refreshed.
4. **Is the port actually bindable right now?** → `--probe`.

Agents that care about freshness should re-run `inspect --json`. Agents that need current generated outputs should run `prepare`. Agents that want bindability certainty should probe. `ready` is the cheap top-level allocation check; useful, but it does not substitute for either of the other surfaces.

## Lane

`lane.branch`, `lane.repoRoot`, and `lane.configPath` were the old top-level `repo` block; they now live under `lane` so every identity field is in one place.

`lane.slug` is the deterministic lane slug defined in `20-concepts.md`. It is contract surface because worktree paths, env projection, and operator-facing lane identity all depend on it.

`lane.repoRoot` is the absolute Git worktree root for the checkout. `lane.configPath` is the absolute path to the active `devlane.yaml`; `filepath.Dir(lane.configPath)` is the corresponding `adapterRoot` used to resolve relative adapter paths.

## Ports (Phase 2)

Each entry in `manifest.ports` is an object:

- `port` — integer, the assigned port
- `allocated` — boolean, `true` when the `(app, repoPath, service)` tuple has an entry in the host catalog
- `healthUrl` — string or null. Rendered from the adapter's `health_path` as `http://localhost:<port><health_path>`. Null when `health_path` is not declared.

Ports are resolved from the host catalog at `prepare` time. Once allocated they are sticky — see `65-host-catalog.md`.

Stable lanes use `stable_port` as a fixture when declared; otherwise the adapter's `default` plays both roles (dev-lane hint + stable fixture). Dev lanes allocate from the pool, preferring any `pool_hint` range before falling back to the host-wide `port_range`. Both write catalog entries and render this manifest shape the same way; the distinction shows up only in collision handling at `prepare`.

When the adapter declares no `ports`, the Phase 2 manifest emits `ports: {}` and `ready: true` (there are no allocations to wait on). The shape stays stable for consumers.

### `allocated: false` (Phase 2)

`inspect --json` always recomputes the manifest in memory from the adapter and the current catalog. It never reads `.devlane/manifest.json` off disk, so it works before `prepare` has ever run.

Before the first `prepare`, no catalog entry exists for `(app, repoPath, service)`. The manifest emits:

```json
"ready": false,
"ports": {
  "web": {"port": 3000, "allocated": false, "healthUrl": "http://localhost:3000/healthz"}
}
```

For a stable lane, `port` is the fixture (`stable_port` when declared, otherwise `default`). For a dev lane, `port` is a **provisional candidate** computed against the live catalog using the same allocator `prepare` would use right now: walk services in adapter declaration order, tentatively holding earlier picks in memory, then for each service try `default`, then `pool_hint` when valid, then the host `port_range`, all while respecting held and reserved ports. `ready: false` and `allocated: false` tell the consumer "this is the current best candidate; run `prepare` to commit it." Because the value is computed against the live catalog rather than reserved ahead of time, it may still change before `prepare` if another writer publishes first. Agents should check `ready` (or at minimum the per-port `allocated`) before relying on a port being bindable.

Branch, lane label, and mode are metadata carried under `lane.*`. They are not the durable identity key for dev-lane allocations; switching branches in place updates the manifest metadata for the same checkout rather than creating drift by itself.

## Paths

`paths.composeEnv` is present only when the adapter declares `runtime.compose_files` (and `outputs.compose_env_path`). It is fully omitted otherwise — the key does not appear at all. All other `paths.*` fields are always present.

## Network

- `projectName` — rendered Compose project name
- `publicHost` — rendered hostname for the current lane mode, or `null` when `lane.host_patterns` is not declared
- `publicUrl` — `http://<publicHost>` when `publicHost` is set, `null` otherwise. Convenience for consumers that want a URL without concatenating.

Hostname-based discovery is optional. Bare-metal adapters that do not declare `lane.host_patterns` emit `publicHost: null` and rely on port-based discovery via `ports.<name>.port` once Phase 2 lands.

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

For optional values, the projection uses empty strings rather than omitting keys:

- `DEVLANE_COMPOSE_ENV=""` when the adapter does not declare compose
- `DEVLANE_PUBLIC_HOST=""` and `DEVLANE_PUBLIC_URL=""` when `lane.host_patterns` is omitted

The projection is not stored in `manifest.json` because it is 1:1 derivable from the other fields. Consumers that want env should read `.devlane/compose.env` or pass manifest + adapter through `compute_env()`.

## Template scope

Templates see the top-level manifest groups for the active phase plus:

- **Phase 1** — `app`, `kind`, `lane`, `paths`, `network`, `compose`, `outputs`, plus `env.<KEY>`. `ready` and `ports.<name>` are not available yet; referencing them is a render error.
- **Phase 2** — adds top-level `ready` plus `ports.<name>` flattened to the integer port number (not the object). Use `{{ports.web}}` to get `3100`, not the `{port, allocated, healthUrl}` object.
- `env.<KEY>` — the env projection described above.

New variables are added to the template scope and the `runtime.run.commands` scope together.

## Required qualities

The manifest should be:

- deterministic
- JSON-serializable
- easy to diff
- safe for agents to consume
- broad enough to drive template rendering and Compose lifecycle

## Why agents should consume the manifest

If agents read the manifest (via `inspect --json`, not the file on disk), they do not need to know:

- which repo-specific env file exists
- which stable/worktree variable names the repo chose
- which hostname pattern the repo uses
- where the runtime, state, or cache directories live

The manifest centralizes those answers. See principle #3 in `00-principles.md`.

## Stability policy

Treat manifest fields as contract surface.

- adding fields is usually safe
- renaming or removing fields is a breaking change
- changing semantics without documentation is not acceptable

Keep `schemas/manifest.schema.json` current when the contract changes.
