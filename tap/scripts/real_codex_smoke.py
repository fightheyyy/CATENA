"""Run one real Codex CLI turn through the committed Catena Stop hook."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


class Receiver(BaseHTTPRequestHandler):
    payloads: list[dict] = []

    def do_POST(self) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        self.__class__.payloads.append(json.loads(self.rfile.read(length)))
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b"{}")

    def log_message(self, _format: str, *_args) -> None:
        return


def run(command: list[str], environment: dict[str, str], *, timeout: int = 180) -> None:
    result = subprocess.run(command, env=environment, text=True, timeout=timeout, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"{command[0]} exited with {result.returncode}")


def main() -> int:
    codex = shutil.which("codex")
    if not codex:
        print("codex is not installed", file=sys.stderr)
        return 2

    source_home = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))).expanduser()
    source_auth = source_home / "auth.json"
    if not source_auth.is_file() and not os.environ.get("OPENAI_API_KEY"):
        print("Codex authentication is unavailable", file=sys.stderr)
        return 2

    repository = Path(__file__).resolve().parents[2]
    marketplace = repository / "tap" / "codex"
    server = ThreadingHTTPServer(("127.0.0.1", 0), Receiver)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    try:
        with tempfile.TemporaryDirectory(prefix="catena-codex-e2e-") as temporary:
            codex_home = Path(temporary)
            if source_auth.is_file():
                shutil.copy2(source_auth, codex_home / "auth.json")
            environment = {
                **os.environ,
                "CODEX_HOME": str(codex_home),
                "CATENA_API_KEY": "catena_agent_real_smoke",
                "CATENA_OTLP_ENDPOINT": (f"http://127.0.0.1:{server.server_port}/v1/otlp/v1/traces"),
            }
            run([codex, "plugin", "marketplace", "add", str(marketplace)], environment)
            run([codex, "plugin", "add", "tracing@catena-runtime"], environment)
            run(
                [
                    codex,
                    "--dangerously-bypass-hook-trust",
                    "--sandbox",
                    "read-only",
                    "--ask-for-approval",
                    "never",
                    "-C",
                    str(repository),
                    "exec",
                    "--skip-git-repo-check",
                    "Use the shell tool to run `printf CATENA_CODEX_E2E` exactly once, "
                    "then answer only CATENA_CODEX_E2E_OK.",
                ],
                environment,
            )
    except (OSError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(str(error), file=sys.stderr)
        return 1
    finally:
        server.shutdown()
        server.server_close()

    spans = [
        span
        for payload in Receiver.payloads
        for resource in payload.get("resourceSpans", [])
        for scope in resource.get("scopeSpans", [])
        for span in scope.get("spans", [])
    ]
    kinds = {
        attribute["value"].get("stringValue")
        for span in spans
        for attribute in span.get("attributes", [])
        if attribute.get("key") == "catena.node.kind"
    }
    if not {"turn", "model", "tool"}.issubset(kinds):
        print(f"Unexpected Span kinds: {sorted(kind for kind in kinds if kind)}", file=sys.stderr)
        return 1
    print(f"Real Codex hook smoke passed: {len(spans)} deterministic spans")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
