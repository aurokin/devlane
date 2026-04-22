# Host catalog

The host catalog is the single source of truth for what `devlane`-managed apps exist on this machine, which lanes are registered, and which ports each lane's services are bound to.

It sits alongside per-repo manifests, not inside them. The manifest answers "what is this lane?" The catalog answers "what is this host running?"

## Why it exists

A single repo can isolate its own lanes with the Compose project name. That breaks down the moment you have two repos on the same host, or a mix of containerized and bare-metal apps, because host ports are global and the tool has no way to coordinate across projects without a shared view.

The catalog is that shared view.

## Files

Two files live under `os.UserConfigDir()/devlane/`:

- `config.yaml` — user-editable configuration (port range, reserved ports)
- `catalog.json` — tool-owned state (allocations)

Keep them separate. The user owns the config. The tool owns the catalog.

Examples in this doc use `~/.config/devlane` as Linux-style shorthand. On macOS the current implementation uses `~/Library/Application Support/devlane`.

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

When `config.yaml` is absent, the defaults are:

- `port_range.start = 3000`
- `port_range.end = 9999`
- `reserved = [22, 80, 443, 5432, 6379]`

## `catalog.json`

```json
{
  "schema": 1,
  "allocations": [
    {
      "app": "agentchat",
      "repoPath": "/home/auro/code/agentchat-feature-x",
      "service": "web",
      "port": 3100,
      "mode": "dev",
      "lane": "feature-x",
      "branch": "feature-x",
      "lastPrepared": "2026-04-11T14:30:00Z"
    }
  ]
}
```

Each allocation answers "which port does this `(app, repoPath, service)` tuple own on this host?"

`mode`, `lane`, and `branch` are stored as metadata for operator output and convenience selection. They are refreshed on `prepare`, but they are not part of the durable identity key for dev-lane allocations.

`repoPath` is the absolute Git worktree root for the checkout, not the adapter directory. For subtree adapters in monorepos, multiple adapters may share the same `repoPath` while still producing different manifests from different `configPath` values.

The catalog is tool-owned. Humans and agents should not hand-edit it. Today `prepare` is the only shipped command that mutates it.

## Concurrency model

The catalog is shared across every `devlane` invocation on the host. Two `prepare` commands from different terminals or agents can race.

Devlane uses a lock-then-rename write discipline:

1. Acquire an exclusive `fcntl.flock` on `catalog.json.lock` in the devlane user config directory. Acquire timeout is 30 seconds; after that, fail with a message naming the lock-holder's PID where possible.
2. Read `catalog.json`.
3. Compute the new allocation set.
4. Write `catalog.json.tmp`.
5. `os.rename` the temp file over `catalog.json` (atomic on POSIX).
6. Release the lock.

Every code path that mutates the catalog uses this discipline. Today that means `prepare`. Readers such as `inspect` do not take the lock; they read `catalog.json` directly and accept that their view may be one write behind.

The lock is OS-managed. If a process is killed mid-write, the lock releases automatically and the next writer reads the unmodified `catalog.json` (because the rename never happened).

POSIX-first. Windows support is deferred to a later phase.

### Unpublished mutations during `prepare`

`prepare` computes catalog mutations under the lock, but it does **not** publish an updated `catalog.json` before repo-local writes succeed.

The sequence is:

1. preflight repo-local work that can fail cheaply (template existence, destination containment, compose-file presence, schema sanity)
2. acquire the catalog lock and compute the allocation mutation
3. perform repo-local writes against that in-memory result (manifest, compose env, generated files)
4. publish the new `catalog.json` only after those writes succeed
5. on failure before publish, roll back any repo-local outputs that were already promoted, then release the lock without publishing the mutation
6. if publish succeeds but lock release fails, return that lock-close error without rolling back the already-published catalog or repo-local outputs

This keeps unlocked readers from observing a misleadingly "ready" catalog state while repo-local outputs are still stale or missing.

Repo-local writes are staged to temp files in the destination directories and then promoted in deterministic order via atomic rename where possible. If a late promotion fails, devlane restores snapshots for any already-promoted outputs before returning the error. The catalog still stays unpublished.

## Allocation algorithm

When `prepare` runs, for each port declared in the adapter, in adapter declaration order:

1. **Existing allocation check.** If there is already a catalog entry for `(app, repoPath, service)`, keep that port for dev lanes. Stable lanes reuse an existing row only when it already matches the service's fixture (`stable_port` when declared, otherwise `default`). If the same checkout is switching into stable mode and its existing row is on a dev-only port, stable evaluates the fixture instead of silently reusing the old port.
2. **Merge reserved lists.** Effective `reserved` = host `config.yaml.reserved` ∪ adapter-level `reserved`. Adapter `reserved` is additive-only; it cannot un-reserve a host-reserved port.
3. **Stable-lane allocation (fixture).** If `lane` matches the adapter's `stable_name`, the stable fixture is `stable_port` when declared on the port, otherwise `default`:
   - If the fixture is in effective `reserved`, `prepare` fails with a message telling the user to change either the adapter or `reserved`. No silent fallback.
   - If the fixture is held by another catalog entry, `prepare` fails. See **Collision handling** below.
   - Otherwise, take the fixture. Write the catalog entry.
