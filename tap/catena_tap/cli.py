"""The `catena tap` command."""

from __future__ import annotations

import argparse
import os
import sys
from collections.abc import Sequence
from typing import Any

from catena_tap import __version__
from catena_tap.exporter import BackgroundTraceExporter, OTLPHTTPClient, endpoint_from_environment

RUNTIMES = ("codex", "codexapp", "claude", "hermes", "openclaw")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="catena", description="Catena local Agent companion")
    parser.add_argument("--version", action="version", version=f"catena-tap {__version__}")
    subcommands = parser.add_subparsers(dest="command", required=True)
    tap = subcommands.add_parser("tap", help="capture a local Agent Runtime and upload canonical traces")
    tap.add_argument("runtime", choices=RUNTIMES)
    tap.add_argument("--catena-url", default="", help="Catena origin (or CATENA_URL)")
    tap.add_argument("--endpoint", default="", help="full OTLP Trace endpoint override")
    tap.add_argument("--api-key", default="", help="Agent API key (or CATENA_API_KEY)")
    tap.add_argument("--tap-ui", action="store_true", help="keep claude-tap's local viewer enabled")
    tap.add_argument("--debug", action="store_true", help="print exporter diagnostics")
    return parser


def upstream_argv(runtime: str, passthrough: Sequence[str], *, tap_ui: bool) -> list[str]:
    values = ["claude-tap", "--tap-client", runtime]
    if not tap_ui:
        values.extend(["--tap-no-open", "--tap-no-live"])
    tail = list(passthrough)
    if tail and tail[0] == "--":
        tail = tail[1:]
    if tail:
        values.append("--")
        values.extend(tail)
    return values


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args, passthrough = parser.parse_known_args(list(argv) if argv is not None else None)
    if args.command != "tap":
        parser.error("a command is required")

    api_key = args.api_key.strip() or os.environ.get("CATENA_API_KEY", "").strip()
    if not api_key:
        parser.error("Agent API key is required: pass --api-key or set CATENA_API_KEY")
    endpoint = endpoint_from_environment(args.endpoint, args.catena_url)
    debug = bool(args.debug or _truthy(os.environ.get("CATENA_TAP_DEBUG", "")))

    client = OTLPHTTPClient(endpoint, api_key, debug=debug)
    exporter = BackgroundTraceExporter(args.runtime, client)
    original_argv = sys.argv
    restore_hook: Any = None
    print(f"Catena Tap · {args.runtime} → {endpoint}")
    print("Capture is fail-open: Catena connectivity never changes the Agent result.")
    try:
        restore_hook = _install_trace_hook(exporter)
        sys.argv = upstream_argv(args.runtime, passthrough, tap_ui=args.tap_ui)
        from claude_tap.cli import main_entry as claude_tap_main

        try:
            claude_tap_main()
        except SystemExit as exc:
            return _exit_code(exc.code)
        return 0
    finally:
        sys.argv = original_argv
        if restore_hook is not None:
            restore_hook()
        stats = exporter.close()
        print(
            "Catena Tap stopped · "
            f"{stats.captured_records} API records · {stats.uploaded_traces} Turn traces uploaded"
            + (f" · {stats.failed_uploads} failed" if stats.failed_uploads else "")
        )


def _install_trace_hook(exporter: BackgroundTraceExporter):
    from claude_tap.trace_store import TraceStore

    original = TraceStore.append_record

    def append_record(store, session_id, record):
        original(store, session_id, record)
        try:
            exporter.submit(session_id, record)
        except Exception:
            pass

    TraceStore.append_record = append_record

    def restore() -> None:
        TraceStore.append_record = original

    return restore


def _exit_code(value: object) -> int:
    if value is None:
        return 0
    if isinstance(value, int):
        return value
    return 1


def _truthy(value: str) -> bool:
    return value.strip().lower() in {"1", "true", "yes", "on"}


def main_entry() -> None:
    raise SystemExit(main())
