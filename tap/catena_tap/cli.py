"""Catena Coding Agent runtime parser CLI."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from collections.abc import Sequence
from pathlib import Path
from typing import Any

from catena_tap import __version__
from catena_tap.runtime import import_claude_transcript, run_claude_hook

RUNTIMES = ("codex", "claude")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="catena", description="Catena coding-agent evidence runtime")
    parser.add_argument("--version", action="version", version=f"catena-runtime {__version__}")
    commands = parser.add_subparsers(dest="command", required=True)
    trace = commands.add_parser("trace", help="parse or upload coding-agent runtime evidence")
    trace_commands = trace.add_subparsers(dest="trace_command", required=True)

    hook = trace_commands.add_parser("hook", help="handle a fail-open Runtime hook on stdin")
    hook.add_argument("runtime", choices=RUNTIMES)

    importer = trace_commands.add_parser("import", help="parse a historical transcript/rollout")
    importer.add_argument("runtime", choices=RUNTIMES)
    importer.add_argument("path")
    importer.add_argument("--session-id", default="", help="validate the Runtime-native session id")
    importer.add_argument("--otlp", action="store_true", help="render OTLP JSON instead of the canonical graph")
    importer.add_argument("--upload", action="store_true", help="upload to Catena after parsing")
    importer.add_argument("--trace-id", default="", help="Codex-only canonical trace filter")
    return parser


def _read_hook_input() -> dict[str, Any]:
    value = json.load(sys.stdin)
    if not isinstance(value, dict):
        raise ValueError("hook stdin must be a JSON object")
    return value


def _codex_bundle() -> Path:
    override = os.environ.get("CATENA_CODEX_RUNTIME", "").strip()
    if override:
        return Path(override).expanduser()
    return Path(__file__).resolve().parents[1] / "codex" / "plugins" / "tracing" / "dist" / "index.mjs"


def _run_codex(arguments: Sequence[str], *, stdin: Any = None) -> int:
    bundle = _codex_bundle()
    if not bundle.is_file():
        raise FileNotFoundError(f"Codex runtime bundle not found: {bundle}; run pnpm --dir tap/codex build")
    result = subprocess.run(
        ["node", str(bundle), *arguments],
        stdin=stdin,
        check=False,
    )
    return result.returncode


def _run_hook(runtime: str) -> int:
    if runtime == "codex":
        # The TypeScript hook owns Codex rollout parsing. Preserve stdin for
        # the official Codex hook JSON contract and always fail open.
        try:
            return 0 if _run_codex([], stdin=sys.stdin) == 0 else 0
        except Exception as error:
            if os.environ.get("CATENA_TRACE_DEBUG", "").lower() == "true":
                print(f"[catena-runtime] Codex hook failed open: {error}", file=sys.stderr)
            return 0
    try:
        run_claude_hook(_read_hook_input())
    except Exception as error:
        if os.environ.get("CATENA_TRACE_DEBUG", "").lower() == "true":
            print(f"[catena-runtime] Claude hook failed open: {error}", file=sys.stderr)
    return 0


def _run_import(args: argparse.Namespace) -> int:
    if args.runtime == "codex":
        arguments = ["import", args.path]
        if args.otlp:
            arguments.append("--otlp")
        if args.upload:
            arguments.append("--upload")
        if args.trace_id:
            arguments.extend(["--trace-id", args.trace_id])
        return _run_codex(arguments)
    if args.trace_id:
        raise ValueError("--trace-id is Codex-only")
    _, rendered, _, failed = import_claude_transcript(
        args.path,
        expected_session_id=args.session_id or None,
        output="otlp" if args.otlp else "canonical",
        upload=args.upload,
    )
    sys.stdout.write(rendered)
    return 1 if failed else 0


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(list(argv) if argv is not None else None)
    try:
        if args.trace_command == "hook":
            return _run_hook(args.runtime)
        return _run_import(args)
    except (FileNotFoundError, ValueError, OSError, json.JSONDecodeError) as error:
        print(f"catena trace: {error}", file=sys.stderr)
        return 1


def main_entry() -> None:
    raise SystemExit(main())


if __name__ == "__main__":
    main_entry()
