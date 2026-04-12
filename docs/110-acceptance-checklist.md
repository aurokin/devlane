# Acceptance checklist

Use this as the practical done bar.

## Init

- `devlane init` creates a valid `devlane.yaml` that passes schema validation
- `devlane init` auto-detects runtime pattern from repo signals (compose files present → containerized; framework manifest without compose → bare-metal; neither → CLI)
- `devlane init --template <name>` overrides detection and uses a named starter template
- `devlane init --from <path>` copies an existing adapter as the starting point
- `devlane init` refuses to overwrite an existing `devlane.yaml` unless `--force` is passed
- `devlane prepare` on a directory with no `devlane.yaml` prints a pointer to `devlane init`

## Core contract

- `devlane.yaml` can be loaded from repo root or an explicit path
- `inspect --json` emits deterministic JSON
- lane names are stable and slugified
- stable vs dev mode is explicit or reproducible
- paths, hostnames, and project names derive from the adapter

## Generated outputs

- `prepare` writes the manifest
- `prepare` writes `.devlane/compose.env`
- `prepare` renders declared templates
- generated directories are created automatically
- missing template fields fail loudly

## Compose lifecycle

- Compose commands include the lane-specific project name
- Compose files are resolved relative to repo root
- default profiles are included
- `--dry-run` shows the exact command
- `status` works without mutating state

## Host catalog

- `~/.config/devlane/catalog.json` is created on first `prepare` and survives process exits
- `~/.config/devlane/config.yaml` is optional and reasonable defaults apply when it is missing
- `prepare` allocates a port for every adapter-declared service
- allocations are sticky across `up`/`down`/`up` cycles
- `prepare` does not re-probe existing allocations
- `down` does not modify the catalog
- `devlane port <service>` prints a plain number by default
- `devlane port <service> --probe` exits non-zero when the assigned port is not bindable
- `devlane reassign <service>` is a no-op when the current port is free
- `devlane reassign <service>` only moves the requested service
- `devlane host status` lists every allocation on the host
- `devlane host gc` never removes an entry without an explicit action (prompt or `--yes`)
- reserved ports in `config.yaml` are never allocated, even when they match an adapter's declared `default`
- allocations from the pool stay within `port_range`
- adapter-declared `default` ports are honored even when they sit outside `port_range`

## Agent experience

- `AGENTS.md` points agents to the correct docs
- docs and schemas agree
- examples still reflect current contracts
- prompt templates remain usable
- the manifest contains everything an agent needs for discovery

## Real adoption bar

A repo can be considered adopted when:

- its generated local files come from `prepare`
- its lane runtime can be started with `up`
- stable vs dev ownership is documented
- an agent can enter the repo, run `inspect --json`, and act without repo-specific port heuristics
