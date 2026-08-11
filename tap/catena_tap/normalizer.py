"""Turn reconstruction and OTLP/HTTP JSON rendering for claude-tap records."""

from __future__ import annotations

import base64
import hashlib
import json
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

MAX_ATTRIBUTE_CHARS = 64_000


@dataclass
class ToolCall:
    call_id: str
    name: str
    arguments: str
    span_id: str
    parent_span_id: str
    start_ns: int
    end_ns: int
    result: str = ""
    error: bool = False


@dataclass
class ModelCall:
    span_id: str
    model: str
    request: str
    response: str
    start_ns: int
    end_ns: int
    status_code: int
    input_tokens: int = 0
    output_tokens: int = 0
    tools: list[ToolCall] = field(default_factory=list)


@dataclass
class TurnBuffer:
    trace_id: str
    root_span_id: str
    session_id: str
    runtime: str
    turn_id: str
    prompt: str
    start_ns: int
    end_ns: int
    models: list[ModelCall] = field(default_factory=list)
    pending_tools: dict[str, ToolCall] = field(default_factory=dict)
    error: bool = False


class TraceNormalizer:
    """Fold claude-tap API records into canonical Agent Turn traces."""

    def __init__(self, runtime: str):
        self.runtime = runtime
        self._turns: dict[str, TurnBuffer] = {}

    def ingest(self, session_id: str, record: dict[str, Any]) -> list[dict[str, Any]]:
        """Consume one captured API request/response and return completed OTLP payloads."""
        request = _mapping(record.get("request"))
        response = _mapping(record.get("response"))
        request_body = _body_mapping(request.get("body"))
        response_body = _body_mapping(response.get("body"))
        if not _is_model_exchange(request, request_body, response_body):
            return []
        start_ns, end_ns = _record_times(record)
        prompt = _extract_latest_user_prompt(request_body)

        completed: list[dict[str, Any]] = []
        turn = self._turns.get(session_id)
        if turn is not None and prompt and turn.prompt and prompt != turn.prompt:
            completed.append(self._finish(turn, incomplete=True))
            self._turns.pop(session_id, None)
            turn = None

        if turn is None:
            seed = str(record.get("request_id") or record.get("turn") or end_ns)
            trace_id = _stable_hex(f"trace:{self.runtime}:{session_id}:{seed}", 32)
            turn = TurnBuffer(
                trace_id=trace_id,
                root_span_id=_stable_hex(f"root:{trace_id}", 16),
                session_id=session_id,
                runtime=self.runtime,
                turn_id=_stable_hex(f"turn:{trace_id}", 24),
                prompt=prompt,
                start_ns=start_ns,
                end_ns=end_ns,
            )
            self._turns[session_id] = turn
        elif not turn.prompt and prompt:
            turn.prompt = prompt

        self._apply_tool_results(turn, _extract_tool_results(request_body), start_ns)

        model_index = len(turn.models)
        model_span_id = _stable_hex(f"model:{turn.trace_id}:{model_index}", 16)
        status_code = _integer(response.get("status"))
        model = ModelCall(
            span_id=model_span_id,
            model=_extract_model(request_body, response_body),
            request=_request_summary(request_body),
            response=_extract_assistant_text(response_body),
            start_ns=start_ns,
            end_ns=end_ns,
            status_code=status_code,
        )
        model.input_tokens, model.output_tokens = _extract_usage(response_body)

        for index, raw_tool in enumerate(_extract_tool_calls(response_body)):
            raw_id = raw_tool[0]
            pending_key = raw_id or f"pending:{model_index}:{index}"
            tool = ToolCall(
                call_id=raw_id or pending_key,
                name=raw_tool[1],
                arguments=raw_tool[2],
                span_id=_stable_hex(f"tool:{turn.trace_id}:{model_index}:{index}:{pending_key}", 16),
                parent_span_id=model_span_id,
                start_ns=end_ns,
                end_ns=end_ns,
            )
            model.tools.append(tool)
            turn.pending_tools[pending_key] = tool

        turn.models.append(model)
        turn.start_ns = min(turn.start_ns, start_ns)
        turn.end_ns = max(turn.end_ns, end_ns)
        if status_code >= 400:
            turn.error = True

        if not model.tools:
            completed.append(self._finish(turn, incomplete=False))
            self._turns.pop(session_id, None)
        return completed

    def flush(self) -> list[dict[str, Any]]:
        """Return every still-open turn as incomplete."""
        payloads = [self._finish(turn, incomplete=True) for turn in self._turns.values()]
        self._turns.clear()
        return payloads

    def _apply_tool_results(self, turn: TurnBuffer, results: list[tuple[str, str, bool]], end_ns: int) -> None:
        for call_id, result, error in results:
            tool = turn.pending_tools.pop(call_id, None) if call_id else None
            if tool is None and turn.pending_tools:
                fallback_key = next(iter(turn.pending_tools))
                tool = turn.pending_tools.pop(fallback_key)
            if tool is None:
                continue
            tool.result = result
            tool.error = error
            tool.end_ns = max(tool.start_ns, end_ns)
            turn.error = turn.error or error

    def _finish(self, turn: TurnBuffer, *, incomplete: bool) -> dict[str, Any]:
        for tool in turn.pending_tools.values():
            tool.end_ns = max(tool.start_ns, turn.end_ns)
            tool.error = True
        if turn.pending_tools:
            incomplete = True
        output = ""
        for model in reversed(turn.models):
            if model.response:
                output = model.response
                break
        return _otlp_payload(turn, output=output, incomplete=incomplete)