4. **Dev-lane allocation (pool).** If `lane` is a dev lane:
   - Try the adapter's declared `default` first, unless it is in effective `reserved` or already held in the catalog.
   - If the port declares `pool_hint: [low, high]` and that range sits inside the host `port_range`, walk `[low, high]` start-to-end next, skipping `reserved` and held ports. Otherwise skip to the next step.
   - Walk the full host `port_range` start-to-end, skipping `reserved` and held ports.
   - Take the first bindable port. If no port is free, `prepare` fails and points the user at manual cleanup.
5. **Refresh metadata** on the entry: `mode`, `lane`, `branch`, `lastPrepared`. When stable claims its fixture for the current checkout after a prior dev allocation, it updates that existing row in place rather than creating a duplicate row for the same `(app, repoPath, service)`.

During both `prepare` and provisional `inspect --json` computation, ports chosen earlier in declaration order are treated as tentatively held while later services are resolved. A single manifest must never assign the same port to two services in one checkout.

`prepare` and provisional `inspect --json` probe only while choosing a new port. They do not re-probe existing catalog entries.

`inspect --json` uses the same allocation rules to compute **provisional** values for unallocated ports, but it does not take the lock and it does not reserve anything. For dev lanes, it reports the current bindable candidate `prepare` would choose right now. For stable lanes, it reports the fixture only when that fixture is currently usable; otherwise `inspect` fails with the same unavailability condition `prepare` would surface. Any provisional answer can still change before `prepare` if another writer publishes first.

### Why `default` can sit outside `port_range`

`port_range` bounds the **pool** devlane allocates from when it needs to pick. It does not constrain adapter-declared `default`s. Real apps sometimes need specific low-numbered ports (`80`, `443`, `5432`) that would never sit inside a typical dev range. The adapter's choice is authoritative over the pool. `reserved` is the only hard "never touch this" list.

## Stable ports are fixtures

The stable fixture is `stable_port` when the adapter declares it on the port, otherwise `default`. Either way, the fixture is reserved in the catalog from the moment stable has been `prepare`d once. It survives `down`, reboots, and long periods of inactivity.

Fixture semantics require strictness: if stable cannot get its fixture, `prepare` fails loudly rather than silently falling back to a pool port. Silent fallback would defeat the whole point of a fixture — wrappers and docs could no longer rely on stable being at its declared port.

Stable does not evict other lanes to take its fixture. Collisions are surfaced as errors that the user resolves explicitly.

If the current checkout already has a dev allocation for the same service, stable does not treat that dev-only port as authoritative. It either updates that same row onto the fixture or fails if the fixture is unavailable.

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

The allocation and collision model below is current. The operator repair commands are not shipped yet, so collision recovery is still manual today.

When stable's `prepare` finds its fixture (`stable_port` when declared, otherwise `default`) already held:

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

Current recovery is manual. Devlane does not yet ship a scoped repair command for this case.

Resolve by either:
  - choosing a different stable fixture in the adapter, or
  - retiring or cleaning up the conflicting checkout before retrying `prepare`
```

### Scenario 3: Held by a dev lane, port currently bound (dev lane running)

```
ERROR: port 3000 is held by dev lane "feature-x" (service "web") and is currently bound by a running process.

devlane does not stop other lanes' processes.

Current recovery is manual:
  - for compose-backed lanes, stop the conflicting lane in its checkout first
  - for pure bare-metal lanes, stop the listening process outside devlane
  - then retry `prepare`
```

## Stickiness guarantee

Once allocated, a port does not move without an explicit action.

- `down` does not release ports
- `up` does not re-probe existing allocations
- `prepare` does not re-probe existing allocations

The only shipped path that can move a port is a fresh allocation for a checkout that does not already own one. Existing allocations stay put across ordinary churn.

This means lane identity is stable across stop/start cycles, worktree shelving, and machine reboots. Agents and external tools can cache port information with confidence.

## Probing

Probing is a best-effort check that a port is bindable. Devlane probes both `0.0.0.0` (IPv4 any-interface) and `::` (IPv6 any-interface with `IPV6_V6ONLY=1`) with a TCP listener, closing immediately. A port is reported bindable only when both families succeed.

Probing happens during initial allocation and while computing provisional unallocated results for `inspect --json`.

A port in `TIME_WAIT` may or may not be reported as free. This is an accepted limitation. Agents should treat the probe as authoritative.

Probing is TCP-only. UDP services are not yet supported by the catalog. Apps that need UDP port coordination should track those ports themselves for now.

## Why `down` does not touch the catalog

`down` stops containers for a lane. The lane itself — its identity, its allocated ports, its generated files — persists.

If `down` released ports, the next `up` would risk landing on different numbers, churning templates and breaking any external tool that cached discovery results.

Keeping `down` narrow preserves lane identity across stop/start cycles. A checkout that still exists on disk keeps its allocation by design; branch switches within that checkout update metadata rather than retiring the lane.

## Multi-user and multi-machine notes

The catalog is per-user. Two users on the same machine have independent catalogs. This is intentional — `devlane` is a developer tool, not a multi-tenant service manager.

Port collisions between users on the same host are still possible at the OS level. The live probe handles these the same way it handles any other external process.

The catalog is not portable across machines. Allocations are a function of local host state.

## Relationship to the manifest

The manifest is a snapshot of the catalog's view of one lane. For each port declared in the adapter, the manifest includes the resolved port number and allocation status under `ports.<name>`, and the compose env exports `DEVLANE_PORT_<NAME>` for both compose and templates.

Agents should read ports from the manifest, not from the catalog directly. The catalog is an implementation detail. The manifest is the contract.
