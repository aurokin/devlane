# Prompt: adopt a repo into devlane

You are adapting an existing repository to the `devlane` lane model.

## Read first

1. `AGENTS.md`
2. `README.md`
3. `docs/README.md`
4. `docs/00-principles.md`
5. `docs/10-when-to-use-this.md`
6. `docs/50-adapter-schema.md`
7. `docs/60-manifest-contract.md`
8. `docs/65-host-catalog.md`
9. `docs/75-baremetal-workflow.md` (default) and `docs/70-container-workflow.md` (opt-in), whichever matches the repo
10. `docs/80-agent-playbook.md`
11. `docs/90-example-integrations.md`

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
- prefer the manifest (`ports.<service>.port`, rendered hostnames if declared) over repo-specific port or hostname guesswork

## Deliverables

- `devlane.yaml`
- any repo-owned templates needed by `prepare`
- updated docs explaining stable vs dev ownership in that repo
- any minimal core changes required to support the adapter cleanly
- tests or acceptance notes

## Workflow

1. run `devlane init` to scaffold a starter `devlane.yaml` (use `--from examples/<example>/devlane.yaml` if one is a closer fit than auto-detection)
2. inspect the current repo for generated env/config files and local wrapper scripts
3. map those files into `outputs.generated`
4. define lane naming, hostname, and Compose project patterns
5. ask the repo owner which files belong in `worktree.seed` (credentials, `.env.secrets`, master keys — files that cannot be templated because they are per-developer or per-deploy). Declare them explicitly; no defaults.
6. make `python -m devlane inspect --json` meaningful in the repo; check `ready` reflects allocation state accurately
7. make `python -m devlane prepare` recreate the current generated local files
8. if the repo uses Compose, make `up` and `down` lane-aware
9. if the repo has bare-metal dev commands (`npm run dev`, `bin/rails server`), declare them under `runtime.run.commands` so `devlane up` can print them — devlane never spawns bare processes
10. update docs and examples

## Definition of done

An agent entering the repo should be able to use the manifest instead of repo-specific port heuristics.
