# Host catalog

The host catalog is the single source of truth for what `devlane`-managed apps exist on this machine, which lanes are registered, and which ports each lane's services are bound to.

It sits alongside per-repo manifests, not inside them. The manifest answers "what is this lane?" The catalog answers "what is this host running?"

## Why it exists

A single repo can isolate its own lanes with the Compose project name. That breaks down the moment you have two repos on the same host, or a mix of containerized and bare-metal apps, because host ports are global and the tool has no way to coordinate across projects without a shared view.

The catalog is that shared view.

## Files

Two files live under `os.UserConfigDir()/devlane/`, with an explicit `XDG_CONFIG_HOME` taking precedence when set:

- `config.yaml` — user-editable configuration (port range, reserved ports)
- `catalog.json` — tool-owned state (allocations)

Keep them separate. The user owns the config. The tool owns the catalog.

Examples in this doc use `~/.config/devlane` as Linux-style shorthand. On macOS the default location is `~/Library/Application Support/devlane` unless `XDG_CONFIG_HOME` is explicitly set.

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

The catalog is tool-owned. Humans and agents should not hand-edit it. Today `prepare` and `reassign` are the shipped commands that mutate it; both go through the same lock-then-rename discipline (`prepare` via its session orchestration, `reassign` via the `Mutate` callback primitive in the port-allocation package).

## Concurrency model

The catalog is shared across every `devlane` invocation on the host. Two `prepare` commands from different terminals or agents can race.

Devlane uses a lock-then-rename write discipline:

1. Acquire an exclusive `fcntl.flock` on `catalog.json.lock` in the devlane user config directory. Acquire timeout is 30 seconds; after that, fail with a message naming the lock-holder's PID where possible.
2. Read `catalog.json`.
3. Compute the new allocation set.
4. Write `catalog.json.tmp`.
5. `os.rename` the temp file over `catalog.json` (atomic on POSIX).
6. Release the lock.

Every code path that mutates the catalog uses this discipline. Today that means `prepare` and `reassign`. Readers such as `inspect` do not take the lock; they read `catalog.json` directly and accept that their view may be one write behind.

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

When stable's `prepare` finds its fixture (`stable_port` when declared, otherwise `default`) already held, the error classifies the holder and — for the two recoverable cases — emits a copy-pasteable recipe. The recipe always points `--cwd` at the **offending** checkout, so it is correct whether the squatting lane belongs to this app or another one.

### Scenario 1: Held by another stable lane

```
stable port 3000 for service "web" is unavailable: it is the stable fixture of lane "stable" (app "otherapp", service "api") at /home/auro/code/otherapp.
Two stable fixtures cannot share a port — choosing which one moves is a human decision.
Edit one adapter's `default`/`stable_port` so the fixtures differ, then re-run `devlane prepare`.
```

Hard error. No command to run — two stable fixtures cannot share a port, so a human picks which adapter moves.

### Scenario 2: Held by a dev lane, port currently free (dev lane offline)

```
stable port 3000 for service "web" is unavailable: it is claimed by dev lane "feature-x" (app "myapp", service "web") at /home/auro/code/myapp-feature-x, which is not currently bound.
Reassign that lane off the fixture, then re-run your original prepare (with the same --cwd/--config/--mode you ran it with):
  devlane reassign --lane feature-x --force --cwd /home/auro/code/myapp-feature-x web    # adjust --cwd/--config if that lane's adapter is not at the checkout root
  devlane prepare
(If /home/auro/code/myapp-feature-x no longer exists, that worktree was removed — drop the stale row with `devlane host gc`, then re-run prepare.)
```

The dev lane only holds the fixture in the catalog, not on the host. `reassign --force` moves it to a fresh pool port; `prepare` then claims the fixture for stable. The recipe's `--cwd` is the offending lane's worktree root — the catalog does not store the adapter's config path, so for a monorepo subtree adapter that lives below the worktree root you point `--cwd`/`--config` at the adapter instead. If that worktree was deleted, the dev row is a stale `missing-repoPath` entry owned by `devlane host doctor`/`host gc`, not `reassign`.

### Scenario 3: Held by a dev lane, port currently bound (a process is listening)

```
stable port 3000 for service "web" is unavailable: it is claimed by dev lane "feature-x" (app "myapp", service "web") at /home/auro/code/myapp-feature-x and the port is currently bound.
Free the port, reassign that lane off the fixture, then re-run your original prepare (with the same --cwd/--config/--mode you ran it with):
  # release port 3000: if dev lane "feature-x" is the listener, run `devlane down` in /home/auro/code/myapp-feature-x; otherwise stop whatever process holds the port
  devlane reassign --lane feature-x --force --cwd /home/auro/code/myapp-feature-x web    # adjust --cwd/--config if that lane's adapter is not at the checkout root
  devlane prepare
(If /home/auro/code/myapp-feature-x no longer exists, that worktree was removed — drop the stale row with `devlane host gc`, then re-run prepare.)
```

A probe can tell the fixture is bound but not by whom — usually the owning dev lane, but possibly an unrelated process — so the recipe covers both. devlane never stops another lane's processes: the listener must release the port before stable can bind it. `reassign --force` then moves the dev lane's catalog row off the fixture — without it, `reassign` would no-op once the port is free, and `prepare` would still see the fixture claimed.

### Other holders: reserved ports and untracked processes

Two non-lane cases produce a plain error rather than a recipe:

- **On the reserved list** — the fixture is in the host or adapter `reserved` set. Reserved is checked before catalog ownership, so this dominates even when a catalog row also sits on the port; reassigning that row would not make the fixture usable. Remove the port from `reserved`, or choose a different stable fixture.
- **Unbindable with no catalog row** — the fixture has no catalog owner and is not reserved, but the OS refuses the bind (a process started outside devlane already holds it, or it is a privileged port). `prepare` surfaces the concrete probe cause (`address already in use`, `permission denied`, …) rather than guessing; free the port or choose a different stable fixture.

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
