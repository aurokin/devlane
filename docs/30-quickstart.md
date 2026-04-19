# Quickstart

This is the fastest path to a first concrete Phase 1 success.

The walkthrough below describes the current scaffold behavior. Host-catalog-backed `ports` / `ready` semantics arrive in Phase 2.

## 1. Create a local environment

```bash
go mod download
go tool gotestsum -- ./...
```

## 2. Run the tests

```bash
go tool gotestsum -- ./...
```

## 3. Inspect the minimal example

```bash
go run ./cmd/devlane inspect \
  --config examples/minimal-web/devlane.yaml \
  --cwd examples/minimal-web \
  --json
```

You should see a manifest with:

- a lane name
- a compose project name
- derived state/cache/runtime roots
- rendered hostnames under `publicHost`/`publicUrl` (the minimal example declares `host_patterns`; adapters without it get `null`)
- generated output paths

## 4. Generate local outputs

```bash
go run ./cmd/devlane prepare \
  --config examples/minimal-web/devlane.yaml \
  --cwd examples/minimal-web
```

This writes:

- `.devlane/manifest.json`
- `.devlane/compose.env`
- any template-driven files declared in the adapter

## 5. Bring the lane up

```bash
go run ./cmd/devlane up \
  --config examples/minimal-web/devlane.yaml \
  --cwd examples/minimal-web \
  --dry-run
```

Start with `--dry-run` so the exact Compose command is visible. The minimal example declares `compose_files`, so `up` drives Compose. For a bare-metal adapter (no `compose_files`), `up` is a no-op unless the adapter declares `runtime.run`, in which case it prints the rendered commands by default. See `75-baremetal-workflow.md`.

`up` does not implicitly run `prepare`. If the adapter depends on generated outputs or `.devlane/compose.env`, run `prepare` first.

## 6. Adopt a real repo

For a real repo, create `devlane.yaml` from the documented adapter schema:

```bash
cd /path/to/your-repo
$EDITOR devlane.yaml
```

`init` detects whether the repo is containerized (compose file present), bare-metal (framework manifest, no compose), or CLI (neither), and writes a starter `devlane.yaml`. If the repo looks mixed, detection stays conservative and points you at `--template hybrid-web`. If the only candidate app root is a descendant of `cwd`, `init` scaffolds there and prints the selected path.

Then make the agent workflow:

1. `inspect --json`
2. `prepare`
3. `up`

That sequence should replace repo-specific guesswork.
