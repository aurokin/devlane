from __future__ import annotations

from pathlib import Path

from devlane.config import load_adapter
from devlane.manifest import ManifestOptions, build_manifest


def test_manifest_uses_branch_as_dev_lane(demo_repo: Path) -> None:
    adapter = load_adapter(demo_repo / "devlane.yaml")
    manifest = build_manifest(
        adapter,
        ManifestOptions(
            cwd=demo_repo,
            config_path=demo_repo / "devlane.yaml",
        ),
    )

    assert manifest["lane"]["mode"] == "dev"
    assert manifest["lane"]["slug"] == "feature-test-lane"
    assert manifest["lane"]["branch"] == "feature/test-lane"
    assert manifest["lane"]["repoRoot"].endswith("demo_repo")
    assert manifest["lane"]["configPath"].endswith("devlane.yaml")
    assert manifest["network"]["projectName"] == "demoapp_feature-test-lane"
    assert manifest["network"]["publicHost"] == "feature-test-lane.demoapp.localhost"
    assert manifest["network"]["publicUrl"] == "http://feature-test-lane.demoapp.localhost"
    assert manifest["paths"]["stateRoot"].endswith(".devlane/state/feature-test-lane")


def test_manifest_can_force_stable_mode(demo_repo: Path) -> None:
    adapter = load_adapter(demo_repo / "devlane.yaml")
    manifest = build_manifest(
        adapter,
        ManifestOptions(
            cwd=demo_repo,
            config_path=demo_repo / "devlane.yaml",
            mode="stable",
        ),
    )

    assert manifest["lane"]["mode"] == "stable"
    assert manifest["lane"]["stable"] is True
    assert manifest["lane"]["name"] == "stable"
    assert manifest["network"]["publicHost"] == "demoapp.localhost"


def test_manifest_has_no_legacy_top_level_fields(demo_repo: Path) -> None:
    adapter = load_adapter(demo_repo / "devlane.yaml")
    manifest = build_manifest(
        adapter,
        ManifestOptions(cwd=demo_repo, config_path=demo_repo / "devlane.yaml"),
    )
    assert "env" not in manifest
    assert "repo" not in manifest
    assert "health" not in manifest


def test_manifest_ports_pre_prepare_projection(demo_repo: Path) -> None:
    adapter = load_adapter(demo_repo / "devlane.yaml")
    manifest = build_manifest(
        adapter,
        ManifestOptions(cwd=demo_repo, config_path=demo_repo / "devlane.yaml"),
    )
    assert manifest["ports"]["web"]["port"] == 3000
    assert manifest["ports"]["web"]["allocated"] is False
    assert manifest["ports"]["web"]["healthUrl"] == "http://localhost:3000/health"


def test_manifest_omits_compose_env_without_compose(tmp_path: Path) -> None:
    config = tmp_path / "devlane.yaml"
    config.write_text(
        """
schema: 1
app: baremetal
kind: web
lane:
  stable_name: stable
  stable_branches: [main]
  project_pattern: "{app}_{lane}"
  path_roots:
    state: .devlane/state
    cache: .devlane/cache
    runtime: .devlane/runtime
runtime:
  compose_files: []
  default_profiles: []
  optional_profiles: []
  env: {}
outputs:
  manifest_path: .devlane/manifest.json
  generated: []
""".strip()
    )
    adapter = load_adapter(config)
    manifest = build_manifest(adapter, ManifestOptions(cwd=tmp_path, config_path=config))
    assert "composeEnv" not in manifest["paths"]
