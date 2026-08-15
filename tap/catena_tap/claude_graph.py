"""Claude Code transcript -> Catena Canonical Event Graph.

Turn assembly and subagent discovery come from the pinned Langfuse parser in
``claude_parser``. This module is Catena-owned: it maps the assembled runtime
objects into the cross-runtime event contract and performs source accounting.
"""

from __future__ import annotations

import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Sequence, Tuple

from catena_tap.canonical import SCHEMA_VERSION, canonical_json, ordered_unique, timestamp_to_unix_nano
from catena_tap.claude_parser import (
    SessionState,
    Turn,
    build_turns,
    extract_text_from_content,
    get_content_from_row,
    get_new_turns_from_transcript,
    get_subagent_transcripts_by_tool_use_id,
    get_tool_result_blocks,
    get_tool_use_blocks,
    get_workflow_agent_transcripts_by_run_id,
    is_interruption_row,
    is_task_notification_row,
    read_subagent_jsonl,
    source_event_id,
)

CLAUDE_UPSTREAM_COMMIT = "5b3d4323c49f3839545fad36883ed02420ebc0ba"
CLAUDE_PARSER_NAME = "langfuse-claude-derived@5b3d432"

CanonicalNode = Dict[str, Any]
CanonicalTrace = Dict[str, Any]


def _trace_id(session_id: str, turn_id: str, rows: Sequence[Dict[str, Any]]) -> str:
    for row in rows:
        for key in ("trace_id", "traceId"):
            value = row.get(key)
            if isinstance(value, str) and len(value) == 32:
                try:
                    int(value, 16)
                except ValueError:
                    continue
                if set(value) != {"0"}:
                    return value.lower()
    # Claude Code does not currently expose a W3C trace id. Its native user
    # row UUID is the turn correlation key, so the canonical W3C id is a
    # deterministic encoding of the two runtime-native identifiers.
    return hashlib.sha256(f"catena:claude-code:{session_id}:{turn_id}".encode()).hexdigest()[:32]


def _source_ids(rows: Iterable[Dict[str, Any]]) -> List[str]:
    return ordered_unique(row.get("__catena_source_event_id") for row in rows)


def _parse_time(value: Any) -> Optional[datetime]:
    if not isinstance(value, str) or not value:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed


def _time_bounds(rows: Sequence[Dict[str, Any]]) -> Tuple[str, str]:
    times = [parsed for row in rows if (parsed := _parse_time(row.get("timestamp"))) is not None]
    if not times:
        return "1", "1"
    return timestamp_to_unix_nano(min(times)), timestamp_to_unix_nano(max(times))


def _content_value(content: Any) -> Any:
    if isinstance(content, str):
        return content
    if not isinstance(content, list):
        return content
    values: List[Any] = []
    for block in content:
        if not isinstance(block, dict):
            values.append(block)
            continue
        block_type = block.get("type")
        if block_type == "text":
            values.append(block.get("text", ""))
        elif block_type not in {"tool_use", "tool_result"}:
            values.append(block)
    if len(values) == 1:
        return values[0]
    return values


def _message(row: Dict[str, Any]) -> Dict[str, Any]:
    value = row.get("message")
    return value if isinstance(value, dict) else {}


def _tool_type(name: str) -> Tuple[str, Dict[str, Any]]:
    lower = name.lower()
    if lower.startswith("mcp__"):
        parts = name.split("__", 2)
        attributes: Dict[str, Any] = {}
        if len(parts) == 3:
            attributes = {"mcp.server": parts[1], "mcp.tool": parts[2]}
        return "mcp", attributes
    if lower in {"bash", "shell", "exec_command", "local_shell"} or "shell" in lower:
        return "local_shell", {}
    if lower in {"websearch", "webfetch", "web_search"} or lower.startswith("web_"):
        return "web_search", {}
    if lower in {"glob", "grep", "read", "filesearch", "file_search"}:
        return "file_search", {}
    if lower in {"computer", "computer_use"}:
        return "computer", {}
    if lower in {"agent", "task", "workflow", "skill", "askuserquestion", "toolsearch"}:
        return "custom", {}
    return "function", {}


def _assistant_rows(turn: Turn, message_id: Optional[str]) -> List[Dict[str, Any]]:
    result: List[Dict[str, Any]] = []
    for row in turn.rows:
        message = _message(row)
        if row.get("type") != "assistant" and message.get("role") != "assistant":
            continue
        if message_id is None or message.get("id") == message_id:
            result.append(row)
    return result


