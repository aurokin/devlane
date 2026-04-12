# Agent playbook

This document explains how coding agents should use `devlane`.

## Default sequence

When asked to work inside a repo that uses `devlane`, agents should prefer this sequence:

1. `inspect --json`
2. `prepare`
3. `up` or `status`

That order keeps discovery explicit and reproducible.

`inspect --json` always works — it recomputes the manifest from the adapter plus the current catalog and never reads stale state off disk. Before `prepare` has ever run, ports will appear with `allocated: false` and the adapter's declared defaults as the port numbers. That is the signal that `prepare` has not been run yet.

## What agents should not do first

Avoid leading with:

- guessing ports
- editing generated env files directly
- scraping random shell scripts
- assuming stable and dev use the same naming rules

## If a repo has no adapter yet

Agents should:

1. run `devlane init` to scaffold a starter `devlane.yaml`. Use `--from examples/<example>/devlane.yaml` if a specific example is a closer fit than auto-detection. Pass `--yes` if you do not have an interactive TTY.
2. read `docs/50-adapter-schema.md` and the relevant workflow doc (`70-container-workflow.md` for containerized, `75-baremetal-workflow.md` for bare-metal)
3. wire the repo's current generated files into `outputs.generated`
4. make `inspect --json` and `prepare` produce the current local state deterministically

## What to ask from a repo

A repo adopting `devlane` only needs to answer a small number of questions:

- what is the app identifier?
- is it `web`, `cli`, or `hybrid`?
- what does stable own?
- which files should be generated?
- which Compose files and profiles exist (if containerized)?
- for bare-metal, what commands start the services (optional, powers `devlane up`)?
- if the host has a proxy or DNS, what hostname pattern should lanes use (optional)?

## Discovery: hostnames vs ports

Two discovery surfaces are first-class:

- **Hostname-based** — when the adapter declares `host_patterns` and the host has a proxy or DNS resolving them. `manifest.network.publicHost` and `publicUrl` are populated.
- **Port-based** — the default for most bare-metal apps. `manifest.ports.<service>.port` is the authoritative number. `manifest.network.publicHost` is null.

Agents should check both. If `publicHost` is non-null, it is the preferred way to reach the app (stable across port changes). Otherwise, use `manifest.ports.<service>.port` on localhost. Always read the manifest; never guess.

## Handling port conflicts

The manifest is the answer, not the agent's memory. An agent that has been running for a while may be acting on a stale port. Another process — another agent, the user, a hook — might have reassigned in the meantime.

When an agent encounters a port conflict, the order is:

1. **Re-check the manifest.** Read `ports.<service>.port` again.
2. If the manifest value differs from what the agent was using, the agent was stale. Use the manifest value. Done.
3. If the manifest value matches what the agent was using, verify the port is actually free for us: `devlane port <service> --probe`. A non-zero exit confirms a real conflict.
4. Only then: `devlane reassign <service>`. This is idempotent and scoped — it will no-op if the conflict resolved itself, and it will only move the one service.
5. Re-read the manifest for the new port and continue.

Reassign should be the last step, not the first. Most "conflicts" are staleness, and the stickiness guarantee of the host catalog is only valuable if agents avoid unnecessary reassigns.

Before calling `reassign`, check whether the process holding the port is actually yours. Orphan processes from a previous run look identical to external collisions from devlane's perspective.

### When `prepare` fails with a collision error

If `prepare` itself fails with a stable-fixture collision (see `65-host-catalog.md`), the error message prints the exact resolution. Agents should parse the message and follow its instructions:

- **Stable vs another app's stable** — no automatic resolution. The agent should report the error to the user and not pick a winner. Which app moves is a human decision.
- **Stable vs offline dev lane** — the error includes a ready-to-paste `devlane reassign web --lane <name> && devlane prepare` command. The agent can run it.
- **Stable vs bound dev lane** — the dev lane must be stopped first. The agent should report the error and not attempt to kill foreign processes.

## Good agent behavior

Good agents leave behind:

- updated docs
- updated example adapters
- updated schemas
- deterministic generated outputs
- explicit acceptance notes

The goal is not only to implement the change, but to preserve the next agent's ability to reason about it quickly.
