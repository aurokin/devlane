from __future__ import annotations

import subprocess
from pathlib import Path


def _run_git(args: list[str], cwd: Path) -> str:
    proc = subprocess.run(
        ["git", *args],
        cwd=str(cwd),
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip() or proc.stdout.strip() or "git command failed")
    return proc.stdout.strip()


def find_repo_root(cwd: Path) -> Path:
    try:
        return Path(_run_git(["rev-parse", "--show-toplevel"], cwd)).resolve()
    except RuntimeError:
        return cwd.resolve()


def current_branch(cwd: Path) -> str:
    try:
        branch = _run_git(["branch", "--show-current"], cwd)
        return branch or "detached"
    except RuntimeError:
        return "detached"
