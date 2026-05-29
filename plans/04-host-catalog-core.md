# Milestone 4: Host Catalog Core

> **Status: shipped (Phase 1 stabilization).** The host config parser, catalog persistence with lock-then-rename atomicity, sticky allocation engine, IPv4/IPv6 probing, catalog-coupled `prepare` orchestration with rollback, and manifest population from catalog state are all in the codebase and exercised by the `portalloc` and `cli` test suites. Current contract: `docs/65-host-catalog.md`; the lifecycle commands that consume it live in `docs/40-cli-contract.md`.

The operator surface built on top of this core — `port`, `host status`, `host doctor`, `host gc`, and `reassign` — shipped in Phase 2; see `plans/05-host-catalog-commands.md`.
