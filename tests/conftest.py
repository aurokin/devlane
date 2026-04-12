from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest


@pytest.fixture()
def demo_repo(tmp_path: Path) -> Path:
    source = Path(__file__).parent / "fixtures" / "demo_repo"
    repo = tmp_path / "demo_repo"
    shutil.copytree(source, repo)

    subprocess.run(["git", "init"], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.email", "devlane@example.test"], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "devlane"], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "checkout", "-b", "feature/test-lane"], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "add", "."], cwd=repo, check=True, capture_output=True)
    subprocess.run(["git", "commit", "-m", "initial"], cwd=repo, check=True, capture_output=True)
    return repo
