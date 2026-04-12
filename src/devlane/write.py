from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .renderer import render_file
from .util import ensure_parent


def write_manifest(manifest: dict[str, Any]) -> None:
    path = Path(manifest["paths"]["manifest"])
    ensure_parent(path)
    path.write_text(json.dumps(manifest, indent=2) + "\n")


def write_compose_env(manifest: dict[str, Any]) -> None:
    path = Path(manifest["paths"]["composeEnv"])
    ensure_parent(path)
    lines = [f"{key}={value}" for key, value in sorted(manifest["env"].items())]
    path.write_text("\n".join(lines) + "\n")


def template_context(manifest: dict[str, Any]) -> dict[str, Any]:
    network = manifest.get("network", {})
    return {
        "app": manifest.get("app"),
        "kind": manifest.get("kind"),
        "repo": manifest.get("repo"),
        "lane": manifest.get("lane"),
        "paths": manifest.get("paths"),
        "network": network,
        "compose": manifest.get("compose"),
        "outputs": manifest.get("outputs"),
        "health": manifest.get("health"),
        "env": manifest.get("env"),
    }


def render_outputs(manifest: dict[str, Any]) -> None:
    context = template_context(manifest)
    for item in manifest["outputs"]["generated"]:
        render_file(Path(item["template"]), Path(item["destination"]), context)
