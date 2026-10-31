"""Private pipeline transport. Identity is supplied only by authenticated Go."""
from __future__ import annotations

import hmac
import json
import os
import threading
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from .executions import Executions
from .rsi.contracts import OptimizationError

MAX_BODY_BYTES = 2 * 1024 * 1024


def create_server(address, token="", rsi_handler=None, receipt_root=None):
    host, _, port = address.rpartition(":")
    slots = threading.BoundedSemaphore(1)
    executions = Executions(receipt_root or os.environ.get("CATENA_ENGINE_EXECUTIONS_ROOT", ".local/executions"))

    class Handler(BaseHTTPRequestHandler):
        def _json(self, status, body):
            data = json.dumps(body, ensure_ascii=False).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(data)

        def _authorized(self):
            supplied = self.headers.get("Authorization", "")
            if not token or not hmac.compare_digest(supplied.encode(), ("Bearer " + token).encode()):
                self._json(401, {"error": "private service authentication required"})
                return False
            return True

        def _owner(self):
            owner = self.headers.get("X-Catena-Owner", "")
            if not owner or len(owner) > 128:
                raise OptimizationError("authenticated owner context is required")
            return owner

        def do_GET(self):
            if self.path in ("/healthz", "/readyz"):
                self._json(200, {"status": "ready", "service": "catena-engine"})
                return
            if not self._authorized():
                return
            try:
                if self.path == "/v2/manifest":
                    self._json(200, {"schema": "catena.engine_manifest.v2", "status": "ready",
                                     "pipelines": ["rsi", "memory"]})
                elif self.path == "/v2/rsi/adapters":
                    from .rsi.registry import default_registry
                    self._json(200, {"adapters": default_registry().describe()})
                elif self.path == "/v2/cases":
                    path = os.environ.get("CATENA_RSI_CASES_MANIFEST")
                    cases = json.loads(open(path).read()) if path else {}
                    self._json(200, {"cases": [{"id": key} for key in cases]})
                elif self.path.startswith("/v2/executions/"):
                    status, result = executions.lookup(self._owner(), self.path.rsplit("/", 1)[1])
                    self._json(status, result)
                else:
                    self._json(404, {"error": "endpoint retired or unknown"})
            except (OptimizationError, ValueError):
                self._json(400, {"error": "invalid private request"})
            except Exception:
                self._json(503, {"error": "pipeline configuration unavailable"})

        def do_DELETE(self):
            if not self._authorized():
                return
            try:
                if self.path.startswith("/v2/executions/"):
                    status, result = executions.cancel(self._owner(), self.path.rsplit("/", 1)[1])
                    self._json(status, result)
                else:
                    self._json(404, {"error": "unknown execution"})
            except OptimizationError:
                self._json(400, {"error": "invalid execution identity"})

        def do_POST(self):
            if not self._authorized():
                return
            try:
                owner = self._owner()
                length = int(self.headers.get("Content-Length", "0"))
                if not 0 < length <= MAX_BODY_BYTES:
                    raise OptimizationError("invalid body size")
                payload = json.loads(self.rfile.read(length))
                if not isinstance(payload, dict):
                    raise OptimizationError("request must be an object")
                if self.path == "/v2/rsi":
                    from .rsi.api import run_payload
                    cached = executions.replay(owner, payload)
                    if cached is not None:
                        self._json(*cached)
                        return
                    if not slots.acquire(blocking=False):
                        self._json(429, {"error": "analysis capacity is full"})
                        return
                    try:
                        status, result = executions.execute(owner, payload,
                            lambda event: rsi_handler(payload, event) if rsi_handler else run_payload(payload, event))
                        self._json(status, result)
                    finally:
                        slots.release()
                elif self.path.startswith("/v2/memory/"):
                    from .memory.api import memory_request
                    result = memory_request(owner, self.path.rsplit("/", 1)[1], payload, executions)
                    self._json(200, result)
                else:
                    self._json(404, {"error": "endpoint retired or unknown"})
            except (OptimizationError, ValueError, TypeError):
                self._json(400, {"error": "invalid pipeline request"})
            except Exception:
                self._json(503, {"error": "pipeline unavailable"})

        def log_message(self, *_):
            pass

    server = ThreadingHTTPServer((host or "127.0.0.1", int(port)), Handler)
    server.executions = executions
    return server


def main():
    server = create_server(os.environ.get("CATENA_ENGINE_ADDR", "0.0.0.0:8790"),
                           os.environ.get("CATENA_ENGINE_TOKEN", ""))
    try:
        server.serve_forever()
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
