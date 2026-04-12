# Acceptance checklist

Use this as the practical done bar.

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