def _tool_call_rows(turn: Turn, call_id: str) -> List[Dict[str, Any]]:
    result: List[Dict[str, Any]] = []
    for row in turn.rows:
        if any(str(block.get("id") or "") == call_id for block in get_tool_use_blocks(get_content_from_row(row))):
            result.append(row)
    return result


def _tool_result_rows(turn: Turn, call_id: str) -> List[Dict[str, Any]]:
    result: List[Dict[str, Any]] = []
    for row in turn.rows:
        matched = any(
            str(block.get("tool_use_id") or "") == call_id
            for block in get_tool_result_blocks(get_content_from_row(row))
        )
        if not matched and is_task_notification_row(row):
            content = extract_text_from_content(get_content_from_row(row))
            matched = f"<tool-use-id>{call_id}</tool-use-id>" in content
        if matched:
            result.append(row)
    return result


def _usage(rows: Sequence[Dict[str, Any]]) -> Optional[Dict[str, int]]:
    mapping = {
        "input_tokens": "input",
        "output_tokens": "output",
        "cache_read_input_tokens": "cache_read_input_tokens",
        "cache_creation_input_tokens": "cache_creation_input_tokens",
    }
    result: Dict[str, int] = {}
    # Claude repeats cumulative usage across some split rows. Max preserves
    # the exact model-call total without multiplying streamed fragments.
    for row in rows:
        usage = _message(row).get("usage")
        if not isinstance(usage, dict):
            continue
        for source, target in mapping.items():
            value = usage.get(source)
            if isinstance(value, int) and value >= 0:
                result[target] = max(result.get(target, 0), value)
    return result or None


def _is_retry_row(row: Dict[str, Any]) -> bool:
    if row.get("isApiErrorMessage") is True:
        return True
    subtype = str(row.get("subtype") or "").lower()
    return subtype in {"api_error", "model_retry", "http_retry", "retry"}


def _abort_rows(turn: Turn) -> List[Dict[str, Any]]:
    result: List[Dict[str, Any]] = []
    for row in turn.rows:
        subtype = str(row.get("subtype") or "").lower()
        stop_reason = str(_message(row).get("stop_reason") or "").lower()
        if is_interruption_row(row) or subtype in {"interrupt", "user_cancel", "cancelled", "aborted"}:
            result.append(row)
        elif stop_reason in {"abort", "aborted", "cancelled", "canceled"}:
            result.append(row)
    return result


def _compact_rows(turn: Turn) -> List[Dict[str, Any]]:
    return [
        row
        for row in turn.rows
        if row.get("subtype") == "compact_boundary"
        or row.get("type") in {"context_compact", "compact_boundary", "summary"}
        or isinstance(row.get("compactMetadata"), dict)
    ]


def _model_output(assistant: Dict[str, Any]) -> Dict[str, Any]:
    content = get_content_from_row(assistant)
    output: Dict[str, Any] = {}
    text = extract_text_from_content(content)
    if text:
        output["content"] = text
    tool_calls = []
    for block in get_tool_use_blocks(content):
        tool_calls.append(
            {
                "id": block.get("id"),
                "name": block.get("name"),
                "arguments": block.get("input"),
            }
        )
    if tool_calls:
        output["tool_calls"] = tool_calls
    return output


def _result_input(call_ids: Sequence[str], turn: Turn) -> Optional[List[Dict[str, Any]]]:
    values: List[Dict[str, Any]] = []
    for call_id in call_ids:
        result = turn.tool_results_by_id.get(call_id)
        if not isinstance(result, dict):
            continue
        values.append(
            {
                "call_id": call_id,
                "content": result.get("final_content", result.get("content")),
                "is_error": result.get("is_error") is True,
            }
        )
    return values or None


