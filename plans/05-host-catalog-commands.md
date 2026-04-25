# Milestone 5: Host Catalog Operator Commands

Execution state is tracked in the Linear milestone "Phase 2: Host Catalog Operator Commands" (AUR-126 through AUR-133). Per-issue acceptance criteria live in Linear.

The shape of the work is summarized in `plans/phase-roadmap.md` Phase 2 section. The operator-facing contract lives in `docs/40-cli-contract.md` (commands flip from "Not shipped" to shipped as each issue lands), `docs/65-host-catalog.md` (collision recovery), and `docs/80-agent-playbook.md` (agent conflict-handling).

## Linear issues

- **AUR-126** Catalog API: exported `Allocation` + `List()` (foundation; unblocks AUR-128 / 129 / 130 / 131)
- **AUR-127** Windows catalog-lock error copy upgrade (independent)
- **AUR-128** `devlane port` command
- **AUR-129** `devlane host status` command
- **AUR-130** Drift detection module + `devlane host doctor`
- **AUR-131** `Mutate(fn)` API + lane resolver + `devlane reassign`
- **AUR-132** `devlane host gc` command (uses drift detection from AUR-130 and `Mutate` from AUR-131)
- **AUR-133** Stable-port collision message upgrade + `docs/65-host-catalog.md` + `docs/80-agent-playbook.md` updates (depends on AUR-131 so recipes can name `reassign --lane … --force`)

After AUR-126 lands, AUR-128 / 129 / 130 / 131 can run in parallel. AUR-127 is independent of the foundation work.

## Out of scope

- worktree lifecycle (Phase 3)
- any automatic background cleanup (would conflict with `AGENTS.md` non-negotiable #11)
- `up` gating on pure ports-only bare-metal adapters (intentionally deferred; current no-op behavior preserved)
- Windows catalog locking implementation (only the error message changes; underlying capability remains deferred)
