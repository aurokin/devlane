# docs index

This docs set is organized for progressive disclosure. Read only as deep as you need.

## Read in this order

1. `../README.md`
2. `00-principles.md` — the design rules that govern every other choice in the tool
3. `10-when-to-use-this.md` — whether devlane is the right fit for your setup
4. `15-tech-stack.md` — implementation language, tooling, and repo policy choices
5. `20-concepts.md`
6. `30-quickstart.md`

Then choose a branch:

- Need the shared tool contract? Read `40-cli-contract.md`.
- Need to adapt a repo? Read `50-adapter-schema.md` and `90-example-integrations.md`.
- Need to wire agents to the tool? Read `60-manifest-contract.md` and `80-agent-playbook.md`.
- Need the host-wide port and lane model? Read `65-host-catalog.md`.
- Need the container pattern? Read `70-container-workflow.md`.
- Need the bare-metal pattern? Read `75-baremetal-workflow.md`.
- Need planning or acceptance context? Read `../plans/README.md`.

## The one-sentence summary

`devlane` gives many repos the same local development mental model by separating a **shared lane lifecycle tool** from a **small per-repo adapter** and a **machine-readable manifest**.
