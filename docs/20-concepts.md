# Concepts

This document introduces the minimum vocabulary.

## Lane

A **lane** is a named local execution context.

Common lanes:

- `stable` — the protected lane that may own global names
- `feature-x` — a dev lane for a worktree
- `bugfix-auth` — another dev lane

A lane is not only a Git branch. It is the combination of:

- checkout
- generated files
- compose project name
- hostnames
- cache/state/runtime roots
- machine-readable manifest

For dev lanes, the checkout path is the durable identity anchor. Branch, lane label, and mode are lane metadata that can change within that checkout without creating a brand-new lane record. Stable is the one special lane that may share a canonical checkout.

## Slugification

Some contract fields are derived through a deterministic slug algorithm rather than copied verbatim.

- `lane.slug`, `DEVLANE_LANE_SLUG`, sibling worktree paths, and other lane-derived slugs use the strict form: lowercase and trim the source text, replace each run of non-`[a-z0-9]` characters with `-`, collapse repeated `-`, then trim leading and trailing `-`
- if that strict form produces an empty string, the slug is invalid and commands such as `worktree create` fail before mutating anything
- Compose project names use the same idea after pattern rendering, but allow `_` in addition to `-`: replace each run of characters outside `[a-z0-9_-]` with `-`, collapse repeated `-` and repeated `_`, then trim leading and trailing `-` and `_`

This algorithm is part of the contract. Docs that talk about `<lane-slug>` or slug collisions are referring to this exact behavior, not an implementation-defined approximation.

## Path anchors

Devlane uses three path anchors consistently:

- `repoRoot` — the absolute Git worktree root for the current checkout. `lane.repoRoot` in the manifest points here. Sibling worktree paths are derived from this root.
- `adapterRoot` — the directory containing the active `devlane.yaml`. Relative adapter paths resolve from here.
- `repoPath` — the absolute checkout/worktree root stored in the host catalog for allocation identity. For dev lanes, this is the durable identity anchor.

Rule of thumb:

- adapter-relative paths (`runtime.compose_files`, generated-template paths, output destinations, `worktree.seed`) resolve from `adapterRoot`
- after normalization, those paths must remain inside `repoRoot`

## Stable vs dev

Stable and dev are not symmetric.

Stable may own things like:

- a friendly hostname
- a global wrapper in `~/.local/bin`
- a protected install location
- a well-known service entrypoint

Dev lanes should be isolated and disposable by default.

## Two runtime patterns

A lane can run in one of two shapes:

- **Bare-metal** (default) — the app binds real host ports directly. Ports are coordinated across the whole machine by the host catalog. `devlane up` is a no-op unless the adapter opts in via `runtime.run.commands`, in which case it prints the declared commands and exits. Devlane never spawns bare processes itself — see principle #1 in `00-principles.md`.
- **Containerized** (opt-in) — the app runs via Docker Compose with a lane-aware project name. Declared by adding `runtime.compose_files` to the adapter.

The pattern is signaled declaratively by what the adapter declares: `ports` for host-port services, `runtime.compose_files` for container lifecycle, `runtime.run.commands` for bare-metal command guidance, `lane.host_patterns` for hostname-based discovery. Many repos use only some of these; all are optional and independent.

`kind` remains a descriptive label for the repo (`web`, `cli`, `hybrid`); it does not override the lifecycle fields. A `cli` repo may still declare `ports` or `runtime.compose_files` when it exposes a local service or uses a sidecar.

## Hostnames are optional

Hostname-based discovery (`feature-x.demoapp.localhost`) is a useful enhancement but not a baseline. Most bare-metal dev is reachable as `localhost:<port>` without any DNS or proxy setup. Adapters declare `lane.host_patterns` when the host has a Caddy, Traefik, `/etc/hosts`, or other mechanism that resolves the rendered hostnames. When omitted, discovery is port-based via `manifest.ports.<service>.port`.

## Adapter

A **repo adapter** is a small declarative file, `devlane.yaml`, that tells the shared tool:

- the app name and kind
- how to derive hostnames and Compose project names
- which Compose files matter
- which profiles are default
- which templates should be rendered into generated files

The adapter should be data, not orchestration logic.

## Manifest

The **manifest** is the authoritative machine-readable result of combining:

- the adapter
- the current checkout
- lane naming rules
- derived paths and hostnames

Agents should prefer the manifest over scraping ad hoc files.

## Host catalog

The **host catalog** at `os.UserConfigDir()/devlane/catalog.json` is the tool-owned record of which `(app, repoPath, service)` owns which host port on this machine. In practice that is typically `~/.config/devlane/catalog.json` on Linux and `~/Library/Application Support/devlane/catalog.json` on macOS.

It is the manifest's peer at host scope: the manifest is the contract inside one lane, the catalog is the contract across lanes and repos.

Allocations are sticky. The tool writes, agents and humans read.

## Generated outputs

Repos still need generated files such as:

- `.env.local`
- wrapper configs
- activation scripts
- `.devlane/compose.env`

Those files should be generated from the manifest so every repo can keep its own naming conventions without making the core tool repo-specific.

## Why this split matters

Without this split, every repo reinvents:

- worktree and lane naming
- port and hostname policy
- state directory layout
- compose project naming
- wrapper ownership rules
- agent-facing documentation

With this split:

- the shared tool owns lifecycle
- the repo adapter owns translation
- the manifest becomes the stable contract for agents
