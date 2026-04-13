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
    compose_env_path: str | None
    generated: list[GeneratedOutput]


@dataclass(slots=True)
class PortConfig:
    name: str
    default: int
    health_path: str | None = None
    stable_port: int | None = None
    pool_hint: tuple[int, int] | None = None


@dataclass(slots=True)
class AdapterConfig:
    schema: int
    app: str
    kind: str
    lane: LaneConfig
    runtime: RuntimeConfig
    outputs: OutputsConfig
    ports: list[PortConfig] = field(default_factory=list)
    reserved: list[int] = field(default_factory=list)

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

    runtime_data = _require_dict(data.get("runtime", {}), "runtime")
    runtime = RuntimeConfig(
        compose_files=[str(x) for x in runtime_data.get("compose_files", [])],
        default_profiles=[str(x) for x in runtime_data.get("default_profiles", [])],
        optional_profiles=[str(x) for x in runtime_data.get("optional_profiles", [])],
        env={str(k): v for k, v in _require_dict(runtime_data.get("env", {}), "runtime.env").items()},
    )

    outputs_data = _require_dict(data["outputs"], "outputs")
    compose_env_raw = outputs_data.get("compose_env_path")
    outputs = OutputsConfig(
        manifest_path=str(outputs_data["manifest_path"]),
        compose_env_path=str(compose_env_raw) if compose_env_raw else None,
        generated=[
            GeneratedOutput(template=str(item["template"]), destination=str(item["destination"]))
            for item in outputs_data.get("generated", [])
        ],
    )

    ports: list[PortConfig] = []
    seen_names: set[str] = set()
    for raw_port in data.get("ports", []) or []:
        port_data = _require_dict(raw_port, "ports[]")
        name = str(port_data["name"])
        if name in seen_names:
            raise ValueError(f"duplicate port name: {name}")
        seen_names.add(name)
        hint_raw = port_data.get("pool_hint")
        pool_hint: tuple[int, int] | None = None
        if hint_raw is not None:
            if not isinstance(hint_raw, list) or len(hint_raw) != 2:
                raise ValueError(f"ports[{name}].pool_hint must be a [low, high] pair")
            pool_hint = (int(hint_raw[0]), int(hint_raw[1]))
            if pool_hint[0] > pool_hint[1]:
                raise ValueError(f"ports[{name}].pool_hint low must be <= high")
        ports.append(
            PortConfig(
                name=name,
                default=int(port_data["default"]),
                health_path=(str(port_data["health_path"]) if port_data.get("health_path") else None),
                stable_port=(int(port_data["stable_port"]) if port_data.get("stable_port") is not None else None),
                pool_hint=pool_hint,
            )
        )

    reserved = [int(p) for p in data.get("reserved", []) or []]

    return AdapterConfig(
        schema=schema,
        app=app,
        kind=kind,
        lane=lane,
        runtime=runtime,
        outputs=outputs,
        ports=ports,
        reserved=reserved,
    )
