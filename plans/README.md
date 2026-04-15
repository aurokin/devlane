# Plans

This folder breaks the rewrite into milestone-sized plans.

- `01-contract-core.md` — rebuild the contract engine and make `inspect` / `prepare` authoritative
- `02-init.md` — ship `devlane init` for zero-friction adoption
- `03-lifecycle.md` — finish `up` / `down` / `status` / `doctor` behavior across containerized, bare-metal, and hybrid adapters
- `04-host-catalog-core.md` — build host config, catalog state, locking, probing, and allocation
- `05-host-catalog-commands.md` — add `port`, `reassign`, `host status`, `host doctor`, and `host gc`
- `06-worktree-lifecycle.md` — add `worktree create` / `remove` and `worktree.seed`
- `07-hardening-acceptance.md` — close gaps, sync docs and schemas, and drive the acceptance checklist to green

The docs and schemas remain the source of truth for the product contract. These plan documents translate that contract into implementation milestones for the rewrite.
