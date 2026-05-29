# docs index

This docs set is organized for progressive disclosure. Each doc carries a one-line **tier + "read this when"** header; open only as deep as your task needs.

## Shipped surface

`devlane` ships these commands today: `init`, `inspect`, `prepare`, `port`, `up`, `down`, `status`, `doctor`, `host` (`status` / `doctor` / `gc`), `reassign`, and `worktree` (`create` / `remove`). The exact contract for each lives in `40-cli-contract.md`.

## Tiers

The leading number groups each doc by tier — **00–30** orientation, **40–65** reference contracts, **70–90** task playbooks — so the filename alone tells you how deep it sits.

- **Orientation** (read once to understand the model): `00-principles.md`, `10-when-to-use-this.md`, `20-concepts.md`. Situational: `15-tech-stack.md` (contributing to devlane itself), `30-quickstart.md` (first run).
- **Reference contracts** (open on demand): `40-cli-contract.md` (commands), `50-adapter-schema.md` (what a repo declares), `60-manifest-contract.md` (what agents consume), `65-host-catalog.md` (ports, catalog, drift model).
- **Task playbooks** (open for a worked sequence): `70-container-workflow.md`, `75-baremetal-workflow.md`, `80-agent-playbook.md`, `90-example-integrations.md`.

## Route by task

- New to devlane? Skim the Orientation tier, then jump to the contract for your task.
- Changing or calling a command? `40-cli-contract.md`.
- Adapting a repo? `50-adapter-schema.md` + `90-example-integrations.md` + the workflow playbook for your runtime pattern.
- Wiring an agent to the tool? `60-manifest-contract.md` + `80-agent-playbook.md`.
- Working on ports / the catalog / drift (`host *`, `reassign`, `worktree remove`)? `65-host-catalog.md` + `40-cli-contract.md`.
- Containerized pattern? `70-container-workflow.md`. Bare-metal? `75-baremetal-workflow.md`.
- Planning or acceptance context? `../plans/README.md`.

## The one-sentence summary

`devlane` gives many repos the same local development mental model by separating a **shared lane lifecycle tool** from a **small per-repo adapter** and a **machine-readable manifest**.
