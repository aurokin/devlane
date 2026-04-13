from __future__ import annotations

from pathlib import Path

from devlane.config import AdapterConfig, HostPatterns, LaneConfig, LanePaths, OutputsConfig, RuntimeConfig
from devlane.renderer import render_text
from devlane.write import template_context


def _adapter() -> AdapterConfig:
    return AdapterConfig(
        schema=1,
        app="demoapp",
        kind="web",
        lane=LaneConfig(
            stable_name="stable",
            stable_branches=["main"],
            project_pattern="{app}_{lane}",
            path_roots=LanePaths(state=".devlane/state", cache=".devlane/cache", runtime=".devlane/runtime"),
            host_patterns=HostPatterns(stable="{app}.localhost", dev="{lane}.{app}.localhost"),
        ),
        runtime=RuntimeConfig(compose_files=[], default_profiles=[], optional_profiles=[], env={}),
        outputs=OutputsConfig(manifest_path=".devlane/manifest.json", compose_env_path=None, generated=[]),
    )


def test_render_text_uses_dot_paths() -> None:
    rendered = render_text(
        "lane={{lane.slug}} host={{network.publicHost}}",
        {
            "lane": {"slug": "feature-x"},
            "network": {"publicHost": "feature-x.demoapp.localhost"},
        },
    )
    assert rendered == "lane=feature-x host=feature-x.demoapp.localhost"


def test_template_context_exposes_manifest_sections() -> None:
    manifest = {
        "app": "demoapp",
        "kind": "web",
        "lane": {
            "name": "stable",
            "slug": "stable",
            "mode": "stable",
            "stable": True,
            "branch": "main",
            "repoRoot": "/tmp/repo",
            "configPath": "/tmp/repo/devlane.yaml",
        },
        "paths": {"manifest": "a", "stateRoot": "c", "cacheRoot": "d", "runtimeRoot": "e"},
        "network": {"projectName": "demoapp_stable", "publicHost": "demoapp.localhost", "publicUrl": "http://demoapp.localhost"},
        "compose": {"files": [], "profiles": []},
        "outputs": {"generated": []},
        "ports": {},
    }
    context = template_context(manifest, _adapter())
    assert context["network"]["publicHost"] == "demoapp.localhost"
    assert context["lane"]["slug"] == "stable"
    assert context["lane"]["repoRoot"] == "/tmp/repo"
    assert context["env"]["DEVLANE_APP"] == "demoapp"
    assert context["env"]["DEVLANE_PUBLIC_HOST"] == "demoapp.localhost"