def _append_turn_nodes(
    nodes: List[CanonicalNode],
    turn: Turn,
    session_id: str,
    *,
    key_prefix: str,
    parent_key: Optional[str] = None,
    turn_kind: str = "turn",
    transcript_path: Optional[Path] = None,
    depth: int = 0,
) -> Tuple[str, str]:
    turn_id = turn.user_msg.get("uuid")
    if not isinstance(turn_id, str) or not turn_id:
        raise ValueError("Claude user row is missing its native uuid turn correlation")
    turn_key = f"{key_prefix}:turn:{turn_id}"
    start, end = _time_bounds(turn.rows)
    abort_rows = _abort_rows(turn)
    compact_rows = _compact_rows(turn)
    call_ids: set[str] = set()
    failed_tool = False
    missing_tool = False
    retry_count = 0
    successful_model_count = 0

    user_input = _content_value(get_content_from_row(turn.user_msg))
    final_text = ""
    if turn.assistant_msgs:
        final_text = extract_text_from_content(get_content_from_row(turn.assistant_msgs[-1]))

    turn_node: CanonicalNode = {
        "key": turn_key,
        **({"parent_key": parent_key} if parent_key else {}),
        "kind": turn_kind,
        "name": "agent.subagent.turn" if turn_kind == "subagent" else "agent.turn",
        "runtime_id": turn_id,
        "start_time_unix_nano": start,
        "end_time_unix_nano": end,
        "state": "incomplete",
        "input": user_input,
        **({"output": final_text} if final_text else {}),
        "attributes": {
            "agent.turn.id": turn_id,
            "claude.user.uuid": turn_id,
        },
        "source_event_ids": _source_ids(turn.rows),
    }
    nodes.append(turn_node)

    previous_call_ids: List[str] = []
    tool_parent_by_id: Dict[str, str] = {}
    for model_index, assistant in enumerate(turn.assistant_msgs):
        message = _message(assistant)
        message_id = message.get("id")
        message_id = message_id if isinstance(message_id, str) and message_id else None
        source_rows = _assistant_rows(turn, message_id)
        retry = any(_is_retry_row(row) for row in source_rows)
        retry_count += int(retry)
        successful_model_count += int(not retry)
        identity = message_id or (_source_ids(source_rows) or [f"index:{model_index}"])[0]
        model_key = f"{turn_key}:model:{identity}"
        model_start, model_end = _time_bounds(source_rows or [assistant])
        request_ids = ordered_unique(row.get("requestId") for row in source_rows)
        model_node: CanonicalNode = {
            "key": model_key,
            "parent_key": turn_key,
            "kind": "retry" if retry else "model",
            "name": "gen_ai.model.retry" if retry else "gen_ai.model.call",
            **({"runtime_id": message_id} if message_id else {}),
            "start_time_unix_nano": model_start,
            "end_time_unix_nano": model_end,
            "state": "retry" if retry else "ok",
            **({"status_message": extract_text_from_content(get_content_from_row(assistant))} if retry else {}),
            "input": user_input if model_index == 0 else _result_input(previous_call_ids, turn),
            "output": _model_output(assistant),
            "model": str(message.get("model") or "claude"),
            **({"usage": _usage(source_rows)} if _usage(source_rows) else {}),
            "attributes": {
                "catena.model.step.index": model_index,
                **({"gen_ai.request.id": request_ids[0]} if request_ids else {}),
            },
            "source_event_ids": _source_ids(source_rows),
        }
        nodes.append(model_node)

        current_call_ids: List[str] = []
        for tool_index, block in enumerate(get_tool_use_blocks(get_content_from_row(assistant))):
            call_id = str(block.get("id") or "")
            name = str(block.get("name") or "tool")
            call_rows = _tool_call_rows(turn, call_id) if call_id else source_rows
            result_rows = _tool_result_rows(turn, call_id) if call_id else []
            result = turn.tool_results_by_id.get(call_id) if call_id else None
            result = result if isinstance(result, dict) else None
            tool_start, _ = _time_bounds(call_rows or source_rows or [assistant])
            _, tool_end = _time_bounds(result_rows or call_rows or source_rows or [assistant])
            tool_state = "incomplete"
            status_message = "tool result not present in transcript"
            output: Any = None
            if result is not None:
                output = result.get("final_content", result.get("content"))
                if result.get("is_error") is True:
                    tool_state = "error"
                    status_message = extract_text_from_content(output) or "Claude tool returned is_error=true"
                    failed_tool = True
                else:
                    tool_state = "ok"
                    status_message = ""
            else:
                missing_tool = True
            tool_kind, extra_attributes = _tool_type(name)
            identity = call_id or (_source_ids(call_rows) or [f"index:{tool_index}"])[0]
            tool_key = f"{turn_key}:tool:{identity}"
            if call_id:
                call_ids.add(call_id)
                current_call_ids.append(call_id)
                tool_parent_by_id[call_id] = tool_key
            nodes.append(
                {
                    "key": tool_key,
                    "parent_key": model_key,
                    "kind": "tool",
                    "name": f"agent.tool.call {name}",
                    **({"runtime_id": call_id} if call_id else {}),
                    "start_time_unix_nano": tool_start,
                    "end_time_unix_nano": tool_end,
                    "state": tool_state,
                    **({"status_message": status_message} if status_message else {}),
                    "input": block.get("input"),
                    **({"output": output} if result is not None else {}),
                    "attributes": {
                        "gen_ai.tool.type": tool_kind,
                        "gen_ai.tool.name": name,
                        **({"gen_ai.tool.call.id": call_id} if call_id else {}),
                        **extra_attributes,
                    },
                    "source_event_ids": ordered_unique([*_source_ids(call_rows), *_source_ids(result_rows)]),
                }
            )
        previous_call_ids = current_call_ids

    # A non-empty unknown result ID is evidence of corruption or an unsupported
    # runtime shape. It gets its own error node and is never rebound by order.
    for call_id, result in turn.tool_results_by_id.items():
        if call_id in call_ids:
            continue
        result_rows = _tool_result_rows(turn, call_id)
        result_start, result_end = _time_bounds(result_rows or turn.rows[-1:])
        nodes.append(
            {
                "key": f"{turn_key}:unmatched-tool-result:{call_id}",
                "parent_key": turn_key,
                "kind": "unmatched_tool_result",
                "name": "agent.tool.unmatched_result",
                "runtime_id": call_id,
                "start_time_unix_nano": result_start,
                "end_time_unix_nano": result_end,
                "state": "error",
                "status_message": f"tool result references unknown tool_use_id {call_id}",
                "output": result.get("final_content", result.get("content")),
                "attributes": {"gen_ai.tool.call.id": call_id},
                "source_event_ids": _source_ids(result_rows),
            }
        )
        failed_tool = True

    for compact_index, row in enumerate(compact_rows):
        compact_start, compact_end = _time_bounds([row])
        metadata = row.get("compactMetadata")
        metadata = metadata if isinstance(metadata, dict) else {}
        event_id = str(row.get("uuid") or row.get("__catena_source_event_id") or compact_index)
        nodes.append(
            {
                "key": f"{turn_key}:context-compact:{event_id}",
                "parent_key": turn_key,
                "kind": "context_compact",
                "name": "agent.context.compact",
                **({"runtime_id": str(row.get("uuid"))} if row.get("uuid") else {}),
                "start_time_unix_nano": compact_start,
                "end_time_unix_nano": compact_end,
                "state": "ok",
                "attributes": {
                    **({"agent.context.compact.trigger": metadata.get("trigger")} if metadata.get("trigger") else {}),
                    **(
                        {"agent.context.tokens.before": metadata.get("preTokens")}
                        if isinstance(metadata.get("preTokens"), int)
                        else {}
                    ),
                    **(
                        {"agent.context.tokens.after": metadata.get("postTokens")}
                        if isinstance(metadata.get("postTokens"), int)
                        else {}
                    ),
                    **(
                        {"agent.context.compact.duration_ms": metadata.get("durationMs")}
                        if isinstance(metadata.get("durationMs"), int)
                        else {}
                    ),
                },
                "source_event_ids": _source_ids([row]),
            }
        )

    # Attach classic and Workflow subagent transcripts only through their
    # exact launching tool_use_id/runId correlation.
    if transcript_path is not None and depth < 8:
        subagents = get_subagent_transcripts_by_tool_use_id(transcript_path)
        for launch_id, metadata in subagents.items():
            if launch_id not in tool_parent_by_id:
                continue
            _append_subagent_thread(
                nodes,
                session_id,
                launch_id,
                tool_parent_by_id[launch_id],
                metadata,
                depth + 1,
            )
        workflows = get_workflow_agent_transcripts_by_run_id(transcript_path)
        for launch_id, result in turn.tool_results_by_id.items():
            if launch_id not in tool_parent_by_id or not isinstance(result, dict):
                continue
            run_id = result.get("workflow_run_id")
            if not isinstance(run_id, str):
                continue
            for index, metadata in enumerate(workflows.get(run_id, [])):
                _append_subagent_thread(
                    nodes,
                    session_id,
                    launch_id,
                    tool_parent_by_id[launch_id],
                    {**metadata, "agent_id": metadata.get("agent_id") or f"{run_id}:{index}"},
                    depth + 1,
                )

    if abort_rows:
        turn_state = "aborted"
        turn_node["status_message"] = "Claude Code turn interrupted by user"
    elif failed_tool or (retry_count > 0 and successful_model_count == 0):
        turn_state = "error"
        turn_node["status_message"] = "turn contains a failed tool or exhausted model retry"
    elif not turn.assistant_msgs or missing_tool:
        turn_state = "incomplete"
        turn_node["status_message"] = "turn has no completed response or has an unresolved tool call"
    else:
        turn_state = "ok"
    turn_node["state"] = turn_state
    turn_node["attributes"]["agent.turn.model_calls"] = len(turn.assistant_msgs)
    turn_node["attributes"]["agent.turn.tool_calls"] = len(call_ids)
    turn_node["attributes"]["agent.turn.retries"] = retry_count
    return turn_key, turn_state


