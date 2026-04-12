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
- a public hostname
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

Start with `--dry-run` so the exact Compose command is visible.

## 6. Adopt a real repo

For a real repo, add `devlane.yaml` and point its generated outputs at the files that are currently created by wrappers, shell scripts, or hand-edited `.env.local` flows.

Then make the agent workflow:

1. `inspect --json`
2. `prepare`
3. `up`

That sequence should replace repo-specific guesswork.
