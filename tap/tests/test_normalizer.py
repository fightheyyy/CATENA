from __future__ import annotations

import base64

from catena_tap.normalizer import TraceNormalizer


def _record(request_body, response_body, *, request_id: str, timestamp: str, status: int = 200):
    return {
        "timestamp": timestamp,
        "request_id": request_id,
        "duration_ms": 100,
        "request": {"body": request_body},
        "response": {"status": status, "body": response_body},
    }


def test_discovery_requests_do_not_create_agent_turns():
    normalizer = TraceNormalizer("codex")
    record = _record(
        {},
        {"models": [{"slug": "gpt-5.6-sol"}]},
        request_id="models-1",
        timestamp="2026-08-11T10:00:00Z",
    )
    record["request"].update({"method": "GET", "path": "/v1/models?client_version=0.147.0"})

    assert normalizer.ingest("session-models", record) == []
    assert normalizer.flush() == []


def _spans(payload):
    return payload["resourceSpans"][0]["scopeSpans"][0]["spans"]


def _attrs(span):
    values = {}
    for item in span["attributes"]:
        value = item["value"]
        values[item["key"]] = next(iter(value.values()))
    return values


def test_openai_responses_turn_pairs_function_output():
    normalizer = TraceNormalizer("codex")
    first = _record(
        {"model": "gpt-5.6-sol", "input": [{"role": "user", "content": "read package.json"}]},
        {
            "model": "gpt-5.6-sol",
            "output": [
                {
                    "type": "function_call",
                    "call_id": "call-read",
                    "name": "read_file",
                    "arguments": '{"path":"package.json"}',
                }
            ],
            "usage": {"input_tokens": 20, "output_tokens": 5},
        },
        request_id="req-1",
        timestamp="2026-08-11T10:00:01Z",
    )
    second = _record(
        {
            "model": "gpt-5.6-sol",
            "input": [
                {"role": "user", "content": "read package.json"},
                {"type": "function_call_output", "call_id": "call-read", "output": '{"name":"catena"}'},
            ],
        },
        {
            "model": "gpt-5.6-sol",
            "output": [{"type": "message", "content": [{"type": "output_text", "text": "name is catena"}]}],
        },
        request_id="req-2",
        timestamp="2026-08-11T10:00:02Z",
    )

    assert normalizer.ingest("session-1", first) == []
    payloads = normalizer.ingest("session-1", second)

    assert len(payloads) == 1
    spans = _spans(payloads[0])
    assert [span["name"] for span in spans] == [
        "agent.turn",
        "gen_ai.model.call",
        "agent.tool.call read_file",
        "gen_ai.model.call",
    ]
    tool = next(span for span in spans if span["name"].startswith("agent.tool.call"))
    assert _attrs(tool)["gen_ai.tool.call.arguments"] == '{"path":"package.json"}'
    assert _attrs(tool)["gen_ai.tool.call.result"] == '{"name":"catena"}'
    root_attrs = _attrs(spans[0])
    assert root_attrs["input.value"] == "read package.json"
    assert root_attrs["output.value"] == "name is catena"
    assert root_attrs["catena.turn.incomplete"] is False
    assert len(base64.b64decode(spans[0]["traceId"])) == 16
    assert len(base64.b64decode(spans[0]["spanId"])) == 8


def test_anthropic_messages_turn_pairs_tool_result():
    normalizer = TraceNormalizer("claude")
    first = _record(
        {"model": "claude-opus", "messages": [{"role": "user", "content": "check status"}]},
        {
            "model": "claude-opus",
            "content": [{"type": "tool_use", "id": "tool-1", "name": "Bash", "input": {"command": "pwd"}}],
        },
        request_id="anthropic-1",
        timestamp="2026-08-11T10:00:01Z",
    )
    second = _record(
        {
            "model": "claude-opus",
            "messages": [
                {"role": "user", "content": "check status"},
                {
                    "role": "user",
                    "content": [{"type": "tool_result", "tool_use_id": "tool-1", "content": "/workspace"}],
                },
            ],
        },
        {"model": "claude-opus", "content": [{"type": "text", "text": "workspace is ready"}]},
        request_id="anthropic-2",
        timestamp="2026-08-11T10:00:02Z",
    )

    assert normalizer.ingest("session-2", first) == []
    payload = normalizer.ingest("session-2", second)[0]
    tool = next(span for span in _spans(payload) if span["name"] == "agent.tool.call Bash")
    assert _attrs(tool)["gen_ai.tool.call.result"] == "/workspace"
    assert tool["status"]["code"] == 1


def test_chat_completions_tool_shape_is_supported():
    normalizer = TraceNormalizer("openclaw")
    first = _record(
        {"model": "gpt", "messages": [{"role": "user", "content": "weather"}]},
        {
            "choices": [
                {
                    "message": {
                        "tool_calls": [
                            {
                                "id": "weather-1",
                                "function": {"name": "weather", "arguments": '{"city":"Shanghai"}'},
                            }
                        ]
                    }
                }
            ]
        },
        request_id="chat-1",
        timestamp="2026-08-11T10:00:01Z",
    )
    second = _record(
        {
            "model": "gpt",
            "messages": [
                {"role": "user", "content": "weather"},
                {"role": "tool", "tool_call_id": "weather-1", "content": "sunny"},
            ],
        },
        {"choices": [{"message": {"content": "It is sunny."}}]},
        request_id="chat-2",
        timestamp="2026-08-11T10:00:02Z",
    )

    normalizer.ingest("session-3", first)
    payload = normalizer.ingest("session-3", second)[0]
    spans = _spans(payload)
    assert len(spans) == 4
    assert _attrs(spans[0])["output.value"] == "It is sunny."


def test_flush_marks_open_turn_incomplete():
    normalizer = TraceNormalizer("hermes")
    normalizer.ingest(
        "session-4",
        _record(
            {"messages": [{"role": "user", "content": "run a tool"}]},
            {"content": [{"type": "tool_use", "id": "tool-4", "name": "shell", "input": {}}]},
            request_id="flush-1",
            timestamp="2026-08-11T10:00:01Z",
        ),
    )

    spans = _spans(normalizer.flush()[0])
    assert _attrs(spans[0])["catena.turn.incomplete"] is True
    tool = next(span for span in spans if span["name"] == "agent.tool.call shell")
    assert tool["status"]["code"] == 2
