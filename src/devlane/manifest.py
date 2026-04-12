from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .config import AdapterConfig, GeneratedOutput
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


def _render_runtime_env(raw_env: dict[str, Any], values: dict[str, str]) -> dict[str, str]:
    rendered: dict[str, str] = {}
    for key, raw in raw_env.items():
        if raw is None:
            rendered[key] = ""
        elif isinstance(raw, bool):
            rendered[key] = "true" if raw else "false"
        elif isinstance(raw, (int, float)):
            rendered[key] = str(raw)
        else:
            rendered[key] = str(raw).format(**values)
    return rendered


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
    compose_env_path = resolve_path(repo_root, adapter.outputs.compose_env_path)
    state_root = _root_for_lane(repo_root, adapter.lane.path_roots.state, lane_slug)
    cache_root = _root_for_lane(repo_root, adapter.lane.path_roots.cache, lane_slug)
    runtime_root = _root_for_lane(repo_root, adapter.lane.path_roots.runtime, lane_slug)

    compose_files = [str(resolve_path(repo_root, item)) for item in adapter.runtime.compose_files]
    profiles = options.profiles if options.profiles is not None else list(adapter.runtime.default_profiles)

    env_values = {
        "DEVLANE_APP": adapter.app,
        "DEVLANE_APP_SLUG": app_slug,
        "DEVLANE_KIND": adapter.kind,
        "DEVLANE_BRANCH": branch,
        "DEVLANE_MODE": mode,
        "DEVLANE_LANE": raw_lane_name,
        "DEVLANE_LANE_SLUG": lane_slug,
        "DEVLANE_STABLE": "true" if is_stable else "false",
        "DEVLANE_REPO_ROOT": str(repo_root),
        "DEVLANE_CONFIG": str(options.config_path.resolve()),
        "DEVLANE_MANIFEST": str(manifest_path),
        "DEVLANE_COMPOSE_ENV": str(compose_env_path),
        "DEVLANE_STATE_ROOT": state_root,
        "DEVLANE_CACHE_ROOT": cache_root,
        "DEVLANE_RUNTIME_ROOT": runtime_root,
        "DEVLANE_COMPOSE_PROJECT": project_name,
        "DEVLANE_PUBLIC_HOST": public_host or "",
        "DEVLANE_PUBLIC_URL": public_url or "",
    }

    render_values = {
        "app": adapter.app,
        "app_slug": app_slug,
        "branch": branch,
        "branch_slug": slugify(branch),
        "lane_name": raw_lane_name,
        "lane_slug": lane_slug,
        "mode": mode,
        "public_host": public_host or "",
        "public_url": public_url or "",
        "project_name": project_name,
        "state_root": state_root,
        "cache_root": cache_root,
        "runtime_root": runtime_root,
    }
    env_values.update(_render_runtime_env(adapter.runtime.env, render_values))

    generated = [
        {
            "template": str(resolve_path(repo_root, item.template)),
            "destination": str(resolve_path(repo_root, item.destination)),
        }
        for item in adapter.outputs.generated
    ]

    health = None
    if adapter.health.http_url:
        health = {"httpUrl": adapter.health.http_url.format(public_host=public_host or "", public_url=public_url or "")}

    return {
        "schema": 1,
        "app": adapter.app,
        "kind": adapter.kind,
        "repo": {
            "root": str(repo_root),
            "config": str(options.config_path.resolve()),
            "branch": branch,
        },
        "lane": {
            "name": raw_lane_name,
            "slug": lane_slug,
            "mode": mode,
            "stable": is_stable,
        },
        "paths": {
            "manifest": str(manifest_path),
            "composeEnv": str(compose_env_path),
            "stateRoot": state_root,
            "cacheRoot": cache_root,
            "runtimeRoot": runtime_root,
        },
        "network": {
            "projectName": project_name,
            "publicHost": public_host,
            "publicUrl": public_url,
        },
        "compose": {
            "files": compose_files,
            "profiles": profiles,
        },
        "outputs": {
            "generated": generated,
        },
        "health": health,
        "env": env_values,
    }
