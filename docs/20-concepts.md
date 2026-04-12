# Concepts

This document introduces the minimum vocabulary.

## Lane

A **lane** is a named local execution context.

Common lanes:

- `stable` — the protected lane that may own global names
- `feature-x` — a dev lane for a worktree or branch
- `bugfix-auth` — another dev lane

A lane is not only a Git branch. It is the combination of:

- checkout
- generated files
- compose project name
- hostnames
- cache/state/runtime roots
- machine-readable manifest

## Stable vs dev

Stable and dev are not symmetric.

Stable may own things like:

- a friendly hostname
- a global wrapper in `~/.local/bin`
- a protected install location
- a well-known service entrypoint

Dev lanes should be isolated and disposable by default.

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