def _is_model_exchange(
    request: dict[str, Any], request_body: dict[str, Any], response_body: dict[str, Any]
) -> bool:
    """Exclude discovery and health traffic captured by the upstream proxy."""
    method = str(request.get("method") or "").upper()
    if method and method != "POST":
        return False

    path = str(request.get("path") or "").split("?", 1)[0].rstrip("/").lower()
    if path.endswith("/models") or path.endswith("/health"):
        return False

    request_markers = {"model", "messages", "input", "prompt"}
    response_markers = {"model", "content", "output", "choices", "usage"}
    return bool(request_markers.intersection(request_body) or response_markers.intersection(response_body))


def _otlp_payload(turn: TurnBuffer, *, output: str, incomplete: bool) -> dict[str, Any]:
    spans: list[dict[str, Any]] = []
    root_attributes = {
        "agent.runtime": turn.runtime,
        "agent.session.id": turn.session_id,
        "agent.turn.id": turn.turn_id,
        "catena.source": "catena-tap",
        "catena.turn.incomplete": incomplete,
        "input.value": turn.prompt,
        "output.value": output,
    }
    spans.append(
        _span(
            trace_id=turn.trace_id,
            span_id=turn.root_span_id,
            parent_span_id="",
            name="agent.turn",
            kind=1,
            start_ns=turn.start_ns,
            end_ns=max(turn.start_ns, turn.end_ns),
            attributes=root_attributes,
            error=turn.error or incomplete,
            error_message="incomplete Agent turn" if incomplete else "",
        )
    )
    for model in turn.models:
        model_attributes: dict[str, Any] = {
            "agent.runtime": turn.runtime,
            "agent.turn.id": turn.turn_id,
            "gen_ai.request.model": model.model,
            "gen_ai.response.model": model.model,
            "input.value": model.request,
            "output.value": model.response,
            "http.response.status_code": model.status_code,
        }
        if model.input_tokens:
            model_attributes["gen_ai.usage.input_tokens"] = model.input_tokens
        if model.output_tokens:
            model_attributes["gen_ai.usage.output_tokens"] = model.output_tokens
        spans.append(
            _span(
                trace_id=turn.trace_id,
                span_id=model.span_id,
                parent_span_id=turn.root_span_id,
                name="gen_ai.model.call",
                kind=3,
                start_ns=model.start_ns,
                end_ns=model.end_ns,
                attributes=model_attributes,
                error=model.status_code >= 400,
                error_message=f"upstream HTTP {model.status_code}" if model.status_code >= 400 else "",
            )
        )
        for tool in model.tools:
            spans.append(
                _span(
                    trace_id=turn.trace_id,
                    span_id=tool.span_id,
                    parent_span_id=tool.parent_span_id,
                    name=f"agent.tool.call {tool.name}",
                    kind=1,
                    start_ns=tool.start_ns,
                    end_ns=max(tool.start_ns, tool.end_ns),
                    attributes={
                        "agent.runtime": turn.runtime,
                        "agent.turn.id": turn.turn_id,
                        "gen_ai.tool.name": tool.name,
                        "gen_ai.tool.call.id": tool.call_id,
                        "gen_ai.tool.call.arguments": tool.arguments,
                        "gen_ai.tool.call.result": tool.result,
                        "tool.call.arguments": tool.arguments,
                        "tool.call.result": tool.result,
                    },
                    error=tool.error,
                    error_message="tool result missing or failed" if tool.error else "",
                )
            )

    service_name = f"catena-tap-{turn.runtime}"
    return {
        "resourceSpans": [
            {
                "resource": {
                    "attributes": _attributes(
                        {
                            "service.name": service_name,
                            "agent.runtime": turn.runtime,
                            "telemetry.sdk.name": "catena-tap",
                            "telemetry.sdk.language": "python",
                            "telemetry.sdk.version": "0.1.0",
                        }
                    )
                },
                "scopeSpans": [
                    {
                        "scope": {"name": "catena.tap", "version": "0.1.0"},
                        "spans": spans,
                    }
                ],
            }
        ]
    }


