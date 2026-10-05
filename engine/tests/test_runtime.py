from __future__ import annotations

import asyncio

import pytest
from conftest import FakeModel, factory_for

from catena_engine.protocol import InvalidRequest, parse_turn_request
from catena_engine.roles import ROLES
from catena_engine.runtime import run_turn


def test_completed_turn_returns_text_usage_and_closes_client(turn_payload):
    model = FakeModel('{"finding":{"title":"t"}}')
    factory = factory_for(model)
    result = asyncio.run(run_turn(parse_turn_request(turn_payload), factory))

    assert result["status"] == "completed"
    assert result["assistant"]["content"] == '{"finding":{"title":"t"}}'
    assert result["usage"] == {"requests": 1, "input_tokens": 11, "output_tokens": 7}
    assert factory.closed == [True]
    assert model.calls[0]["instructions"] == ROLES["inspector"].instructions
    assert model.calls[0]["tools"] == []


def test_provider_error_is_reported_without_the_api_key(turn_payload):
    model = FakeModel(error=RuntimeError("401 bad key sk-test-secret-value"))
    factory = factory_for(model)
    result = asyncio.run(run_turn(parse_turn_request(turn_payload), factory))

    assert result["status"] == "failed"
    assert result["reason_code"] == "model_error"
    assert "sk-test-secret-value" not in result["detail"]
    assert factory.closed == [True]


def test_timeout_is_reported(turn_payload):
    turn_payload["timeout_ms"] = 20
    result = asyncio.run(run_turn(parse_turn_request(turn_payload), factory_for(FakeModel(delay=1))))

    assert result["status"] == "failed"
    assert result["reason_code"] == "timeout"


def test_empty_output_is_a_failure(turn_payload):
    result = asyncio.run(run_turn(parse_turn_request(turn_payload), factory_for(FakeModel("  "))))

    assert result["reason_code"] == "empty_output"


@pytest.mark.parametrize(
    ("key", "value"),
    [
        ("schema", "barena.xiaoba_evolution_request.v1"),
        ("role", "user-cat"),
        ("request_id", "../escape"),
        ("prompt", "   "),
        ("timeout_ms", 0),
        ("timeout_ms", True),
        ("model", {"provider": "x", "base_url": "file:///etc/passwd", "model": "m", "api_key": "k"}),
        ("model", {"provider": "x", "base_url": "https://h/v1", "model": "m"}),
    ],
)
def test_invalid_requests_are_rejected(turn_payload, key, value):
    turn_payload[key] = value
    with pytest.raises(InvalidRequest):
        parse_turn_request(turn_payload)
