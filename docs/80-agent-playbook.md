# Agent playbook

This document explains how coding agents should use `devlane`.

## Default sequence

When asked to work inside a repo that uses `devlane`, agents should prefer this sequence:

1. `inspect --json`
2. `prepare`
3. `up` or `status`

That order keeps discovery explicit and reproducible.

## What agents should not do first

Avoid leading with:

- guessing ports
- editing generated env files directly
- scraping random shell scripts
- assuming stable and dev use the same naming rules

## If a repo has no adapter yet

Agents should:

1. run `devlane init` to scaffold a starter `devlane.yaml`. Use `--from examples/<example>/devlane.yaml` if a specific example is a closer fit than auto-detection.
2. read `docs/50-adapter-schema.md` and the relevant workflow doc (`70-container-workflow.md` for containerized, `75-baremetal-workflow.md` for bare-metal)
3. wire the repo's current generated files into `outputs.generated`
4. make `inspect --json` and `prepare` produce the current local state deterministically

## What to ask from a repo

A repo adopting `devlane` only needs to answer a small number of questions:

- what is the app identifier?
- is it `web`, `cli`, or `hybrid`?
- what does stable own?
- which files should be generated?
- which Compose files and profiles exist?
- if it is a web app, what hostname pattern should dev lanes use?

## Handling port conflicts

The manifest is the answer, not the agent's memory. An agent that has been running for a while may be acting on a stale port. Another process — another agent, the user, a hook — might have reassigned in the meantime.

When an agent encounters a port conflict, the order is:

1. **Re-check the manifest.** Read `ports.<service>` again.
2. If the manifest value differs from what the agent was using, the agent was stale. Use the manifest value. Done.
3. If the manifest value matches what the agent was using, verify the port is actually free for us: `devlane port <service> --probe`. A non-zero exit confirms a real conflict.
4. Only then: `devlane reassign <service>`. This is idempotent and scoped — it will no-op if the conflict resolved itself, and it will only move the one service.
5. Re-read the manifest for the new port and continue.

Reassign should be the last step, not the first. Most "conflicts" are staleness, and the stickiness guarantee of the host catalog is only valuable if agents avoid unnecessary reassigns.

Before calling `reassign`, check whether the process holding the port is actually yours. Orphan processes from a previous run look identical to external collisions from devlane's perspective.

## Good agent behavior

Good agents leave behind:

- updated docs
- updated example adapters
- updated schemas
- deterministic generated outputs
- explicit acceptance notes

The goal is not only to implement the change, but to preserve the next agent's ability to reason about it quickly.
