"""Wire contract between the Go control plane and the engine (catena.engine_*.v1)."""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any
from urllib.parse import urlparse

from . import __version__
from .roles import ROLES

REQUEST_SCHEMA = "catena.engine_turn_request.v1"
RESPONSE_SCHEMA = "catena.engine_turn_response.v1"
MANIFEST_SCHEMA = "catena.engine_manifest.v1"
RUNTIME_ID = "catena-engine"

MAX_PROMPT_BYTES = 1_000_000
MAX_TIMEOUT_MS = 15 * 60 * 1000
_SAFE_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")


class InvalidRequest(ValueError):
    """Raised when a turn request violates the wire contract."""


@dataclass(frozen=True)
class ModelConfig:
    provider: str
    base_url: str
    model: str
    api_key: str


@dataclass(frozen=True)
class TurnRequest:
    request_id: str
    run_id: str
    role: str
    prompt: str
    timeout_ms: int
    model: ModelConfig


def parse_turn_request(payload: Any) -> TurnRequest:
    if not isinstance(payload, dict) or payload.get("schema") != REQUEST_SCHEMA:
        raise InvalidRequest(f"schema must be {REQUEST_SCHEMA}")
    request_id = _safe_id(payload, "request_id")
    run_id = _safe_id(payload, "run_id")
    role = payload.get("role")
    if role not in ROLES:
        raise InvalidRequest("role is not supported by this engine")
    prompt = payload.get("prompt")
    if not isinstance(prompt, str) or not prompt.strip() or len(prompt.encode()) > MAX_PROMPT_BYTES:
        raise InvalidRequest(f"prompt must contain from 1 to {MAX_PROMPT_BYTES} bytes")
    timeout_ms = payload.get("timeout_ms")
    if not isinstance(timeout_ms, int) or isinstance(timeout_ms, bool) or not 1 <= timeout_ms <= MAX_TIMEOUT_MS:
        raise InvalidRequest("timeout_ms must be from 1 to 900000")
    return TurnRequest(
        request_id=request_id,
        run_id=run_id,
        role=role,
        prompt=prompt,
        timeout_ms=timeout_ms,
        model=_model_config(payload.get("model")),
    )


def _safe_id(payload: dict[str, Any], key: str) -> str:
    value = payload.get(key)
    if not isinstance(value, str) or not _SAFE_ID.match(value):
        raise InvalidRequest(f"{key} must be a safe identifier")
    return value


def _model_config(value: Any) -> ModelConfig:
    if not isinstance(value, dict):
        raise InvalidRequest("model configuration is required")
    fields = {key: value.get(key) for key in ("provider", "base_url", "model", "api_key")}
    if not all(isinstance(item, str) and item.strip() for item in fields.values()):
        raise InvalidRequest("model configuration is incomplete")
    parsed = urlparse(fields["base_url"])
    if parsed.scheme not in ("http", "https") or not parsed.netloc or parsed.username or parsed.query:
        raise InvalidRequest("model base_url must be an absolute HTTP(S) URL")
    return ModelConfig(**{key: item.strip() for key, item in fields.items()})


def manifest(status: str = "ready", reason_code: str = "", detail: str = "") -> dict[str, Any]:
    result: dict[str, Any] = {
        "schema": MANIFEST_SCHEMA,
        "runtime_id": RUNTIME_ID,
        "display_name": "Catena Engine",
        "kind": "embedded_evolution",
        "source": "configured",
        "status": status,
        "version": f"catena-engine {__version__}",
        "detail": detail or "Catena Engine is ready.",
        "roles": [
            {
                "id": role.id,
                "display_name": role.display_name,
                "responsibility": role.responsibility,
                "output": role.output,
            }
            for role in ROLES.values()
        ],
        "capabilities": {
            "probe": True,
            "role_turn": True,
            # A turn ends at its own timeout; client disconnects do not abort it.
            "cancellation": False,
            "telemetry": "none",
            "target_runtime_hosted": False,
        },
    }
    if reason_code:
        result["reason_code"] = reason_code
    return result


def turn_response(request_id: str, result: dict[str, Any]) -> dict[str, Any]:
    return {"schema": RESPONSE_SCHEMA, "request_id": request_id, "status": "ok", "result": result}


def error_response(request_id: str, code: str, detail: str) -> dict[str, Any]:
    return {
        "schema": RESPONSE_SCHEMA,
        "request_id": request_id,
        "status": "error",
        "error": {"code": code, "detail": detail[:500]},
    }
