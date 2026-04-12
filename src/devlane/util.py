from __future__ import annotations

import re
from pathlib import Path
from typing import Any


def slugify(value: str, allow_underscore: bool = False) -> str:
    text = value.strip().lower()
    if allow_underscore:
        text = re.sub(r"[^a-z0-9_-]+", "-", text)
        text = re.sub(r"-{2,}", "-", text)
        text = re.sub(r"_{2,}", "_", text)
        return text.strip("-_") or "lane"
    text = re.sub(r"[^a-z0-9]+", "-", text)
    text = re.sub(r"-{2,}", "-", text)
    return text.strip("-") or "lane"


def ensure_parent(path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)


def resolve_path(base: Path, raw: str) -> Path:
    expanded = Path(raw).expanduser()
    if expanded.is_absolute():
        return expanded
    return (base / expanded).resolve()


def deep_get(mapping: dict[str, Any], path: str) -> Any:
    current: Any = mapping
    for part in path.split("."):
        if isinstance(current, dict) and part in current:
            current = current[part]
            continue
        raise KeyError(path)
    return current
