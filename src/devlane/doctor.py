from __future__ import annotations

from pathlib import Path

from .config import AdapterConfig
from .compose import docker_available


def run_doctor(adapter: AdapterConfig, config_path: Path) -> list[str]:
    messages: list[str] = []

    if config_path.exists():
        messages.append(f"ok: config exists at {config_path}")
    else:
        messages.append(f"error: config missing at {config_path}")

    if adapter.runtime.compose_files:
        messages.append("ok: adapter declares compose files")
        if docker_available():
            messages.append("ok: docker binary found")
        else:
            messages.append("warn: docker binary not found")
    else:
        messages.append("info: adapter does not declare compose files")

    if adapter.outputs.generated:
        messages.append(f"ok: adapter declares {len(adapter.outputs.generated)} generated output(s)")
    else:
        messages.append("warn: adapter declares no generated outputs")

    return messages
