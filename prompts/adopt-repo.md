# Prompt: adopt a repo into devlane

You are adapting an existing repository to the `devlane` lane model.

## Read first

1. `AGENTS.md`
2. `README.md`
3. `docs/README.md`
4. `docs/50-adapter-schema.md`
5. `docs/60-manifest-contract.md`
6. `docs/65-host-catalog.md`
7. `docs/75-baremetal-workflow.md` (default) and `docs/70-container-workflow.md` (opt-in), whichever matches the repo
8. `docs/90-example-integrations.md`

## Goal

Add a small declarative `devlane.yaml` to the target repo and make the shared tool capable of:

- deriving a lane manifest
- generating the repo's local runtime files
- starting the repo through lane-aware Compose lifecycle when applicable

## Constraints

- keep repo-specific behavior in the adapter or repo-owned templates
- do not add app-specific logic to the core unless it is clearly generalizable
- generated files should be tool-owned
- stable owns global names
- prefer hostname discovery over port guesswork for HTTP apps

## Deliverables

- `devlane.yaml`
- any repo-owned templates needed by `prepare`
- updated docs explaining stable vs dev ownership in that repo
- any minimal core changes required to support the adapter cleanly
- tests or acceptance notes

## Workflow

1. inspect the current repo for generated env/config files and local wrapper scripts
2. map those files into `outputs.generated`
3. define lane naming, hostname, and Compose project patterns
4. make `python -m devlane inspect --json` meaningful in the repo
5. make `python -m devlane prepare` recreate the current generated local files
6. if the repo uses Compose, make `up` and `down` lane-aware
7. update docs and examples

## Definition of done

An agent entering the repo should be able to use the manifest instead of repo-specific port heuristics.