def _span(
    *,
    trace_id: str,
    span_id: str,
    parent_span_id: str,
    name: str,
    kind: int,
    start_ns: int,
    end_ns: int,
    attributes: dict[str, Any],
    error: bool,
    error_message: str,
) -> dict[str, Any]:
    value: dict[str, Any] = {
        "traceId": _proto_bytes(trace_id),
        "spanId": _proto_bytes(span_id),
        "name": name,
        "kind": kind,
        "startTimeUnixNano": str(start_ns),
        "endTimeUnixNano": str(max(start_ns, end_ns)),
        "attributes": _attributes(attributes),
        "status": {"code": 2 if error else 1},
    }
    if parent_span_id:
        value["parentSpanId"] = _proto_bytes(parent_span_id)
    if error_message:
        value["status"]["message"] = error_message
    return value


def _attributes(values: dict[str, Any]) -> list[dict[str, Any]]:
    return [{"key": key, "value": _any_value(value)} for key, value in values.items() if value not in (None, "")]


def _any_value(value: Any) -> dict[str, Any]:
    if isinstance(value, bool):
        return {"boolValue": value}
    if isinstance(value, int):
        return {"intValue": str(value)}
    if isinstance(value, float):
        return {"doubleValue": value}
    return {"stringValue": _stringify(value)}


def _extract_tool_calls(body: dict[str, Any]) -> list[tuple[str, str, str]]:
    calls: list[tuple[str, str, str]] = []
    for block in _response_blocks(body):
        block_type = str(block.get("type") or "")
        if block_type == "tool_use":
            calls.append(
                (
                    str(block.get("id") or block.get("tool_use_id") or ""),
                    str(block.get("name") or "tool"),
                    _stringify(block.get("input", {})),
                )
            )
        elif block_type in {"function_call", "custom_tool_call", "local_shell_call"}:
            arguments = block.get("arguments", block.get("input", block.get("action", {})))
            calls.append(
                (
                    str(block.get("call_id") or block.get("id") or ""),
                    str(block.get("name") or block_type),
                    _stringify(arguments),
                )
            )
    for choice in _list(body.get("choices")):
        message = _mapping(_mapping(choice).get("message"))
        for item in _list(message.get("tool_calls")):
            item = _mapping(item)
            function = _mapping(item.get("function"))
            calls.append(
                (
                    str(item.get("id") or ""),
                    str(function.get("name") or item.get("name") or "tool"),
                    _stringify(function.get("arguments", item.get("arguments", {}))),
                )
            )
    return _dedupe_calls(calls)


def _extract_tool_results(body: dict[str, Any]) -> list[tuple[str, str, bool]]:
    results: list[tuple[str, str, bool]] = []
    for key in ("messages", "input"):
        for item in _list(body.get(key)):
            item = _mapping(item)
            if item.get("role") == "tool":
                results.append(
                    (
                        str(item.get("tool_call_id") or item.get("call_id") or ""),
                        _stringify(item.get("content", "")),
                        bool(item.get("is_error")),
                    )
                )
            item_type = str(item.get("type") or "")
            if item_type in {"function_call_output", "custom_tool_call_output", "tool_result"}:
                results.append(
                    (
                        str(item.get("call_id") or item.get("tool_use_id") or item.get("id") or ""),
                        _stringify(item.get("output", item.get("content", ""))),
                        bool(item.get("is_error")),
                    )
                )
            for block in _content_blocks(item.get("content")):
                if block.get("type") not in {"tool_result", "function_call_output", "custom_tool_call_output"}:
                    continue
                results.append(
                    (
                        str(block.get("tool_use_id") or block.get("call_id") or block.get("id") or ""),
                        _stringify(block.get("content", block.get("output", ""))),
                        bool(block.get("is_error")),
                    )
                )
    return results


def _extract_latest_user_prompt(body: dict[str, Any]) -> str:
    prompts: list[str] = []
    for key in ("messages", "input"):
        for item in _list(body.get(key)):
            item = _mapping(item)
            if item.get("role") != "user":
                continue
            blocks = _content_blocks(item.get("content"))
            if blocks and all(
                block.get("type") in {"tool_result", "function_call_output", "custom_tool_call_output"}
                for block in blocks
            ):
                continue
            text = _text_content(item.get("content"))
            if text:
                prompts.append(text)
    direct = body.get("prompt")
    if isinstance(direct, str) and direct.strip():
        prompts.append(direct.strip())
    return prompts[-1] if prompts else ""


