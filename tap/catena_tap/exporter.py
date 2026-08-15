"""Catena-owned deterministic OTLP/HTTP exporter.

This replaces both the old proxy-record exporter and Langfuse's emitter. It
accepts only Catena Canonical Event Graphs and emits stable span identities so
retries are idempotent in the Go receiver/ClickHouse storage key.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import sys
import time
import urllib.error
import urllib.request
from typing import Any, Dict, Iterable, List, Optional, Tuple

RUNTIME_VERSION = "0.2.0"


def _canonical_string(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def _stable_span_id(runtime: str, session_id: str, trace_id: str, node_key: str) -> str:
    raw = f"catena:{runtime}:{session_id}:{trace_id}:{node_key}"
    return hashlib.sha256(raw.encode()).hexdigest()[:16]


def _proto_bytes(hex_value: str) -> str:
    return base64.b64encode(bytes.fromhex(hex_value)).decode("ascii")


def _any_value(value: Any) -> Dict[str, Any]:
    if isinstance(value, bool):
        return {"boolValue": value}
    if isinstance(value, int):
        return {"intValue": str(value)}
    if isinstance(value, float):
        return {"doubleValue": value}
    return {"stringValue": value if isinstance(value, str) else _canonical_string(value)}


def _attributes(values: Dict[str, Any]) -> List[Dict[str, Any]]:
    return [
        {"key": key, "value": _any_value(value)} for key, value in values.items() if value is not None and value != ""
    ]


def _node_attributes(graph: Dict[str, Any], trace: Dict[str, Any], node: Dict[str, Any]) -> Dict[str, Any]:
    values: Dict[str, Any] = {
        "agent.runtime": graph["runtime"],
        "agent.session.id": graph["session_id"],
        "agent.turn.id": trace["turn_id"],
        "catena.canonical.schema": graph["schema_version"],
        "catena.node.key": node["key"],
        "catena.node.kind": node["kind"],
        "catena.state": node["state"],
        "catena.source.event.ids": json.dumps(node["source_event_ids"], separators=(",", ":")),
        **node.get("attributes", {}),
    }
    if node.get("runtime_id"):
        values["catena.runtime.id"] = node["runtime_id"]
    if "input" in node:
        values["input.value"] = _canonical_string(node["input"])
    if "output" in node:
        values["output.value"] = _canonical_string(node["output"])
    if node.get("model"):
        values["gen_ai.request.model"] = node["model"]
        values["gen_ai.response.model"] = node["model"]
    if node["kind"] in {"tool", "unmatched_tool_result"}:
        call_id = node.get("attributes", {}).get("gen_ai.tool.call.id") or node.get("runtime_id")
        if call_id:
            values["gen_ai.tool.call.id"] = call_id
        if "input" in node:
            values["gen_ai.tool.call.arguments"] = _canonical_string(node["input"])
            values["tool.call.arguments"] = _canonical_string(node["input"])
        if "output" in node:
            values["gen_ai.tool.call.result"] = _canonical_string(node["output"])
            values["tool.call.result"] = _canonical_string(node["output"])
    for key, value in node.get("usage", {}).items():
        values[f"gen_ai.usage.{key}"] = value
    return values


def trace_to_otlp(graph: Dict[str, Any], trace: Dict[str, Any]) -> Dict[str, Any]:
    span_ids: Dict[str, str] = {}
    for node in trace["nodes"]:
        if node["key"] in span_ids:
            raise ValueError(f"duplicate canonical node key {node['key']}")
        span_ids[node["key"]] = _stable_span_id(graph["runtime"], graph["session_id"], trace["trace_id"], node["key"])

    spans: List[Dict[str, Any]] = []
    for node in trace["nodes"]:
        parent_id: Optional[str] = None
        if node.get("parent_key"):
            parent_id = span_ids.get(node["parent_key"])
            if parent_id is None:
                raise ValueError(f"canonical parent {node['parent_key']} is missing for {node['key']}")
        span: Dict[str, Any] = {
            "traceId": _proto_bytes(trace["trace_id"]),
            "spanId": _proto_bytes(span_ids[node["key"]]),
            "name": node["name"],
            "kind": 3 if node["kind"] in {"model", "retry"} else 1,
            "startTimeUnixNano": node["start_time_unix_nano"],
            "endTimeUnixNano": node["end_time_unix_nano"],
            "attributes": _attributes(_node_attributes(graph, trace, node)),
            "status": {
                "code": 1 if node["state"] == "ok" else 2,
                **({"message": node["status_message"]} if node.get("status_message") else {}),
            },
        }
        if parent_id:
            span["parentSpanId"] = _proto_bytes(parent_id)
        spans.append(span)

    return {
        "resourceSpans": [
            {
                "resource": {
                    "attributes": _attributes(
                        {
                            "service.name": f"catena-runtime-{graph['runtime']}",
                            "agent.runtime": graph["runtime"],
                            "agent.session.id": graph["session_id"],
                            "telemetry.sdk.name": "catena-runtime",
                            "telemetry.sdk.language": "python",
                            "telemetry.sdk.version": RUNTIME_VERSION,
                        }
                    )
                },
                "scopeSpans": [
                    {
                        "scope": {"name": "catena.runtime", "version": RUNTIME_VERSION},
                        "spans": spans,
                    }
                ],
            }
        ]
    }


def endpoint_from_environment(explicit_endpoint: str = "", explicit_url: str = "") -> str:
    endpoint = explicit_endpoint.strip() or os.environ.get("CATENA_OTLP_ENDPOINT", "").strip()
    if endpoint:
        return endpoint
    base_url = explicit_url.strip() or os.environ.get("CATENA_URL", "").strip() or "http://127.0.0.1:5570"
    return base_url.rstrip("/") + "/v1/otlp/v1/traces"


class OTLPHTTPClient:
    def __init__(
        self,
        endpoint: str,
        api_key: str,
        *,
        timeout: float = 4.0,
        attempts: int = 3,
        debug: bool = False,
    ) -> None:
        self.endpoint = endpoint
        self.api_key = api_key
        self.timeout = timeout
        self.attempts = max(1, attempts)
        self.debug = debug
        self._warned = False

    def send(self, payload: Dict[str, Any]) -> bool:
        body = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        request = urllib.request.Request(
            self.endpoint,
            data=body,
            method="POST",
            headers={
                "Authorization": f"Bearer {self.api_key}",
                "Content-Type": "application/json",
                "User-Agent": f"catena-runtime/{RUNTIME_VERSION}",
            },
        )
        last_error: Exception | None = None
        for attempt in range(self.attempts):
            try:
                with urllib.request.urlopen(request, timeout=self.timeout) as response:
                    if 200 <= response.status < 300:
                        return True
                    last_error = RuntimeError(f"HTTP {response.status}")
                    if response.status not in {408, 425, 429} and response.status < 500:
                        break
            except urllib.error.HTTPError as exc:
                last_error = exc
                if exc.code not in {408, 425, 429} and exc.code < 500:
                    break
            except (OSError, urllib.error.URLError) as exc:
                last_error = exc
            if attempt + 1 < self.attempts:
                time.sleep(0.1 * (attempt + 1))
        if self.debug or not self._warned:
            print(
                f"catena runtime: OTLP upload failed; Runtime execution was not affected ({last_error})",
                file=sys.stderr,
            )
            self._warned = True
        return False


def export_graph(
    graph: Dict[str, Any],
    client: OTLPHTTPClient,
    traces: Optional[Iterable[Dict[str, Any]]] = None,
) -> Tuple[List[str], List[str]]:
    uploaded: List[str] = []
    failed: List[str] = []
    for trace in traces if traces is not None else graph["traces"]:
        target = uploaded if client.send(trace_to_otlp(graph, trace)) else failed
        target.append(trace["turn_id"])
    return uploaded, failed
