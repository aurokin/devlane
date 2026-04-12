from __future__ import annotations

from devlane.compose import build_compose_command


def test_compose_command_contains_project_and_profiles() -> None:
    manifest = {
        "paths": {"composeEnv": "/tmp/repo/.devlane/compose.env"},
        "network": {"projectName": "demoapp_feature-x"},
        "compose": {
            "files": ["/tmp/repo/compose.yaml", "/tmp/repo/compose.devlane.yaml"],
            "profiles": ["web"],
        },
    }

    command = build_compose_command(manifest, "up", extra_profiles=["db"])

    assert command[:4] == ["docker", "compose", "--env-file", "/tmp/repo/.devlane/compose.env"]
    assert "-p" in command
    assert "demoapp_feature-x" in command
    assert "--profile" in command
    assert "web" in command
    assert "db" in command
    assert command[-2:] == ["up", "-d"]
