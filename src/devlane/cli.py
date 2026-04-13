from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from .compose import build_compose_command, run_compose
from .config import load_adapter
from .doctor import run_doctor
from .manifest import ManifestOptions, build_manifest
from .write import render_outputs, write_compose_env, write_manifest


def _resolve_config(raw: str, cwd: Path) -> Path:
    path = Path(raw).expanduser()
    if path.is_absolute():
        return path
    direct = path.resolve()
    if direct.exists():
        return direct
    return (cwd / path).resolve()


def _common_parser(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--config", default="devlane.yaml", help="Path to devlane.yaml")
    parser.add_argument("--cwd", default=".", help="Working directory used for discovery")
    parser.add_argument("--lane", help="Override lane name")
    parser.add_argument("--mode", choices=["stable", "dev"], help="Override lane mode")
    parser.add_argument("--profile", action="append", default=[], help="Extra compose profile(s)")


def _load(args: argparse.Namespace):
    cwd = Path(args.cwd).expanduser().resolve()
    config_path = _resolve_config(args.config, cwd)
    adapter = load_adapter(config_path)
    manifest = build_manifest(
        adapter,
        ManifestOptions(
            cwd=cwd,
            config_path=config_path,
            lane_name=args.lane,
            mode=args.mode,
            profiles=None if not args.profile else args.profile,
        ),
    )
    return cwd, config_path, adapter, manifest


def _prepare(manifest: dict, adapter) -> None:
    write_manifest(manifest)
    write_compose_env(manifest, adapter)
    render_outputs(manifest, adapter)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="devlane", description="Lane-oriented local development control plane")
    subparsers = parser.add_subparsers(dest="command", required=True)

    inspect_parser = subparsers.add_parser("inspect", help="Print the derived manifest")
    _common_parser(inspect_parser)
    inspect_parser.add_argument("--json", action="store_true", help="Print JSON output")

    prepare_parser = subparsers.add_parser("prepare", help="Write manifest, compose env, and generated outputs")
    _common_parser(prepare_parser)

    up_parser = subparsers.add_parser("up", help="Run docker compose up for the lane")
    _common_parser(up_parser)
    up_parser.add_argument("--dry-run", action="store_true", help="Print the compose command without executing it")

    down_parser = subparsers.add_parser("down", help="Run docker compose down for the lane")
    _common_parser(down_parser)
    down_parser.add_argument("--dry-run", action="store_true", help="Print the compose command without executing it")

    status_parser = subparsers.add_parser("status", help="Run docker compose ps for the lane")
    _common_parser(status_parser)
    status_parser.add_argument("--dry-run", action="store_true", help="Print the compose command without executing it")

    doctor_parser = subparsers.add_parser("doctor", help="Check obvious prerequisites")
    _common_parser(doctor_parser)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    cwd, config_path, adapter, manifest = _load(args)

    if args.command == "inspect":
        if args.json:
            print(json.dumps(manifest, indent=2))
        else:
            print(f"app: {manifest['app']}")
            print(f"kind: {manifest['kind']}")
            print(f"lane: {manifest['lane']['name']} ({manifest['lane']['mode']})")
            print(f"project: {manifest['network']['projectName']}")
            print(f"public_url: {manifest['network']['publicUrl'] or '-'}")
            print(f"manifest: {manifest['paths']['manifest']}")
        return 0

    if args.command == "prepare":
        _prepare(manifest, adapter)
        print(f"prepared lane '{manifest['lane']['name']}' at {manifest['paths']['manifest']}")
        return 0

    if args.command in {"up", "down", "status"}:
        _prepare(manifest, adapter)
        command = build_compose_command(manifest, args.command)
        print(" ".join(command))
        if args.dry_run:
            return 0
        return run_compose(command, cwd=str(cwd))

    if args.command == "doctor":
        messages = run_doctor(adapter, config_path)
        for message in messages:
            print(message)
        return 0

    parser.error("unhandled command")
    return 2
