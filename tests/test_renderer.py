from __future__ import annotations

from pathlib import Path

from devlane.renderer import render_text
from devlane.write import template_context


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
    context = template_context(
        {
            "app": "demoapp",
            "kind": "web",
            "repo": {"root": "/tmp/repo", "config": "/tmp/repo/devlane.yaml", "branch": "main"},
            "lane": {"name": "stable", "slug": "stable", "mode": "stable", "stable": True},
            "paths": {"manifest": "a", "composeEnv": "b", "stateRoot": "c", "cacheRoot": "d", "runtimeRoot": "e"},
            "network": {"projectName": "demoapp_stable", "publicHost": "demoapp.localhost", "publicUrl": "http://demoapp.localhost"},
            "compose": {"files": [], "profiles": []},
            "outputs": {"generated": []},
            "health": None,
            "env": {"DEVLANE_APP": "demoapp"},
        }
    )
    assert context["network"]["publicHost"] == "demoapp.localhost"
    assert context["lane"]["slug"] == "stable"
