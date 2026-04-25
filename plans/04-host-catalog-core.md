# Milestone 4: Host Catalog Core

This milestone shipped during Phase 1 stabilization. The host config parser, catalog persistence with lock-then-rename atomicity, sticky allocation engine, IPv4/IPv6 probing, catalog-coupled `prepare` orchestration with rollback, and manifest population from catalog state are all in the codebase today and exercised by the existing portalloc and CLI tests.

Read `docs/65-host-catalog.md` for the current contract and `docs/40-cli-contract.md` for the lifecycle commands that consume it.

The remaining host-catalog work — operator commands plus polish — is tracked in the Linear milestone "Phase 2: Host Catalog Operator Commands" (AUR-126 through AUR-133), summarized in `plans/phase-roadmap.md` Phase 2 section, and indexed in `plans/05-host-catalog-commands.md`.
