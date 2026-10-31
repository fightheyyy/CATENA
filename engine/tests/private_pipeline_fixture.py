"""Opt-in cross-process protocol fixture, never a real model/provider benchmark."""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from catena_engine.server import create_server


class Provider(BaseHTTPRequestHandler):
    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        result = {"task_id": "fixture-task"} if self.path.endswith("/commit") else {}
        self.reply(result)

    def do_GET(self):
        self.reply({"status": "completed", "result": {"memory_count": 1, "fixture": True}})

    def reply(self, result):
        data = json.dumps({"status": "ok", "result": result}).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, *_):
        pass


def rsi(payload, event):
    return {
        "status": "completed",
        "candidates": [{"id": "fixture-candidate", "files": {"AGENTS.md": "Inspect evidence before acting."}}],
        "trials": [{"phase": "validation", "fixture": True, "valid": False, "error": "No real verifier executed"}],
        "validation_case_ids": [],
    }


if __name__ == "__main__":
    provider = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    threading.Thread(target=provider.serve_forever, daemon=True).start()
    os.environ["CATENA_OPENVIKING_URL"] = f"http://127.0.0.1:{provider.server_port}"
    os.environ["CATENA_OPENVIKING_API_KEY"] = "fixture-provider-only"
    server = create_server("127.0.0.1:0", "fixture-private", rsi, os.environ["CATENA_ENGINE_EXECUTIONS_ROOT"])
    print(f"http://127.0.0.1:{server.server_port}", flush=True)
    server.serve_forever()
