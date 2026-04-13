from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .config import AdapterConfig, PortConfig
from .gitutils import current_branch, find_repo_root
from .util import resolve_path, slugify


@dataclass(slots=True)
class ManifestOptions:
    cwd: Path
    config_path: Path
    lane_name: str | None = None
    mode: str | None = None
    profiles: list[str] | None = None


def _render_pattern(pattern: str | None, values: dict[str, str]) -> str | None:
    if not pattern:
        return None
    return pattern.format(**values)


def _root_for_lane(repo_root: Path, raw_root: str, lane_slug: str) -> str:
    return str((resolve_path(repo_root, raw_root) / lane_slug).resolve())


def _port_entry(port: PortConfig, is_stable: bool) -> dict[str, Any]:
    number = port.stable_port if (is_stable and port.stable_port is not None) else port.default
    health_url = f"http://localhost:{number}{port.health_path}" if port.health_path else None
    return {
        "port": number,
        "allocated": False,
        "healthUrl": health_url,
    }


def build_manifest(adapter: AdapterConfig, options: ManifestOptions) -> dict[str, Any]:
    cwd = options.cwd.resolve()
    repo_root = find_repo_root(cwd)
    branch = current_branch(cwd)

    explicit_mode = options.mode
    if explicit_mode is not None and explicit_mode not in {"stable", "dev"}:
        raise ValueError("mode must be 'stable' or 'dev'")

    is_stable = (
        explicit_mode == "stable"
        or (explicit_mode is None and branch in adapter.lane.stable_branches)
    )
    mode = "stable" if is_stable else "dev"

    raw_lane_name = options.lane_name or (adapter.lane.stable_name if is_stable else branch if branch != "detached" else cwd.name)
    lane_slug = slugify(raw_lane_name)
    app_slug = slugify(adapter.app)
    project_name = slugify(
        adapter.lane.project_pattern.format(app=app_slug, lane=lane_slug, mode=mode, branch=slugify(branch)),
        allow_underscore=True,
    )

    pattern_values = {
        "app": app_slug,
        "lane": lane_slug,
        "mode": mode,
        "branch": slugify(branch),
        "project": project_name,
    }
    public_host = _render_pattern(
        adapter.lane.host_patterns.stable if is_stable else adapter.lane.host_patterns.dev,
        pattern_values,
    )
    public_url = f"http://{public_host}" if public_host else None

    manifest_path = resolve_path(repo_root, adapter.outputs.manifest_path)
    state_root = _root_for_lane(repo_root, adapter.lane.path_roots.state, lane_slug)
    cache_root = _root_for_lane(repo_root, adapter.lane.path_roots.cache, lane_slug)
    runtime_root = _root_for_lane(repo_root, adapter.lane.path_roots.runtime, lane_slug)

    paths: dict[str, Any] = {
        "manifest": str(manifest_path),
        "stateRoot": state_root,
        "cacheRoot": cache_root,
        "runtimeRoot": runtime_root,
    }
    if adapter.runtime.compose_files and adapter.outputs.compose_env_path:
        paths["composeEnv"] = str(resolve_path(repo_root, adapter.outputs.compose_env_path))

    compose_files = [str(resolve_path(repo_root, item)) for item in adapter.runtime.compose_files]
    profiles = options.profiles if options.profiles is not None else list(adapter.runtime.default_profiles)

    ports = {port.name: _port_entry(port, is_stable) for port in adapter.ports}

    generated = [
        {
            "template": str(resolve_path(repo_root, item.template)),
            "destination": str(resolve_path(repo_root, item.destination)),
        }
        for item in adapter.outputs.generated
    ]

    return {
        "schema": 1,
        "app": adapter.app,
        "kind": adapter.kind,
        "lane": {
            "name": raw_lane_name,
            "slug": lane_slug,
            "mode": mode,
            "stable": is_stable,
            "branch": branch,
            "repoRoot": str(repo_root),
            "configPath": str(options.config_path.resolve()),
        },
        "paths": paths,
        "network": {
            "projectName": project_name,
            "publicHost": public_host,
            "publicUrl": public_url,
        },
        "ports": ports,
        "compose": {
            "files": compose_files,
            "profiles": profiles,
        },
        "outputs": {
            "generated": generated,
        },
    }
