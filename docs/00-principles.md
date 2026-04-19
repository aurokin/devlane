# Principles

These are the load-bearing design rules for `devlane`. They are stable, they explain *why* the tool makes the choices it makes, and they are the filter every new feature or scope expansion has to pass.

If you are staring at a proposal that seems to fit technically but feels off, check it against these principles first.

## 1. State vs processes — the supervised-substrate rule

Devlane owns **state** and reads **process state** freely. It does not spawn or stop **user processes** unless the substrate itself supervises them.

Concretely:

- **Devlane mutates** its own state: the host catalog, per-lane manifests, `.devlane/compose.env`, generated files, worktree seed copies.
- **Devlane executes** commands against supervised substrates: `docker compose up`, `docker compose down`, `docker compose ps`. Compose is already a supervisor — it handles PIDs, logs, restarts, `ps`. Running it for you is safe.
- **Devlane prints** commands for unsupervised processes: `bin/rails server`, `npm run dev`, anything declared in `runtime.run.commands`. Nothing supervises these. Devlane refuses to fire-and-forget them and then pretend they are running.
- **Devlane reads** process state whenever it helps: TCP probes (`devlane port --probe`), `docker compose ps` for status, OS-level port checks. Reads are always allowed.

This rule is what keeps the tool from drifting into being a process manager, a foreman/overmind replacement, or an ad hoc PID tracker. It also kills the asymmetry where `devlane up` means two totally different things in different adapters.

### Hybrid adapters

When an adapter declares both `compose_files` and `runtime.run.commands`, `devlane up` prints the bare-metal commands first, then runs compose. If compose fails, the bare-metal plan is still visible in the terminal.

## 2. Adapters describe, the tool orchestrates

Adapters (`devlane.yaml`) are **data**, not orchestration logic.

- If a repo needs to say "we bind these ports," that is an adapter field.
- If a repo needs to say "we need these commands run before the dev server starts," that is not an adapter concern. Put it in a repo-owned wrapper, a Makefile, or `package.json` scripts.

When a proposal wants to add imperative behavior to the adapter, stop and ask whether the behavior belongs in:

- core lifecycle logic in the tool, or
- a repo-owned wrapper outside the tool

The adapter should never be the place where product logic lives.

## 3. `inspect --json` is the contract; files on disk are snapshots

Agents should read `inspect --json`. It always recomputes the manifest in memory from the adapter plus the current catalog. It never reads `.devlane/manifest.json` off disk.

`.devlane/manifest.json` exists for wrappers, external tools, and humans who want a static file to eyeball. It is a snapshot of what was true at the last `prepare`. It can drift: another process may have reassigned a port, another agent may have run `host gc`, an adapter field may have changed.

Rules of thumb:

- Agents that care about freshness always call `inspect --json`.
- Agents that want to check "is this port actually bindable right this second" use `devlane port <svc> --probe`. That is a different question from "is it allocated."
- The `ready` field in the manifest says "the catalog has entries for every declared port." Fresh from `inspect --json`, snapshot on disk, and orthogonal to bindability.

## 4. Stable is asymmetric

`stable` and `dev` lanes are not interchangeable. Stable lanes own things dev lanes do not:

- stable may own a friendly hostname
- stable may own a global wrapper in `~/.local/bin`
- stable owns **fixture ports** — its declared `stable_port` (or `default` when that is absent) is a promise, not a preference

This asymmetry is enforced at `prepare` time:

- Stable either gets its fixture port or `prepare` fails loudly. No silent fallback to the pool.
- Stable does not evict other lanes to take its fixture. Collisions are surfaced as errors that the user resolves explicitly.
- Dev lanes allocate from the pool and defer to stable.

Silent fallback would defeat the whole point of a fixture. Wrappers, docs, and external tools that cache a stable port keep working across dev-lane churn *because* the fixture is a promise devlane upholds.

## 5. Allocations are sticky

Once a `(app, repoPath, service)` allocation exists, it does not move during ordinary dev-lane churn.

- `up` does not re-probe.
- `down` does not release.
- `prepare` does not re-probe existing allocations.

The usual commands that move a port are `devlane reassign <service>` (explicit repair, scoped, idempotent) and `devlane host gc` (explicit cleanup by staleness heuristics). The one stable-specific exception is the current checkout flipping from dev mode to stable mode: if its existing row is on a dev-only port, `prepare` / `inspect --json` must still honor the stable fixture instead of treating the dev port as authoritative.

Stickiness is what makes lane identity stable across stop/start cycles, worktree shelving, and machine reboots. Agents and external tools can cache port information with confidence. Cacheability is the whole point.

## 6. The tool does not become an application framework

This is the hardest rule to respect because every feature that fails it looks useful in isolation.

Out of scope, permanently:

- proxy integration (devlane emits `publicHost` / `publicUrl`; the user's proxy config consumes them; no direct coupling between devlane and Caddy / Traefik / nginx)
- stable deploy mechanics (deploy hooks, rollback hooks, global wrapper installers — these are per-product concerns)
- process supervision (see principle #1)
- log collection, log shipping, metric collection
- per-worktree git config policy (users configure their own git)
- credential management beyond the explicit `worktree.seed` copy
- CI/CD coordination

In scope:

- lane metadata
- lane lifecycle (create/remove, prepare, supervised up/down)
- orchestration of generated files
- host-wide coordination of ports and lanes
- the manifest contract

When a future feature proposal arrives, check it against this list. If it belongs in the "out of scope" column, decline it and point at this principle.

---

These principles are versioned with the docs. Changes to them are changes to the design center of the tool and should be treated with the same care as breaking changes to the manifest contract.