def _append_subagent_thread(
    nodes: List[CanonicalNode],
    session_id: str,
    launch_id: str,
    parent_tool_key: str,
    metadata: Dict[str, Any],
    depth: int,
) -> None:
    path_value = metadata.get("path")
    agent_id = str(metadata.get("agent_id") or "")
    if not isinstance(path_value, Path):
        path_value = Path(path_value) if isinstance(path_value, str) else None
    rows = read_subagent_jsonl(path_value) if path_value is not None else None
    thread_key = f"{parent_tool_key}:subagent-thread:{agent_id or launch_id}"
    start, end = _time_bounds(rows or [])
    nodes.append(
        {
            "key": thread_key,
            "parent_key": parent_tool_key,
            "kind": "subagent",
            "name": "agent.subagent.thread",
            "runtime_id": agent_id or launch_id,
            "start_time_unix_nano": start,
            "end_time_unix_nano": end,
            "state": "ok" if rows else "incomplete",
            **({"status_message": "subagent transcript missing or empty"} if not rows else {}),
            "attributes": {
                "agent.subagent.thread.id": agent_id or launch_id,
                "agent.subagent.spawn.call_id": launch_id,
                **({"agent.subagent.type": metadata.get("agent_type")} if metadata.get("agent_type") else {}),
                **({"agent.subagent.description": metadata.get("description")} if metadata.get("description") else {}),
            },
            "source_event_ids": [],
        }
    )
    if not rows:
        return
    for turn in build_turns(rows):
        _append_turn_nodes(
            nodes,
            turn,
            session_id,
            key_prefix=thread_key,
            parent_key=thread_key,
            turn_kind="subagent",
            transcript_path=path_value,
            depth=depth,
        )


