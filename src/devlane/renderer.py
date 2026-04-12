from __future__ import annotations

import re
from pathlib import Path
from typing import Any

from .util import deep_get, ensure_parent


TOKEN_RE = re.compile(r"\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}")


def render_text(template: str, context: dict[str, Any]) -> str:
    def replace(match: re.Match[str]) -> str:
        path = match.group(1)
        value = deep_get(context, path)
        return "" if value is None else str(value)

    return TOKEN_RE.sub(replace, template)


def render_file(template_path: Path, destination_path: Path, context: dict[str, Any]) -> None:
    ensure_parent(destination_path)
    rendered = render_text(template_path.read_text(), context)
    destination_path.write_text(rendered)
