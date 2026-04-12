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
    assert manifest["network"]["projectName"] == "demoapp_feature-test-lane"
    assert manifest["network"]["publicHost"] == "feature-test-lane.demoapp.localhost"
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
