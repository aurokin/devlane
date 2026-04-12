from __future__ import annotations

import shutil
import subprocess
from typing import Any


def build_compose_command(manifest: dict[str, Any], action: str, extra_profiles: list[str] | None = None, extra_args: list[str] | None = None) -> list[str]:
    compose_files = manifest["compose"]["files"]
    if not compose_files:
        raise ValueError("adapter does not declare any compose files")

    command = ["docker", "compose", "--env-file", manifest["paths"]["composeEnv"], "-p", manifest["network"]["projectName"]]
    for compose_file in compose_files:
        command.extend(["-f", compose_file])

    profiles = list(manifest["compose"]["profiles"])
    if extra_profiles:
        for item in extra_profiles:
            if item not in profiles:
                profiles.append(item)

    for profile in profiles:
        command.extend(["--profile", profile])

    if action == "up":
        command.extend(["up", "-d"])
    elif action == "down":
        command.append("down")
    elif action == "status":
        command.append("ps")
    else:
        raise ValueError(f"unsupported compose action: {action}")

    if extra_args:
        command.extend(extra_args)
    return command


def docker_available() -> bool:
    return shutil.which("docker") is not None


def run_compose(command: list[str], cwd: str) -> int:
    proc = subprocess.run(command, cwd=cwd, check=False)
    return int(proc.returncode)
