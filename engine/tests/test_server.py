from __future__ import annotations

import json
import threading
import urllib.error
import urllib.request

import pytest
from conftest import FakeModel, factory_for

from catena_engine.protocol import MANIFEST_SCHEMA, RESPONSE_SCHEMA
from catena_engine.server import create_server


@pytest.fixture
def engine():
    def start(token: str = "", model: FakeModel | None = None):
        server = create_server("127.0.0.1:0", token, factory_for(model or FakeModel()))
        threading.Thread(target=server.serve_forever, daemon=True).start()
        started.append(server)
        return f"http://127.0.0.1:{server.server_address[1]}"

    started = []
    yield start
    for server in started:
        server.shutdown()
        server.server_close()


def call(url: str, body: dict | None = None, token: str = ""):
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(url, data=data, method="POST" if data else "GET")
    request.add_header("Content-Type", "application/json")
    if token:
        request.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            return response.status, json.loads(response.read())
    except urllib.error.HTTPError as error:
        return error.code, json.loads(error.read())


def test_manifest_lists_the_three_roles(engine):
    status, body = call(engine() + "/v1/manifest")

    assert status == 200
    assert body["schema"] == MANIFEST_SCHEMA
    assert body["status"] == "ready"
    assert [role["id"] for role in body["roles"]] == ["inspector", "evolution", "reviewer"]
    assert body["capabilities"]["target_runtime_hosted"] is False


def test_turn_round_trip(engine, turn_payload):
    status, body = call(engine(model=FakeModel("hello")) + "/v1/turn", turn_payload)

    assert status == 200
    assert body["schema"] == RESPONSE_SCHEMA
    assert body["request_id"] == "inspector-1"
    assert body["result"]["assistant"]["content"] == "hello"


def test_invalid_turn_is_a_400_with_an_error_envelope(engine, turn_payload):
    turn_payload["role"] = "planner"
    status, body = call(engine() + "/v1/turn", turn_payload)

    assert status == 400
    assert body["status"] == "error"
    assert body["error"]["code"] == "invalid_request"


def test_token_is_required_when_configured(engine, turn_payload):
    base = engine(token="s3cret")

    assert call(base + "/v1/manifest")[0] == 401
    assert call(base + "/v1/turn", turn_payload, token="wrong")[0] == 401
    assert call(base + "/v1/turn", turn_payload, token="s3cret")[0] == 200
    assert call(base + "/healthz")[0] == 200
