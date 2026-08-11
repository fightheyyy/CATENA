"""Run a real Codex tool turn through Catena Tap and a local OTLP receiver."""

from __future__ import annotations

import json
import os
import subprocess
import sys
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


def main() -> int:
    server = ThreadingHTTPServer(("127.0.0.1", 0), Receiver)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    catena = Path(sys.executable).with_name("catena")
    repository = Path(__file__).resolve().parents[2]
    env = {
        **os.environ,
        "CATENA_API_KEY": "catena_agent_real_smoke",
        "CATENA_OTLP_ENDPOINT": f"http://127.0.0.1:{server.server_port}/v1/otlp/v1/traces",
    }
    command = [
        str(catena),
        "tap",
        "codex",
        "--",
        "--sandbox",
        "read-only",
        "--ask-for-approval",
        "never",
        "-C",
        str(repository),
        "exec",
        "--skip-git-repo-check",
        "Use the shell tool to run pwd exactly once, then answer only CATENA_TAP_OK.",
    ]
    try:
        result = subprocess.run(command, env=env, text=True, timeout=180, check=False)
    finally:
        server.shutdown()
        server.server_close()
    if result.returncode != 0:
        print(f"Codex exited with {result.returncode}", file=sys.stderr)
        return result.returncode
    if not Receiver.payloads:
        print("No OTLP payload was received", file=sys.stderr)
        return 1
    if len(Receiver.payloads) != 1:
        print(f"Expected one Agent Turn, received {len(Receiver.payloads)}", file=sys.stderr)
        return 1
    spans = Receiver.payloads[-1]["resourceSpans"][0]["scopeSpans"][0]["spans"]
    names = [span["name"] for span in spans]
    tool_spans = [span for span in spans if span["name"].startswith("agent.tool.call")]
    if "agent.turn" not in names or "gen_ai.model.call" not in names or not tool_spans:
        print(f"Unexpected Span tree: {names}", file=sys.stderr)
        return 1
    tool_attributes = {
        item["key"]: next(iter(item["value"].values())) for item in tool_spans[0].get("attributes", [])
    }
    if not tool_attributes.get("gen_ai.tool.call.result"):
        print("Tool Span has no result", file=sys.stderr)
        return 1
    print(f"Real Codex smoke passed: {len(spans)} spans · {names}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
