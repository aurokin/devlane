# Host catalog

The host catalog is the single source of truth for what `devlane`-managed apps exist on this machine, which lanes are registered, and which ports each lane's services are bound to.

It sits alongside per-repo manifests, not inside them. The manifest answers "what is this lane?" The catalog answers "what is this host running?"

## Why it exists

A single repo can isolate its own lanes with the Compose project name. That breaks down the moment you have two repos on the same host, or a mix of containerized and bare-metal apps, because host ports are global and the tool has no way to coordinate across projects without a shared view.

The catalog is that shared view.

## Files

Two files live in `~/.config/devlane/`:

- `config.yaml` — user-editable configuration (port range, reserved ports)
- `catalog.json` — tool-owned state (allocations)

Keep them separate. The user owns the config. The tool owns the catalog.

## `config.yaml`

```yaml
port_range:
  start: 3000
  end: 9999
reserved:
  - 5432    # postgres
  - 6379    # redis
  - 22
  - 80
  - 443
```

- `port_range` bounds where `devlane` is allowed to allocate
- `reserved` ports are never allocated

The file is optional. Defaults are baked in.

## `catalog.json`

```json
{
  "schema": 1,
  "allocations": [
    {
      "app": "agentchat",
      "lane": "feature-x",
      "service": "web",
      "port": 3100,
      "repoPath": "/home/auro/code/agentchat",
      "lastPrepared": "2026-04-11T14:30:00Z"
    }
  ]
}
```

Each allocation answers "which port does this `(app, lane, service)` tuple own on this host?"

The catalog is tool-owned. Humans and agents should not hand-edit it. Use `devlane host gc` and `devlane reassign` to change it.

## Allocation algorithm

When `prepare` runs, for each port declared in the adapter:

1. If there is already a catalog entry for `(app, lane, service)`, keep that port. The allocation is sticky and is not re-probed.
2. Otherwise, allocate a new port. Ports in `reserved` are never chosen by any path.
   - if the declared `default` is not in `reserved`, try it first. The default may sit outside `port_range` — the adapter's choice is authoritative over the pool.
   - if the default is unavailable, in `reserved`, or not declared, probe upward from the start of `port_range`, skipping anything in `reserved`.
   - take the first bindable port.
3. Write the allocation to the catalog.
4. Refresh `lastPrepared`.

When an adapter's declared `default` collides with `reserved`, `prepare` emits a warning on stderr and falls through to the pool walk. This is treated as a soft misconfiguration (typically from a user adding to `reserved` after the adapter was written) rather than a fatal error, so existing adapters keep working.

`prepare` only probes during allocation. It does not re-probe existing entries.

### Why `default` can sit outside `port_range`

`port_range` is the pool `devlane` allocates *from* when the declared default is not available. It is not a hard constraint on what an adapter may prefer. Real apps sometimes need specific low-numbered ports (`80`, `443`, `5432`) that would never sit inside a typical dev range. `reserved` is the only hard "never touch this" list.

## Stickiness guarantee

Once allocated, a port does not move without an explicit action.

- `down` does not release ports
- `up` does not re-probe existing allocations
- `prepare` does not re-probe existing allocations

The only commands that move a port are:

- `devlane reassign <service>` — explicit repair
- `devlane host gc` — explicit cleanup

This means lane identity is stable across stop/start cycles, worktree shelving, and machine reboots. Agents and external tools can cache port information with confidence.

## Probing

Probing is a best-effort check that a port is bindable. In practice this means attempting to bind a TCP listener to the port on `localhost` and closing it immediately.

Probing happens in:

- initial allocation during `prepare`
- explicit checks via `devlane port <service> --probe`
- reassignment logic in `devlane reassign <service>`
- host-wide audits via `devlane host doctor`

A port in `TIME_WAIT` may or may not be reported as free. This is an accepted limitation. Agents should treat the probe as authoritative.

Probing is TCP-only. UDP services are not yet supported by the catalog. Apps that need UDP port coordination should track those ports themselves for now. UDP support is on the long-term roadmap.

## Reassignment

`devlane reassign <service>` is the repair tool.

1. Look up the current allocation
2. Probe the current port
3. If bindable, no-op — return the current port
4. If not bindable, find a new port using the same rules as initial allocation: ports in `reserved` are never chosen; honor the adapter's `default` if free and not reserved; otherwise walk `port_range`, skipping reserved, and take the first bindable port
5. Update the catalog and re-run the write half of `prepare` (manifest, compose env, rendered templates)

Calling `reassign` when nothing is wrong is safe. It is idempotent.

Only the requested service is moved. Sibling services and other lanes are untouched.

## Garbage collection

Catalog entries are never removed implicitly. `devlane host gc` is the only command that removes them.

Default heuristic for gc candidates:

- `repoPath` no longer exists, or
- the repo no longer has the declared lane (for git-backed lanes)

Optional flags:

- `--older-than <duration>` — include entries whose `lastPrepared` is older than the given interval
- `--app <name>` — scope gc to one app
- `--yes` — skip the confirmation prompt

By default `gc` prints what it would remove and prompts for confirmation.

## Why `down` does not touch the catalog

`down` stops containers for a lane. The lane itself — its identity, its allocated ports, its generated files — persists.

If `down` released ports, the next `up` would risk landing on different numbers, churning templates and breaking any external tool that cached discovery results.

Keeping `down` narrow preserves lane identity across stop/start cycles. To fully retire a lane, use `devlane host gc`.

## Multi-user and multi-machine notes

The catalog is per-user. Two users on the same machine have independent catalogs. This is intentional — `devlane` is a developer tool, not a multi-tenant service manager.

Port collisions between users on the same host are still possible at the OS level. The live probe handles these the same way it handles any other external process.

The catalog is not portable across machines. Allocations are a function of local host state.

## Relationship to the manifest

The manifest is a snapshot of the catalog's view of one lane. For each port declared in the adapter, the manifest includes the resolved port number under `ports`, and the compose env exports `DEVLANE_PORT_<NAME>` for both compose and templates.

Agents should read ports from the manifest, not from the catalog directly. The catalog is an implementation detail. The manifest is the contract.
