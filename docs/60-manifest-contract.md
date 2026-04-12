# Manifest contract

The manifest is the shared language between humans, agents, wrappers, and automation.

## Example shape

```json
{
  "schema": 1,
  "app": "demoapp",
  "kind": "web",
  "repo": {
    "root": "/repo/path",
    "config": "/repo/path/devlane.yaml",
    "branch": "feature-x"
  },
  "lane": {
    "name": "feature-x",
    "slug": "feature-x",
    "mode": "dev",
    "stable": false
  },
  "paths": {
    "manifest": "/repo/path/.devlane/manifest.json",
    "composeEnv": "/repo/path/.devlane/compose.env",
    "stateRoot": "/repo/path/.devlane/state/feature-x",
    "cacheRoot": "/repo/path/.devlane/cache/feature-x",
    "runtimeRoot": "/repo/path/.devlane/runtime/feature-x"
  },
  "network": {
    "projectName": "demoapp_feature-x",
    "publicHost": "feature-x.demoapp.localhost",
    "publicUrl": "http://feature-x.demoapp.localhost"
  },
  "compose": {
    "files": ["/repo/path/compose.yaml"],
    "profiles": ["web"]
  },
  "outputs": {
    "generated": [
      {
        "template": "/repo/path/templates/app.env.tmpl",
        "destination": "/repo/path/.devlane/generated/app.env"
      }
    ]
  },
  "env": {
    "DEVLANE_APP": "demoapp",
    "DEVLANE_LANE": "feature-x"
  }
}
```

## Required qualities

The manifest should be:

- deterministic
- JSON-serializable
- easy to diff
- safe for agents to consume
- broad enough to drive template rendering and Compose lifecycle

## Why agents should consume the manifest

If agents read the manifest, they do not need to know:

- which repo-specific env file exists
- which stable/worktree variable names the repo chose
- which hostname pattern the repo uses
- where the runtime, state, or cache directories live

The manifest centralizes those answers.

## Stability policy

Treat manifest fields as contract surface.

- adding fields is usually safe
- renaming or removing fields is a breaking change
- changing semantics without documentation is not acceptable

Keep `schemas/manifest.schema.json` current when the contract changes.
