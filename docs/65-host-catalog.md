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

- `port_range` bounds where `devlane` is allowed to allocate from the pool
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

## Concurrency model

The catalog is shared across every `devlane` invocation on the host. Two `prepare` commands from different terminals or agents can race.

Devlane uses a lock-then-rename write discipline:

1. Acquire an exclusive `fcntl.flock` on `~/.config/devlane/catalog.json.lock`. Acquire timeout is 30 seconds; after that, fail with a message naming the lock-holder's PID where possible.
2. Read `catalog.json`.
3. Compute the new allocation set.
4. Write `catalog.json.tmp`.
5. `os.rename` the temp file over `catalog.json` (atomic on POSIX).
6. Release the lock.

Every code path that mutates the catalog — `prepare`, `reassign`, `host gc` — uses this discipline. Readers (`inspect`, `port`) do not take the lock; they read `catalog.json` directly and accept that their view may be one write behind.

The lock is OS-managed. If a process is killed mid-write, the lock releases automatically and the next writer reads the unmodified `catalog.json` (because the rename never happened).

POSIX-first. Windows support is deferred to a later phase.

### Unpublished mutations during `prepare` / `reassign`

`prepare` and the write half of `reassign` compute catalog mutations under the lock, but they do **not** publish an updated `catalog.json` before repo-local writes succeed.

The sequence is:

1. preflight repo-local work that can fail cheaply (template existence, destination containment, compose-file presence, schema sanity)
2. acquire the catalog lock and compute the allocation mutation
3. perform repo-local writes against that in-memory result (manifest, compose env, generated files)
4. publish the new `catalog.json` only after those writes succeed
5. on failure, release the lock without publishing the mutation

This keeps unlocked readers from observing a misleadingly "ready" catalog state while repo-local outputs are still stale or missing.

## Allocation algorithm

When `prepare` runs, for each port declared in the adapter:

1. **Existing allocation check.** If there is already a catalog entry for `(app, lane, service)`, keep that port. Do not re-probe. Do not move. Non-negotiable #10.
2. **Merge reserved lists.** Effective `reserved` = host `config.yaml.reserved` ∪ adapter-level `reserved`. Adapter `reserved` is additive-only; it cannot un-reserve a host-reserved port.
3. **Stable-lane allocation (fixture).** If `lane` matches the adapter's `stable_name`, the stable fixture is `stable_port` when declared on the port, otherwise `default`:
   - If the fixture is in effective `reserved`, `prepare` fails with a message telling the user to change either the adapter or `reserved`. No silent fallback.
   - If the fixture is held by another catalog entry, `prepare` fails. See **Collision handling** below.
   - Otherwise, take the fixture. Write the catalog entry.
4. **Dev-lane allocation (pool).** If `lane` is a dev lane:
   - Try the adapter's declared `default` first, unless it is in effective `reserved` or already held in the catalog.
   - If the port declares `pool_hint: [low, high]` and that range sits inside the host `port_range`, walk `[low, high]` start-to-end next, skipping `reserved` and held ports. Otherwise skip to the next step.
   - Walk the full host `port_range` start-to-end, skipping `reserved` and held ports.
   - Take the first bindable port. If no port is free, `prepare` fails and points the user at `devlane host gc` or widening `port_range`.
5. **Refresh `lastPrepared`** on the entry.

`prepare` only probes during allocation. It does not re-probe existing entries.

`inspect --json` uses the same allocation rules to compute **provisional** values for unallocated ports, but it does not take the lock and it does not reserve anything. It answers "what would `prepare` pick if it ran right now?" That answer can still change before `prepare` if another writer publishes first.

### Why `default` can sit outside `port_range`

`port_range` bounds the **pool** devlane allocates from when it needs to pick. It does not constrain adapter-declared `default`s. Real apps sometimes need specific low-numbered ports (`80`, `443`, `5432`) that would never sit inside a typical dev range. The adapter's choice is authoritative over the pool. `reserved` is the only hard "never touch this" list.

