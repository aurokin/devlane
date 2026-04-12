from __future__ import annotations

import json
from pathlib import Path

from devlane.config import load_adapter
from devlane.manifest import ManifestOptions, build_manifest
from devlane.write import render_outputs, write_compose_env, write_manifest


def test_prepare_writes_manifest_and_rendered_files(demo_repo: Path) -> None:
    adapter = load_adapter(demo_repo / "devlane.yaml")
    manifest = build_manifest(
        adapter,
        ManifestOptions(
            cwd=demo_repo,
            config_path=demo_repo / "devlane.yaml",
        ),
    )

    write_manifest(manifest)
    write_compose_env(manifest)
    render_outputs(manifest)

    manifest_path = Path(manifest["paths"]["manifest"])
    compose_env_path = Path(manifest["paths"]["composeEnv"])
    rendered_env = demo_repo / ".devlane" / "generated" / "app.env"

    assert manifest_path.exists()
    assert compose_env_path.exists()
    assert rendered_env.exists()

    payload = json.loads(manifest_path.read_text())
    assert payload["network"]["publicHost"] == "feature-test-lane.demoapp.localhost"
    assert "DEVLANE_PUBLIC_HOST=feature-test-lane.demoapp.localhost" in compose_env_path.read_text()
    assert "DEVLANE_LANE=feature/test-lane" in rendered_env.read_text()
