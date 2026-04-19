# Adapter schema

Each repo contributes a `devlane.yaml`.

The adapter should stay small and declarative. See principle #2 in `00-principles.md` — adapters describe, the tool orchestrates.

## Scaffolding a new adapter

`devlane init` writes a starter `devlane.yaml` based on what it finds in the repo — Compose files such as `compose.yaml`, `compose.yml`, `compose.dev.yaml`, or `docker-compose.yml` (containerized), framework manifest without compose (bare-metal), or neither (CLI). Hybrid is available as an explicit starter template rather than an inferred detection result: use `--template hybrid-web` when the repo mixes bare-metal and compose on purpose. When `init` detects containerized signals, it preserves the matched Compose filename list in `runtime.compose_files` instead of normalizing it to `compose.yaml`. `--from <path>` copies an existing example literally rather than migrating it: relative paths and repo-specific identifiers are preserved as written, then reviewed by the user in the target repo. Most adoptions can start from `init`, then compare against the example adapters in `examples/` when they need a closer reference.

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

## Path anchor rule

Relative adapter paths resolve from `adapterRoot`, the directory containing `devlane.yaml`. They must remain inside `repoRoot`, the Git worktree root for the checkout. `lane.repoRoot` in the manifest points at `repoRoot`; `filepath.Dir(lane.configPath)` is the corresponding `adapterRoot`.

## `init --from` review rule

When you scaffold from another adapter with `devlane init --from <path>`, devlane first validates the source adapter against the current schema and then copies the YAML as-is. It does not attempt to rewrite:

- `app`
- `lane.host_patterns`
- `runtime.compose_files`
- `outputs.manifest_path`
- `outputs.compose_env_path`
- `outputs.generated[].template`
- `outputs.generated[].destination`
- `worktree.seed`

That keeps the command deterministic and repo-agnostic. It also means imported adapters may contain paths or identifiers that do not make sense in the target repo. After copying, review those fields explicitly before treating the new adapter as adopted.

If a copied relative path would resolve outside the target repo root, `init --from` fails before writing anything. For copied input paths that stay inside the repo root (`runtime.compose_files`, `outputs.generated[].template`, `worktree.seed`), `init` still succeeds but warns when the referenced file does not exist in the target repo yet.

### Top-level

- `schema` — adapter schema version
- `app` — stable app identifier
- `kind` — `web`, `cli`, or `hybrid`. This is descriptive repo metadata, not the runtime switch. Runtime behavior still comes from declared fields such as `ports`, `compose_files`, and `runtime.run`.

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

- `compose_files` — Compose files relative to `adapterRoot`
- `default_profiles` — profiles enabled by default
- `optional_profiles` — known optional profiles
- `env` — extra env values that should be projected into Compose and exposed to templates / bare-metal command rendering as `env.<KEY>`
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
        command: "bin/rails server"
      - name: worker
        command: "bin/sidekiq"
```

- `devlane up` **always prints** these commands and exits. Devlane never spawns bare-metal processes — nothing would supervise them. This is the supervised-substrate rule (principle #1).
- In a hybrid adapter (both `compose_files` and `runtime.run.commands`), `up` prints these commands first, then runs `docker compose up`. If compose fails, the bare-metal plan is still visible.
- `devlane down` is always a no-op for bare-metal. Users stop their own processes.

Commands accept `{{...}}` templating. The scope is the same as `outputs.generated` templates:

- **Phase 1** — top-level Phase 1 manifest groups (`app`, `kind`, `lane`, `paths`, `network`, `compose`, `outputs`) plus `env.<KEY>`. `ready` and `ports.<name>` are not available yet; referencing them is a render error.
- **Phase 2** — adds top-level `ready` plus flattened `ports.<name>` values. `ports.<name>` resolves to the integer port number, not the `{port, allocated, healthUrl}` object.

New variables are added to both scopes together.

### `ports`

Optional. A list of named port needs.

- `name` — service identity, referenced from the manifest (`ports.<name>`) and env (`DEVLANE_PORT_<NAME>`) once Phase 2 host-catalog-backed ports land
- `default` — preferred port, tried first during dev-lane allocation. Plays the stable-fixture role too when `stable_port` is absent.
- `health_path` — optional HTTP path. When declared, the manifest emits `ports.<name>.healthUrl` as `http://localhost:<port><health_path>`. Devlane itself does not probe this URL; it is for agents and tooling.
- `stable_port` — optional. When declared, the stable lane asserts this port as a fixture at `prepare` time. Omit to let `default` play both roles. Declaring `stable_port` lets teams have a distinct dev-lane preference (via `default`) from the stable fixture.
- `pool_hint` — optional `[low, high]` pair. Dev-lane pool allocation walks this subrange first before falling back to the host-wide `port_range`. Must sit inside the host range; if not, the walk falls back immediately.

