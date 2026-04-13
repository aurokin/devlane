# Acceptance checklist

Use this as the practical done bar.

## Init

- `devlane init` creates a valid `devlane.yaml` that passes schema validation
- `devlane init` auto-detects runtime pattern from signals (compose files present → containerized; framework manifest without compose → bare-metal; neither → CLI)
- `devlane init` scans cwd and up to depth 3 below for candidate app roots using `compose*.yaml`, `package.json`, `pyproject.toml`, `Cargo.toml`, `go.mod`, `Gemfile`, `*.csproj` as signals
- `devlane init` in a single-candidate tree scaffolds in place (the common case)
- `devlane init` in a multi-candidate tree enters monorepo mode: lists candidates with inferred kinds and prompts for one or all
- `devlane init --all` in monorepo mode scaffolds an adapter in every candidate without prompting
- `devlane init --app <path>` targets a specific subtree and skips scanning
- `devlane init --list` prints detected candidates without writing anything
- `devlane init --template <name>` overrides detection and uses a named starter template (`containerized-web`, `baremetal-web`, `cli`)
- `devlane init --from <path>` copies an existing adapter as the starting point
- `devlane init` refuses to overwrite an existing `devlane.yaml` unless `--force` is passed
- `devlane init` prompts for confirmation when stdin is a TTY; `--yes` / `--all` / a non-TTY stdin skips the prompt
- `devlane init` prints its detection reasoning (e.g., `Detected: containerized (found compose.yaml)`) so the user knows why it picked what it picked
- `devlane init` scaffolds `host_patterns` as a commented-out block in all three templates; users opt in by uncommenting
- `devlane init` never scaffolds `runtime.run.mode: execute`
- Ambiguous detection defaults to CLI with a clear notice pointing at `--template baremetal-web`
- `devlane prepare` on a directory with no `devlane.yaml` prints a pointer to `devlane init`

## Core contract

- `devlane.yaml` can be loaded from cwd or an explicit path
- `prepare`/`inspect`/`up` walk up from cwd to find the nearest `devlane.yaml`
- `inspect --json` always recomputes from adapter + catalog; never reads `.devlane/manifest.json`
- `inspect --json` works before `prepare` has ever run; emits `allocated: false` for pre-prepare ports
- `inspect --json` emits deterministic JSON
- lane names are stable and slugified
- stable vs dev mode is explicit or reproducible
- paths, hostnames, and project names derive from the adapter
- `host_patterns` is optional; the manifest emits `publicHost: null` when omitted

## Manifest shape

- top-level groups are exactly: `schema`, `app`, `kind`, `lane`, `paths`, `network`, `ports`, `compose`, `outputs` (no top-level `env`, `repo`, or `health`)
- `lane` carries `name`, `slug`, `mode`, `stable`, `branch`, `repoRoot`, `configPath` (the old top-level `repo` fields merged in)
- `paths.composeEnv` is present only when the adapter declares `compose_files` — the key is omitted entirely otherwise, not set to `null`
- `network.publicUrl` is `http://<publicHost>` when `publicHost` is set, `null` otherwise
- env is a *projection* computed at write time, not stored in the manifest; consumers read it from `.devlane/compose.env` or the template `env.*` scope
- template scope flattens `ports.<name>` to the integer port number (not the `{port, allocated, healthUrl}` object); templates use `{{ports.web}}` to get `3100`

## Generated outputs

- `prepare` writes the manifest
- `prepare` writes `.devlane/compose.env` when the adapter declares `compose_files`, omits it otherwise
- `prepare` renders declared templates
- generated directories are created automatically
- missing template fields fail loudly
- templates with undefined variables fail loudly (no silent empty string)
- generated destinations must resolve inside the repo root; absolute paths outside the repo are refused
- `prepare` tracks a sidecar hash per generated file under `.devlane/`
- when an on-disk generated file has been hand-edited, `prepare` prints a one-line warning and writes anyway
- first `prepare` with no sidecar hash quietly overwrites existing files with a one-line notice

## Compose lifecycle