def _account(nodes: Sequence[CanonicalNode], records: Sequence[Tuple[str, str]]) -> List[Dict[str, Any]]:
    priority = {
        "unmatched_tool_result": 7,
        "tool": 6,
        "context_compact": 5,
        "retry": 4,
        "model": 3,
        "subagent": 2,
        "turn": 1,
    }
    primary: Dict[str, str] = {}
    for node in sorted(nodes, key=lambda value: priority.get(value["kind"], 0), reverse=True):
        for event_id in node["source_event_ids"]:
            primary.setdefault(event_id, node["key"])
    accounting: List[Dict[str, Any]] = []
    seen: set[str] = set()
    for event_id, event_type in records:
        if event_id in seen:
            continue
        seen.add(event_id)
        node_key = primary.get(event_id)
        if node_key:
            accounting.append({"event_id": event_id, "disposition": "span", "node_key": node_key})
        else:
            accounting.append(
                {
                    "event_id": event_id,
                    "disposition": "ignored",
                    "reason": f"runtime record {event_type} has no canonical semantic node",
                }
            )
    return accounting


def _records(rows: Iterable[Dict[str, Any]]) -> List[Tuple[str, str]]:
    result: List[Tuple[str, str]] = []
    for row in rows:
        event_id = row.get("__catena_source_event_id")
        if isinstance(event_id, str):
            event_type = str(row.get("subtype") or row.get("type") or "unknown")
            result.append((event_id, event_type))
    return result


def _session_id(turns: Sequence[Turn], expected: Optional[str], transcript_path: Path) -> str:
    values = ordered_unique(row.get("sessionId") or row.get("session_id") for turn in turns for row in turn.rows)
    if expected:
        if values and any(value != expected for value in values):
            raise ValueError(f"Claude hook session_id {expected} does not match transcript {values}")
        return expected
    if len(values) > 1:
        raise ValueError(f"Claude transcript contains multiple session ids: {values}")
    if values:
        return values[0]
    # Claude stores the native session UUID as the transcript filename. This
    # is a runtime identifier, not a Catena capture-session surrogate.
    if transcript_path.stem:
        return transcript_path.stem
    raise ValueError("Claude transcript has no runtime session id")