def _extract_assistant_text(body: dict[str, Any]) -> str:
    texts: list[str] = []
    for block in _response_blocks(body):
        if block.get("type") in {"text", "output_text"}:
            text = block.get("text")
            if isinstance(text, dict):
                text = text.get("value")
            if isinstance(text, str) and text:
                texts.append(text)
        elif block.get("type") == "message":
            value = _text_content(block.get("content"))
            if value:
                texts.append(value)
    for choice in _list(body.get("choices")):
        message = _mapping(_mapping(choice).get("message"))
        value = _text_content(message.get("content"))
        if value:
            texts.append(value)
    for key in ("output_text", "completion"):
        value = body.get(key)
        if isinstance(value, str) and value:
            texts.append(value)
    return _truncate("\n".join(dict.fromkeys(texts)))


def _response_blocks(body: dict[str, Any]) -> list[dict[str, Any]]:
    blocks: list[dict[str, Any]] = []
    blocks.extend(_content_blocks(body.get("content")))
    for item in _list(body.get("output")):
        item = _mapping(item)
        blocks.append(item)
        blocks.extend(_content_blocks(item.get("content")))
    nested = body.get("response")
    if isinstance(nested, dict):
        blocks.extend(_response_blocks(nested))
    return blocks


def _content_blocks(content: Any) -> list[dict[str, Any]]:
    if isinstance(content, dict):
        return [content]
    return [_mapping(item) for item in _list(content) if isinstance(item, dict)]


def _text_content(content: Any) -> str:
    if isinstance(content, str):
        return _truncate(content)
    texts: list[str] = []
    for block in _content_blocks(content):
        if block.get("type") in {"tool_result", "function_call_output", "custom_tool_call_output"}:
            continue
        value = block.get("text", block.get("value"))
        if isinstance(value, dict):
            value = value.get("value")
        if isinstance(value, str) and value:
            texts.append(value)
    return _truncate("\n".join(texts))


def _extract_model(request: dict[str, Any], response: dict[str, Any]) -> str:
    for value in (response.get("model"), request.get("model"), _mapping(response.get("response")).get("model")):
        if isinstance(value, str) and value:
            return value
    return "unknown"


def _extract_usage(body: dict[str, Any]) -> tuple[int, int]:
    usage = _mapping(body.get("usage"))
    if not usage:
        usage = _mapping(_mapping(body.get("response")).get("usage"))
    input_tokens = _integer(usage.get("input_tokens") or usage.get("prompt_tokens"))
    output_tokens = _integer(usage.get("output_tokens") or usage.get("completion_tokens"))
    return input_tokens, output_tokens


def _request_summary(body: dict[str, Any]) -> str:
    summary: dict[str, Any] = {}
    for key in ("model", "messages", "input", "prompt", "tools", "instructions", "system"):
        if key in body:
            summary[key] = body[key]
    return _stringify(summary or body)


def _record_times(record: dict[str, Any]) -> tuple[int, int]:
    timestamp = str(record.get("timestamp") or "")
    try:
        parsed = datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
    except ValueError:
        parsed = datetime.now(timezone.utc)
    end_ns = int(parsed.timestamp() * 1_000_000_000)
    duration_ns = max(0, _integer(record.get("duration_ms"))) * 1_000_000
    return max(1, end_ns - duration_ns), max(1, end_ns)


def _body_mapping(value: Any) -> dict[str, Any]:
    if isinstance(value, dict):
        return value
    return {}


def _mapping(value: Any) -> dict[str, Any]:
    return value if isinstance(value, dict) else {}


def _list(value: Any) -> list[Any]:
    return value if isinstance(value, list) else []


def _integer(value: Any) -> int:
    try:
        return int(value or 0)
    except (TypeError, ValueError):
        return 0


def _stringify(value: Any) -> str:
    if isinstance(value, str):
        return _truncate(value)
    try:
        return _truncate(json.dumps(value, ensure_ascii=False, separators=(",", ":"), default=str))
    except (TypeError, ValueError):
        return _truncate(str(value))


def _truncate(value: str) -> str:
    if len(value) <= MAX_ATTRIBUTE_CHARS:
        return value
    return value[: MAX_ATTRIBUTE_CHARS - 20] + "…[truncated]"


def _stable_hex(seed: str, length: int) -> str:
    return hashlib.sha256(seed.encode("utf-8")).hexdigest()[:length]


def _proto_bytes(hex_value: str) -> str:
    return base64.b64encode(bytes.fromhex(hex_value)).decode("ascii")


def _dedupe_calls(calls: list[tuple[str, str, str]]) -> list[tuple[str, str, str]]:
    result: list[tuple[str, str, str]] = []
    seen: set[tuple[str, str, str]] = set()
    for call in calls:
        if call in seen:
            continue
        seen.add(call)
        result.append(call)
    return result
