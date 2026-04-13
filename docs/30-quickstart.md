# Quickstart

This is the fastest path to a first concrete success.

## 1. Create a local environment

```bash
python -m venv .venv
source .venv/bin/activate
pip install -e '.[dev]'
```

## 2. Run the tests

```bash
pytest
```

## 3. Inspect the minimal example

```bash
python -m devlane inspect \
  --config examples/minimal-web/devlane.yaml \
  --cwd examples/minimal-web \
  --json
```

You should see a manifest with:

- a lane name
- a compose project name
- derived state/cache/runtime roots
- a `ports` section (the minimal example declares a `web` port; `allocated: false` until `prepare` runs)
- rendered hostnames under `publicHost`/`publicUrl` (the minimal example declares `host_patterns`; adapters without it get `null`)
- generated output paths

## 4. Generate local outputs

```bash
python -m devlane prepare \
  --config examples/minimal-web/devlane.yaml \
  --cwd examples/minimal-web
```

This writes:

- `.devlane/manifest.json`
- `.devlane/compose.env`
- any template-driven files declared in the adapter

If the adapter declares `ports`, `prepare` also allocates real host ports via the catalog at `~/.config/devlane/catalog.json`. Allocations are sticky — they persist across `up`/`down` cycles and machine reboots. See `65-host-catalog.md` for the model.

## 5. Bring the lane up

```bash
python -m devlane up \
  --config examples/minimal-web/devlane.yaml \
  --cwd examples/minimal-web \
  --dry-run
```

Start with `--dry-run` so the exact Compose command is visible. The minimal example declares `compose_files`, so `up` drives Compose. For a bare-metal adapter (no `compose_files`), `up` is a no-op unless the adapter declares `runtime.run`, in which case it prints the rendered commands by default. See `75-baremetal-workflow.md`.

## 6. Adopt a real repo

For a real repo, scaffold an adapter from the repo's current shape:

```bash
cd /path/to/your-repo
python -m devlane init
```

`init` detects whether the repo is containerized (compose file present), bare-metal (framework manifest, no compose), or CLI (neither), and writes a starter `devlane.yaml`. Edit the file to point generated outputs at whatever your repo currently creates by wrappers, shell scripts, or hand-edited `.env.local` flows.

Then make the agent workflow:

1. `inspect --json`
2. `prepare`
3. `up`

That sequence should replace repo-specific guesswork.