The adapter declares what the app needs. The shared tool resolves real numbers via the host catalog. Once allocated, ports are sticky — they do not move during ordinary dev-lane churn, with `devlane reassign`, `devlane host gc`, and stable reclaiming the current checkout's fixture as the explicit exceptions. See `65-host-catalog.md` for the allocation model, including the fixture semantics that apply to stable lanes.

If `ports` is omitted, no ports are allocated. This is appropriate for pure-CLI repos that do not bind host ports.

Declaring `ports` is still valid for `kind: cli` when the repo exposes a local HTTP/gRPC/debug service or needs coordinated sidecar access. The runtime pattern is determined by the declared fields, not by `kind` alone.

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

- `seed` — explicit list of paths (relative to `adapterRoot`) copied from the source checkout into a new worktree when `devlane worktree create` runs, **before `prepare`**. Directories are copied recursively. Absolute paths are rejected. Normalized paths may not escape `repoRoot`. Missing source files warn and continue rather than failing.
- symlinks are recreated as symlinks. Devlane does not dereference them or rewrite their targets.
- existing destination paths in the new worktree are overwritten for explicit seed entries, except when the path is also a generated output and therefore skipped. Copy destinations must remain inside the target worktree root.
- regular-file mode bits are preserved best-effort; ownership is not preserved.
- The full list of copied paths is printed on completion, so the user can see exactly which credentials just moved.

There is no default seed list. Devlane does not guess which files are sensitive or which secrets should follow a worktree. Each adapter declares its own list, explicitly. See principle #6 in `00-principles.md`.

Phase 3 worktree commands are supported only when `adapterRoot == repoRoot`. Subtree adapters in monorepos are still valid for in-place commands such as `inspect`, `prepare`, `up`, and `status`, but `worktree create` / `worktree remove` are out of scope for them.

### Seed vs generated — two different categories of file

The seed list and `outputs.generated` answer different questions. Keeping them separate in your repo is the cleanest setup:

- **Generated files** are derived from the manifest (lane-specific URLs, roots, and other deterministic metadata). `prepare` renders them every time, in every worktree. Example: `.env.local` with `NEXT_PUBLIC_SITE_URL={{network.publicUrl}}`.
- **Seed files** are per-developer or per-machine inputs that cannot be derived from anything devlane knows. Example: `.env.secrets` holding an OpenAI key, or `config/master.key` decrypting Rails credentials.

The shapes should not be the same file. If a single `.env.local` mixes a templated port with a hand-pasted API key, split it: generate the lane-derived parts from a template, and put the secrets in a sibling file (`.env.secrets`, `.env.local.personal`, etc.) that the app reads alongside. That way `prepare` owns the generated file and `worktree.seed` owns the secret, with no collision.

If a path *does* appear in both `worktree.seed` and `outputs.generated[].destination`, `worktree create` skips the seed copy with a one-line notice and lets `prepare` render it. Treat this skip as a signal that the adapter is mixing the two categories — nothing is broken, but the seed entry is dead weight.

### `outputs`

- `manifest_path` — where to write the manifest
- `compose_env_path` — where to write the Compose env file. Required when `runtime.compose_files` is declared; omit otherwise.
- `generated` — files rendered from templates. `template`, `manifest_path`, `compose_env_path`, and `destination` are resolved relative to `adapterRoot`. Destinations must remain inside `repoRoot`; absolute paths outside the checkout are refused at prepare time.

Generated files are tool-owned. `prepare` tracks a sidecar hash under `.devlane/` for each generated destination. If the on-disk file has been hand-edited since the last `prepare`, the tool prints a one-line warning and writes anyway. On first `prepare` (no sidecar hash yet), existing files are quietly overwritten with a notice.

## Design rule

If you find yourself adding repo-specific imperative behavior to the adapter, stop and ask whether it belongs in:

- core lifecycle logic, or
- a repo-owned wrapper outside the adapter

The adapter should describe, not orchestrate. See principle #2.
