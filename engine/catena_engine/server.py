"""Private HTTP surface for the control plane. Never expose it publicly."""

from __future__ import annotations

import asyncio
import hmac
import json
import logging
import os
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

from .protocol import InvalidRequest, error_response, manifest, parse_turn_request, turn_response
from .runtime import ModelFactory, openai_compatible_model, run_turn

MAX_BODY_BYTES = 2 * 1024 * 1024
DEFAULT_ADDR = "127.0.0.1:8790"

log = logging.getLogger("catena_engine")


def create_server(
    address: str, token: str = "", model_factory: ModelFactory = openai_compatible_model
) -> ThreadingHTTPServer:
    host, _, port = address.rpartition(":")

    class Handler(BaseHTTPRequestHandler):
        server_version = "catena-engine"
        sys_version = ""

        def do_GET(self) -> None:
            if self.path in ("/healthz", "/readyz"):
                self._json(HTTPStatus.OK, {"status": "ready", "service": "catena-engine"})
            elif self.path == "/v1/manifest":
                if self._authorized():
                    self._json(HTTPStatus.OK, manifest())
            else:
                self._json(HTTPStatus.NOT_FOUND, {"error": "not_found"})

        def do_POST(self) -> None:
            if self.path not in ("/v1/turn", "/v1/experiment", "/v1/harbor/submit", "/v1/harbor/status", "/v1/harbor/lookup", "/v1/harbor/evidence"):
                self._json(HTTPStatus.NOT_FOUND, {"error": "not_found"})
                return
            if not self._authorized():
                return
            payload = self._read_json()
            if payload is None:
                return
            if self.path.startswith("/v1/harbor/"):
                from .harbor_bridge import harbor_request

                status, result = asyncio.run(harbor_request(self.path, payload))
                self._json(status, result)
                return
            request_id = payload.get("request_id") if isinstance(payload.get("request_id"), str) else ""
            try:
                request = parse_turn_request(payload)
            except InvalidRequest as error:
                self._json(HTTPStatus.BAD_REQUEST, error_response(request_id[:128], "invalid_request", str(error)))
                return
            if self.path == "/v1/experiment":
                from .experiment_runtime import run_experiment_turn

                result = asyncio.run(run_experiment_turn(request, model_factory, payload.get("environment"), payload.get("target_agent", "sdk")))
            else:
                result = asyncio.run(run_turn(request, model_factory))
            log.info("turn role=%s run=%s status=%s", request.role, request.run_id, result["status"])
            self._json(HTTPStatus.OK, turn_response(request.request_id, result))

        def _authorized(self) -> bool:
            if not token:
                return True
            supplied = self.headers.get("Authorization", "")
            if hmac.compare_digest(supplied.encode(), f"Bearer {token}".encode()):
                return True
            self._json(HTTPStatus.UNAUTHORIZED, {"error": "unauthorized"})
            return False

        def _read_json(self) -> dict[str, Any] | None:
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError:
                length = -1
            if not 0 < length <= MAX_BODY_BYTES:
                self._json(HTTPStatus.REQUEST_ENTITY_TOO_LARGE, {"error": "body must be from 1 byte to 2 MiB"})
                return None
            try:
                payload = json.loads(self.rfile.read(length))
            except (json.JSONDecodeError, UnicodeDecodeError):
                payload = None
            if not isinstance(payload, dict):
                self._json(HTTPStatus.BAD_REQUEST, {"error": "body must be a JSON object"})
                return None
            return payload

        def _json(self, status: HTTPStatus, body: dict[str, Any]) -> None:
            encoded = json.dumps(body, ensure_ascii=False).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(encoded)

        def log_message(self, format: str, *args: Any) -> None:
            log.debug(format, *args)

    server = ThreadingHTTPServer((host or "127.0.0.1", int(port)), Handler)
    server.daemon_threads = True
    return server


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    address = os.environ.get("CATENA_ENGINE_ADDR", DEFAULT_ADDR)
    server = create_server(address, os.environ.get("CATENA_ENGINE_TOKEN", ""))
    log.info("catena-engine listening on %s", address)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