## Stable ports are fixtures

The stable fixture is `stable_port` when the adapter declares it on the port, otherwise `default`. Either way, the fixture is reserved in the catalog from the moment stable has been `prepare`d once. It survives `down`, reboots, and long periods of inactivity. The only paths that move a stable allocation are `devlane reassign` and `devlane host gc`.

Fixture semantics require strictness: if stable cannot get its fixture, `prepare` fails loudly rather than silently falling back to a pool port. Silent fallback would defeat the whole point of a fixture — wrappers and docs could no longer rely on stable being at its declared port.

Stable does not evict other lanes to take its fixture. Collisions are surfaced as errors that the user resolves explicitly.

### When to declare `stable_port` vs let `default` do the work

Most adapters can leave `stable_port` unset — `default` plays both roles (dev-lane hint + stable fixture). Declare `stable_port` only when the team wants a distinct dev-lane preference:

```yaml
ports:
  - name: web
    default: 3100          # dev lanes prefer 3100 (then fall back to pool)
    stable_port: 3000      # stable is pinned to 3000
```

This is a deliberate opt-in. The common shape is one number that means both.

## Collision handling

When stable's `prepare` finds its declared default already held:

### Scenario 1: Held by another app's stable

```
ERROR: port 3000 is held by stable lane of app "otherapp" (service "api").
Two stable fixtures cannot share a port.

Resolve by editing one adapter's default, then re-running prepare:
  - here:  /home/auro/code/myapp/devlane.yaml  (service "web", currently default: 3000)
  - there: /home/auro/code/otherapp/devlane.yaml  (service "api", currently default: 3000)
```

Hard error. No command to run. Human picks which adapter moves.

### Scenario 2: Held by a dev lane, port currently free (dev lane offline)

```
ERROR: port 3000 is held by dev lane "feature-x" (service "web") but is not currently bound.

To move that dev lane aside and retry, run:

  devlane reassign web --lane feature-x && devlane prepare
```

Soft error. Exact command printed, ready to paste. One command to resolve.

### Scenario 3: Held by a dev lane, port currently bound (dev lane running)

```
ERROR: port 3000 is held by dev lane "feature-x" (service "web") and is currently bound by a running process.

devlane does not stop other lanes' processes. Stop the dev lane yourself:

  (in /home/auro/code/myapp-feature-x)
  devlane down
  devlane reassign web
  devlane prepare

Then retry prepare here.
```

Hard error with a recipe. Devlane does not kill foreign processes.

## Stickiness guarantee

Once allocated, a port does not move without an explicit action.

- `down` does not release ports
- `up` does not re-probe existing allocations
- `prepare` does not re-probe existing allocations

The only commands that move a port are:

- `devlane reassign <service>` — explicit repair, scoped to the requested service
- `devlane host gc` — explicit cleanup based on staleness heuristics

This means lane identity is stable across stop/start cycles, worktree shelving, and machine reboots. Agents and external tools can cache port information with confidence.

## Probing

Probing is a best-effort check that a port is bindable. Devlane probes both `0.0.0.0` (IPv4 any-interface) and `::` (IPv6 any-interface with `IPV6_V6ONLY=1`) with a TCP listener, closing immediately. A port is reported bindable only when both families succeed.

Probing happens in:

- initial allocation during `prepare`
- explicit checks via `devlane port <service> --probe`
- reassignment logic in `devlane reassign <service>`
- host-wide audits via `devlane host doctor`

A port in `TIME_WAIT` may or may not be reported as free. This is an accepted limitation. Agents should treat the probe as authoritative.

Probing is TCP-only. UDP services are not yet supported by the catalog. Apps that need UDP port coordination should track those ports themselves for now. UDP support is on the long-term roadmap.

## Reassignment

`devlane reassign <service>` is the repair tool.