- Compose commands include the lane-specific project name
- Compose files are resolved relative to the adapter location
- default profiles are included
- `--dry-run` shows the exact command
- `status` works without mutating state

## Bare-metal lifecycle

- `devlane up` is a no-op for bare-metal adapters without `runtime.run`, printing a one-line hint
- `devlane up` prints rendered commands when `runtime.run.mode: suggest` (default)
- `devlane up` runs commands fire-and-forget when `runtime.run.mode: execute`
- `devlane down` is always a no-op for bare-metal (no process tracking)
- `runtime.run.commands[].command` renders with the same template scope as `outputs.generated`

## Host catalog

- `~/.config/devlane/catalog.json` is created on first `prepare` and survives process exits
- `~/.config/devlane/config.yaml` is optional and reasonable defaults apply when it is missing
- catalog writes use `fcntl.flock` on `~/.config/devlane/catalog.json.lock` + atomic rename
- catalog write lock acquire timeout is 30 seconds; failure prints a clear message
- catalog reads do not take the lock
- `prepare` allocates a port for every adapter-declared service
- allocations are sticky across `up`/`down`/`up` cycles
- `prepare` does not re-probe existing allocations
- `down` does not modify the catalog
- stable lanes treat `stable_port` as a fixture when declared, otherwise `default`
- stable-lane `prepare` fails loudly on any collision (no silent fallback to pool)
- stable-vs-stable collision prints both adapter paths; no command to paste
- stable-vs-offline-dev collision prints a ready-to-paste `reassign --lane ... && prepare`
- stable-vs-bound-dev collision prints a multi-step recipe; devlane does not kill foreign processes
- `devlane port <service>` prints a plain number by default
- `devlane port <service> --probe` exits non-zero when the assigned port is not bindable
- `--probe` tests both `0.0.0.0` and `::` (IPv6 with V6ONLY=1), TCP-only
- `devlane reassign <service>` is a no-op when the current port is free
- `devlane reassign <service>` only moves the requested service
- `devlane reassign <service> --lane <name>` operates on a lane by name without requiring cd
- `devlane host status` lists every allocation on the host
- `devlane host gc` removes entries whose `repoPath` is missing OR whose service is no longer declared
- `devlane host gc` never removes an entry without an explicit action (prompt or `--yes`)
- reserved ports in `config.yaml` are never allocated, even when they match a dev-lane adapter's declared `default`
- adapter-level `reserved` is merged with host `reserved` at allocation time; additive only
- `pool_hint: [low, high]` is walked before the host-wide `port_range` during dev-lane pool allocation
- `pool_hint` falls back silently to `port_range` when it sits outside the host range
- stable-lane `prepare` fails when its fixture (`stable_port` or `default`) is in effective `reserved`
- allocations from the pool stay within `port_range`
- adapter-declared `default` and `stable_port` are honored even when they sit outside `port_range`

## Validation strictness

- schema-load errors fail before `prepare` logic runs: unknown schema version, invalid `kind`, duplicate `ports[].name`, `host_patterns.dev` missing `{lane}`, `host_patterns.stable == host_patterns.dev`, missing `outputs.manifest_path`
- `prepare`-time errors fail loudly: `kind: cli` with non-empty `ports`, missing template file, absolute destination outside repo, undefined template variable, out-of-scope template variable, missing compose file, stable fixture in `reserved`, pool exhaustion
- warnings do not block: adapter `default` changed since last allocation, `default` outside `port_range` (noted in `inspect --verbose`), `kind: web` with no `ports` and no `compose_files`

## Agent experience

- `AGENTS.md` points agents to the correct docs
- docs and schemas agree
- examples still reflect current contracts
- prompt templates remain usable
- the manifest contains everything an agent needs for discovery (ports with allocation state, hostnames when declared, generated file paths)

## Real adoption bar

A repo can be considered adopted when:

- its generated local files come from `prepare`
- its lane runtime can be started with `up` (or, for bare-metal without `runtime.run`, its run commands are documented elsewhere and discoverable via the manifest)
- stable vs dev ownership is documented
- an agent can enter the repo, run `inspect --json`, and act without repo-specific port heuristics
