from __future__ import annotations
from dataclasses import dataclass
from typing import Any
from urllib.parse import urlparse

class InvalidRequest(ValueError):
    """Raised when a turn request violates the wire contract."""


@dataclass(frozen=True)
class ModelConfig:
    provider: str
    base_url: str
    model: str
    api_key: str


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


@dataclass(frozen=True)
class JsonRequest:
    request_id: str
    run_id: str
    prompt: str
    timeout_ms: int
    model: ModelConfig
