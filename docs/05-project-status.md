# Project status

> **Orientation** · Read this first — devlane's status and whether to use it at all. Onward: `10-when-to-use-this.md` (the adoption gate if you do), `00-principles.md` (the design rules).

**devlane is superseded for the author's day-to-day workflow.** It was built as a harness-engineering exercise — a docs-first local-development control plane for parallel agents. Two purpose-built, more polished tools now cover most of what it does, more automatically:

- **[worktrunk](https://worktrunk.dev)** (`github.com/max-sixty/worktrunk`) — a git-worktree manager for running agents in parallel: `wt switch` / `remove` / `list`, create hooks, merge workflow, interactive picker, build-cache copying, PR checkout, and per-worktree dev-server ports (`hash_port`).
- **[portless](https://github.com/vercel-labs/portless)** (from Vercel) — a local reverse proxy that replaces port numbers with stable `https://<app>.localhost` URLs. It auto-assigns the real port, fronts it over HTTPS, and prepends the branch as a subdomain so **each worktree gets its own URL with no config and no collisions**.

If you're picking tooling for a worktree-heavy, parallel-agent workflow, reach for those first.

## Where devlane overlaps — and loses

| devlane capability | Covered by worktrunk + portless? |
| --- | --- |
| Stable local hostnames | **portless, better** — an actual HTTPS proxy; you never touch a port |
| Per-worktree URLs / ports, no collisions | **portless** (branch subdomain) **+ worktrunk** (`hash_port`) |
| Worktree create / remove + seed-file copy | **worktrunk, far richer** — your `worktree.seed` is just an on-create hook |
| Port allocation strategy | **portless sidesteps it** (hostnames are the namespace; `portless alias` even fronts Docker ports) |

The overlapping majority is covered better and more automatically by the two upstream tools. devlane also explicitly **cut proxy integration from its roadmap** and was designed to emit `publicHost` / `publicUrl` into a manifest so you'd bring your own proxy — portless *is* that proxy, and it auto-discovers everything itself, so it doesn't even need the manifest.

## Where devlane still has an edge

Two capabilities have no equivalent in the stack above. Both are narrow:

1. **A machine-wide port catalog with audit and repair.** portless makes ports invisible; worktrunk's `hash_port` is deterministic but uncoordinated. Neither keeps a persistent, lock-coordinated registry of *what is allocated where across every repo and lane* with drift detection (`host doctor`), garbage collection (`host gc`), and `reassign`. This earns its keep only when you run **many services that must bind real fixed host ports** — raw TCP databases you reach by port, non-HTTP protocols a `.localhost` proxy can't front — **across many repos at once**, and you want to audit and repair collisions.
2. **A structured per-lane manifest as an agent contract.** `inspect --json` recomputes a deterministic description of a lane (identity, paths, ports with allocation state, `publicHost`, `ready`, generated-output metadata). That is richer than "the hostname is `<app>.localhost`" — but for most agent tasks the predictable hostname *is* a sufficient discovery contract, which is portless's whole pitch.

## So: should you use devlane?

- **Worktree-heavy Node / web workflow with parallel agents?** Use worktrunk + portless. devlane is redundant.
- **Many fixed-host-port, non-HTTP services across many repos, and you keep hitting cross-repo port collisions you need to audit and repair?** devlane's host catalog is the only thing here that solves that directly — keep it for that, or lift the catalog model into your own tooling.
- **Want a single structured JSON contract describing a lane** (ports + paths + generated outputs) for agents, beyond a hostname? That is devlane-specific.

If none of those bite, treat devlane as archived. The durable value is in the ideas — catalog lock-then-rename discipline, the drift categories, scoped `gc`, the supervised-substrate rule — not in running a third tool alongside two that do the shared parts better.