def graph_from_turns(
    transcript_path: Path,
    turns: Sequence[Turn],
    *,
    expected_session_id: Optional[str] = None,
    additional_records: Sequence[Tuple[str, str]] = (),
) -> Dict[str, Any]:
    session_id = _session_id(turns, expected_session_id, transcript_path)
    traces: List[CanonicalTrace] = []
    assigned_record_ids: set[str] = set()
    for turn in turns:
        nodes: List[CanonicalNode] = []
        _, state = _append_turn_nodes(
            nodes,
            turn,
            session_id,
            key_prefix="main",
            transcript_path=transcript_path,
        )
        turn_id = str(turn.user_msg["uuid"])
        records = _records(turn.rows)
        # Subagent source ids live only in the expanded nodes, so include them
        # in the same trace's one-to-one accounting ledger.
        known = {event_id for event_id, _ in records}
        for node in nodes:
            for event_id in node["source_event_ids"]:
                if event_id not in known:
                    records.append((event_id, "subagent"))
                    known.add(event_id)
        assigned_record_ids.update(event_id for event_id, _ in records)
        traces.append(
            {
                "trace_id": _trace_id(session_id, turn_id, turn.rows),
                "turn_id": turn_id,
                "state": state,
                "nodes": nodes,
                "accounting": _account(nodes, records),
            }
        )

    extras = [record for record in additional_records if record[0] not in assigned_record_ids]
    if extras and traces:
        traces[0]["accounting"].extend(_account(traces[0]["nodes"], extras))
    return {
        "schema_version": SCHEMA_VERSION,
        "runtime": "claude-code",
        "session_id": session_id,
        "source": {
            "format": "claude-transcript-jsonl",
            "path": transcript_path.name,
            "parser": CLAUDE_PARSER_NAME,
            "upstream_commit": CLAUDE_UPSTREAM_COMMIT,
        },
        "traces": traces,
    }


def read_all_rows(transcript_path: Path) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    for raw_line in transcript_path.read_text(encoding="utf-8").splitlines():
        if not raw_line.strip():
            continue
        try:
            row = json.loads(raw_line)
        except json.JSONDecodeError:
            continue
        if not isinstance(row, dict):
            continue
        row["__catena_source_event_id"] = source_event_id(row, transcript_path, raw_line.strip())
        rows.append(row)
    return rows


def parse_claude_transcript(
    transcript_path: str | Path,
    *,
    expected_session_id: Optional[str] = None,
) -> Dict[str, Any]:
    path = Path(transcript_path)
    state = SessionState()
    subagents = get_subagent_transcripts_by_tool_use_id(path)
    turns, _ = get_new_turns_from_transcript(
        path,
        state,
        subagents,
        flush_deferred_agent_turns=True,
    )
    all_rows = read_all_rows(path)
    return graph_from_turns(
        path,
        turns,
        expected_session_id=expected_session_id,
        additional_records=_records(all_rows),
    )


def parse_claude_increment(
    transcript_path: str | Path,
    session_state: SessionState,
    *,
    expected_session_id: Optional[str] = None,
    flush: bool = False,
) -> Tuple[Dict[str, Any], SessionState]:
    path = Path(transcript_path)
    subagents = get_subagent_transcripts_by_tool_use_id(path)
    turns, session_state = get_new_turns_from_transcript(
        path,
        session_state,
        subagents,
        flush_deferred_agent_turns=flush,
    )
    if not flush:
        held_rows = session_state.open_turn.get("rows") if isinstance(session_state.open_turn, dict) else None
        if isinstance(held_rows, list) and held_rows:
            held_turns = build_turns(
                held_rows, {metadata.get("agent_id"): tool_id for tool_id, metadata in subagents.items()}
            )
            existing_ids = {turn.user_msg.get("uuid") for turn in turns}
            turns.extend(turn for turn in held_turns if turn.user_msg.get("uuid") not in existing_ids)
    graph = graph_from_turns(path, turns, expected_session_id=expected_session_id)
    return graph, session_state


def canonical_graph_json(graph: Dict[str, Any]) -> str:
    return canonical_json(graph)
