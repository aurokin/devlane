# Plans

Planning and acceptance material that supports implementation work. **These are not the product contract** — read `docs/` first for current behavior, and treat Linear as the execution source of truth. Use the files here when you need durable in-repo planning detail or the original milestone reasoning.

Phases 1–3 are shipped. Each milestone doc leads with a `> **Status**` banner; the body is kept as the historical plan.

## Milestone docs

- `01-contract-core.md` — Phase 1: adapter / lane / manifest core behind `inspect` and `prepare` · **shipped**
- `02-init.md` — Phase 1: `devlane init` adoption flow · **shipped**
- `03-lifecycle.md` — Phase 1: `up` / `down` / `status` / `doctor` · **shipped**
- `04-host-catalog-core.md` — Phase 1 stabilization: catalog persistence + allocation engine · **shipped**
- `05-host-catalog-commands.md` — Phase 2: `port`, `host status` / `doctor` / `gc`, `reassign` · **shipped**
- `06-worktree-lifecycle.md` — Phase 3: `worktree create` / `remove` · **shipped**
- `07-hardening-acceptance.md` — closeout: examples/docs sync, dead-code removal, acceptance audit · **ongoing**

## Living references

- `phase-roadmap.md` — the phased plan: what shipped, what was cut, and the unscheduled deep roadmap
- `acceptance-checklist.md` — the target acceptance bar, layered into global invariants, per-phase gates, and the adoption bar