1. Look up the current allocation for `(app, lane, service)`. If invoked without flags, `lane` is derived from the cwd's adapter and git state. `--lane <name>` overrides with an explicit lane.
2. Probe the current port.
3. If bindable, no-op — return the current port.
4. If not bindable, find a new port using the same rules as initial allocation. Stable lanes fail if their fixture (`stable_port` when declared, otherwise `default`) is unavailable. Dev lanes walk the pool.
5. Update the catalog and re-run the write half of `prepare` (manifest, compose env, rendered templates).

Calling `reassign` when nothing is wrong is safe. It is idempotent.

Only the requested service is moved. Sibling services and other lanes are untouched.

### `--lane <name>` lookup rules

Without `--lane`, `reassign` operates on the current repo + current lane derived from the adapter and git state.

With `--lane <name>`, the command keeps the current app context and only swaps the lane:

1. Resolve the target app from the current repo, or from an explicit `--config` / `--cwd`.
2. Look up the matching `(app, lane=<name>, service=<service>)` allocation.
3. If it exists, operate there. If it does not, fail clearly.

If repo context is unavailable and an implementation chooses to fall back to the catalog directly, it must still respect the true key shape:

1. Find catalog entries matching `(lane=<name>, service=<service>)`.
2. If exactly one entry matches, load the adapter at that entry's `repoPath` and continue there.
3. If no entries match, fail clearly.
4. If multiple entries match across different apps, fail on ambiguity and print the matching `(app, repoPath)` pairs.

This keeps `--lane` usable without `cd` while still respecting the fact that the true key is `(app, lane, service)`, not lane name alone.

## Garbage collection

Catalog entries are never removed by `up`, `down`, or `prepare`. `devlane host gc` is the host-wide stale-entry cleanup command. `devlane worktree remove <lane>` uses a separate targeted deletion path for one removed worktree.

Staleness heuristics — an allocation is stale or drifted if any of the following are true:

1. `repoPath` no longer exists on disk, or
2. the adapter at `repoPath` loads and no longer declares a service matching the allocation's `service` field, or
3. the adapter currently loaded from `repoPath` no longer derives the same `(app, lane)` pair the catalog row claims.

Optional flags:

- `--app <name>` — scope gc to one app
- `--dry-run` — print what would be removed, do not modify the catalog
- `--yes` — skip the confirmation prompt

By default `gc` prints what it would remove and prompts for confirmation.

The third rule is the repo-identity drift check. It is intentionally based on the current adapter and lane derivation at `repoPath`, not on a second persisted identity token in the catalog. If today's checkout at `repoPath` now identifies itself as a different app or lane, the old row is drifted and should not survive indefinitely.

### Scoped cleanup for worktree removal

`devlane worktree remove <lane>` uses a narrower cleanup than `host gc`. After the worktree is removed, devlane deletes only allocations whose `(app, lane, repoPath)` match that removed worktree. It does not scan unrelated repos, it does not remove sibling lanes for the same app, and it does not invoke `host gc`.

## Why `down` does not touch the catalog

`down` stops containers for a lane. The lane itself — its identity, its allocated ports, its generated files — persists.

If `down` released ports, the next `up` would risk landing on different numbers, churning templates and breaking any external tool that cached discovery results.

Keeping `down` narrow preserves lane identity across stop/start cycles. To fully retire a lane, use `devlane host gc`.

## Multi-user and multi-machine notes

The catalog is per-user. Two users on the same machine have independent catalogs. This is intentional — `devlane` is a developer tool, not a multi-tenant service manager.

Port collisions between users on the same host are still possible at the OS level. The live probe handles these the same way it handles any other external process.

The catalog is not portable across machines. Allocations are a function of local host state.

## Relationship to the manifest

The manifest is a snapshot of the catalog's view of one lane. For each port declared in the adapter, the manifest includes the resolved port number and allocation status under `ports.<name>`, and the compose env exports `DEVLANE_PORT_<NAME>` for both compose and templates.

Agents should read ports from the manifest, not from the catalog directly. The catalog is an implementation detail. The manifest is the contract.
