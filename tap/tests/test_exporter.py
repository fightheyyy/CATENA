from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from catena_tap.exporter import BackgroundTraceExporter, OTLPHTTPClient, endpoint_from_environment


class _Handler(BaseHTTPRequestHandler):
    requests = []

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        self.__class__.requests.append(
            {
                "path": self.path,
                "authorization": self.headers.get("Authorization"),
                "body": json.loads(self.rfile.read(length)),
            }
        )
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b"{}")

    def log_message(self, _format, *_args):
        return


def test_background_exporter_posts_authenticated_turn():
    _Handler.requests = []
    server = ThreadingHTTPServer(("127.0.0.1", 0), _Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    endpoint = f"http://127.0.0.1:{server.server_port}/v1/otlp/v1/traces"
    exporter = BackgroundTraceExporter("codex", OTLPHTTPClient(endpoint, "catena_agent_test"))
    exporter.submit(
        "session",
        {
            "timestamp": "2026-08-11T10:00:01Z",
            "request_id": "request",
            "duration_ms": 10,
            "request": {"body": {"model": "gpt", "input": [{"role": "user", "content": "hello"}]}},
            "response": {
                "status": 200,
                "body": {
                    "model": "gpt",
                    "output": [{"type": "message", "content": [{"type": "output_text", "text": "hi"}]}],
                },
            },
        },
    )

    stats = exporter.close()
    server.shutdown()
    server.server_close()

    assert stats.uploaded_traces == 1
    assert stats.failed_uploads == 0
    assert _Handler.requests[0]["path"] == "/v1/otlp/v1/traces"
    assert _Handler.requests[0]["authorization"] == "Bearer catena_agent_test"
    assert _Handler.requests[0]["body"]["resourceSpans"]


def test_endpoint_joins_catena_origin(monkeypatch):
    monkeypatch.delenv("CATENA_OTLP_ENDPOINT", raising=False)
    monkeypatch.delenv("CATENA_URL", raising=False)
    assert endpoint_from_environment("", "https://catena.example/") == (
        "https://catena.example/v1/otlp/v1/traces"
    )
