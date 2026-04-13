from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .config import AdapterConfig
from .renderer import render_file
from .util import ensure_parent, slugify


def write_manifest(manifest: dict[str, Any]) -> None:
    path = Path(manifest["paths"]["manifest"])
    ensure_parent(path)
    path.write_text(json.dumps(manifest, indent=2) + "\n")


def write_compose_env(manifest: dict[str, Any], adapter: AdapterConfig) -> None:
    compose_env_path = manifest["paths"].get("composeEnv")
    if not compose_env_path:
        return
    env = compute_env(manifest, adapter)
    lines = [f"{key}={value}" for key, value in sorted(env.items())]
    path = Path(compose_env_path)
    ensure_parent(path)
    path.write_text("\n".join(lines) + "\n")


def compute_env(manifest: dict[str, Any], adapter: AdapterConfig) -> dict[str, str]:
    lane = manifest["lane"]
    paths = manifest["paths"]
    network = manifest["network"]
    ports = manifest["ports"]

    env: dict[str, str] = {
        "DEVLANE_APP": adapter.app,
        "DEVLANE_APP_SLUG": slugify(adapter.app),
        "DEVLANE_KIND": adapter.kind,
        "DEVLANE_BRANCH": lane["branch"],
        "DEVLANE_MODE": lane["mode"],
        "DEVLANE_LANE": lane["name"],
        "DEVLANE_LANE_SLUG": lane["slug"],
        "DEVLANE_STABLE": "true" if lane["stable"] else "false",
        "DEVLANE_REPO_ROOT": lane["repoRoot"],
        "DEVLANE_CONFIG": lane["configPath"],
        "DEVLANE_MANIFEST": paths["manifest"],
        "DEVLANE_COMPOSE_ENV": paths.get("composeEnv", ""),
        "DEVLANE_STATE_ROOT": paths["stateRoot"],
        "DEVLANE_CACHE_ROOT": paths["cacheRoot"],
        "DEVLANE_RUNTIME_ROOT": paths["runtimeRoot"],
        "DEVLANE_COMPOSE_PROJECT": network["projectName"],
        "DEVLANE_PUBLIC_HOST": network["publicHost"] or "",
        "DEVLANE_PUBLIC_URL": network["publicUrl"] or "",
    }
    for name, entry in ports.items():
        env[f"DEVLANE_PORT_{name.upper()}"] = str(entry["port"])

    render_values = {
        "app": adapter.app,
        "app_slug": slugify(adapter.app),
        "branch": lane["branch"],
        "branch_slug": slugify(lane["branch"]),
        "lane_name": lane["name"],
        "lane_slug": lane["slug"],
        "mode": lane["mode"],
        "public_host": network["publicHost"] or "",
        "public_url": network["publicUrl"] or "",
        "project_name": network["projectName"],
        "state_root": paths["stateRoot"],
        "cache_root": paths["cacheRoot"],
        "runtime_root": paths["runtimeRoot"],
    }
    for key, raw in adapter.runtime.env.items():
        env[key] = _render_env_value(raw, render_values)

    return env


def _render_env_value(raw: Any, values: dict[str, str]) -> str:
    if raw is None:
        return ""
    if isinstance(raw, bool):
        return "true" if raw else "false"
    if isinstance(raw, (int, float)):
        return str(raw)
    return str(raw).format(**values)


def template_context(manifest: dict[str, Any], adapter: AdapterConfig) -> dict[str, Any]:
    flat_ports = {name: entry["port"] for name, entry in manifest["ports"].items()}
    return {
        "app": manifest["app"],
        "kind": manifest["kind"],
        "lane": manifest["lane"],
        "paths": manifest["paths"],
        "network": manifest["network"],
        "compose": manifest["compose"],
        "outputs": manifest["outputs"],
        "ports": flat_ports,
        "env": compute_env(manifest, adapter),
    }


def render_outputs(manifest: dict[str, Any], adapter: AdapterConfig) -> None:
    context = template_context(manifest, adapter)
    for item in manifest["outputs"]["generated"]:
        render_file(Path(item["template"]), Path(item["destination"]), context)
