from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml


@dataclass(slots=True)
class GeneratedOutput:
    template: str
    destination: str


@dataclass(slots=True)
class LanePaths:
    state: str
    cache: str
    runtime: str


@dataclass(slots=True)
class HostPatterns:
    stable: str | None = None
    dev: str | None = None


@dataclass(slots=True)
class LaneConfig:
    stable_name: str
    stable_branches: list[str]
    project_pattern: str
    path_roots: LanePaths
    host_patterns: HostPatterns = field(default_factory=HostPatterns)


@dataclass(slots=True)
class RuntimeConfig:
    compose_files: list[str]
    default_profiles: list[str]
    optional_profiles: list[str]
    env: dict[str, Any]


@dataclass(slots=True)
class OutputsConfig:
    manifest_path: str
    compose_env_path: str
    generated: list[GeneratedOutput]


@dataclass(slots=True)
class HealthConfig:
    http_url: str | None = None


@dataclass(slots=True)
class AdapterConfig:
    schema: int
    app: str
    kind: str
    lane: LaneConfig
    runtime: RuntimeConfig
    outputs: OutputsConfig
    health: HealthConfig

    @property
    def allowed_profiles(self) -> list[str]:
        return list(dict.fromkeys([*self.runtime.default_profiles, *self.runtime.optional_profiles]))


def _require_dict(data: Any, name: str) -> dict[str, Any]:
    if not isinstance(data, dict):
        raise ValueError(f"{name} must be a mapping")
    return data


def load_adapter(config_path: Path) -> AdapterConfig:
    raw = yaml.safe_load(config_path.read_text())
    data = _require_dict(raw, "adapter")

    schema = int(data.get("schema", 0))
    if schema != 1:
        raise ValueError("only schema: 1 is supported in this scaffold")

    app = str(data["app"])
    kind = str(data["kind"])
    if kind not in {"web", "cli", "hybrid"}:
        raise ValueError("kind must be one of: web, cli, hybrid")

    lane_data = _require_dict(data["lane"], "lane")
    path_roots_data = _require_dict(lane_data["path_roots"], "lane.path_roots")
    host_patterns_data = _require_dict(lane_data.get("host_patterns", {}), "lane.host_patterns")

    lane = LaneConfig(
        stable_name=str(lane_data["stable_name"]),
        stable_branches=[str(x) for x in lane_data.get("stable_branches", [])],
        project_pattern=str(lane_data["project_pattern"]),
        path_roots=LanePaths(
            state=str(path_roots_data["state"]),
            cache=str(path_roots_data["cache"]),
            runtime=str(path_roots_data["runtime"]),
        ),
        host_patterns=HostPatterns(
            stable=host_patterns_data.get("stable"),
            dev=host_patterns_data.get("dev"),
        ),
    )

    runtime_data = _require_dict(data["runtime"], "runtime")
    runtime = RuntimeConfig(
        compose_files=[str(x) for x in runtime_data.get("compose_files", [])],
        default_profiles=[str(x) for x in runtime_data.get("default_profiles", [])],
        optional_profiles=[str(x) for x in runtime_data.get("optional_profiles", [])],
        env={str(k): v for k, v in _require_dict(runtime_data.get("env", {}), "runtime.env").items()},
    )

    outputs_data = _require_dict(data["outputs"], "outputs")
    outputs = OutputsConfig(
        manifest_path=str(outputs_data["manifest_path"]),
        compose_env_path=str(outputs_data["compose_env_path"]),
        generated=[
            GeneratedOutput(template=str(item["template"]), destination=str(item["destination"]))
            for item in outputs_data.get("generated", [])
        ],
    )

    health = HealthConfig(http_url=_require_dict(data.get("health", {}), "health").get("http_url"))

    return AdapterConfig(
        schema=schema,
        app=app,
        kind=kind,
        lane=lane,
        runtime=runtime,
        outputs=outputs,
        health=health,
    )
