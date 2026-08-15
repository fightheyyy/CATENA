"""Live and historical runtime entry points sharing the same parsers."""

from __future__ import annotations

import copy
import hashlib
import json
import os
from pathlib import Path
from typing import Any, Dict, Optional, Tuple

from catena_tap.claude_graph import (
    canonical_graph_json,
    parse_claude_increment,
    parse_claude_transcript,
)
from catena_tap.claude_parser import (
    LOCK_FILE,
    FileLock,
    get_session_state,
    get_session_state_key,
    load_hook_state,
    save_session_state,
)
from catena_tap.exporter import OTLPHTTPClient, endpoint_from_environment, export_graph, trace_to_otlp


def _truthy(value: str) -> bool:
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _client_from_environment(environment: Dict[str, str]) -> Optional[OTLPHTTPClient]:
    api_key = environment.get("CATENA_API_KEY", "").strip()
    if not api_key:
        return None
    return OTLPHTTPClient(
        endpoint_from_environment(
            environment.get("CATENA_OTLP_ENDPOINT", ""),
            environment.get("CATENA_URL", ""),
        ),
        api_key,
        debug=_truthy(environment.get("CATENA_TRACE_DEBUG", "")),
    )


def _trace_digest(trace: Dict[str, Any]) -> str:
    encoded = json.dumps(trace, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(encoded.encode()).hexdigest()


def _mark_open_trace_incomplete(graph: Dict[str, Any], open_turn_id: Optional[str]) -> None:
    if not open_turn_id:
        return
    for trace in graph["traces"]:
        if trace["turn_id"] != open_turn_id:
            continue
        trace["state"] = "incomplete"
        for node in trace["nodes"]:
            if node["kind"] in {"turn", "subagent"} and not node.get("parent_key"):
                node["state"] = "incomplete"
                node["status_message"] = "turn is still open at this hook boundary"
                break


def run_claude_hook(
    hook_input: Dict[str, Any],
    environment: Optional[Dict[str, str]] = None,
) -> Dict[str, int]:
    """Parse and incrementally upload one Claude Stop/SessionEnd hook.

    Fail-open behavior belongs to the CLI boundary. This function returns
    explicit failures for tests and lets unexpected parser errors surface.
    Parser offsets advance only after every candidate trace is accepted.
    """
    environment = dict(os.environ if environment is None else environment)
    transcript_value = hook_input.get("transcript_path")
    session_id = hook_input.get("session_id")
    if not isinstance(transcript_value, str) or not transcript_value:
        return {"parsed": 0, "uploaded": 0, "skipped": 0, "failed": 0}
    if not isinstance(session_id, str) or not session_id:
        return {"parsed": 0, "uploaded": 0, "skipped": 0, "failed": 0}
    client = _client_from_environment(environment)
    if client is None:
        return {"parsed": 0, "uploaded": 0, "skipped": 0, "failed": 0}

    transcript_path = Path(transcript_value).expanduser().resolve()
    event_name = str(hook_input.get("hook_event_name") or hook_input.get("hook_event") or "Stop")
    flush = event_name.lower() in {"sessionend", "session_end"}
    state_key = get_session_state_key(session_id, str(transcript_path))

    with FileLock(LOCK_FILE):
        global_state = load_hook_state()
        # Work on a detached candidate. A failed upload must not consume bytes
        # or mutate the persisted emission cursor.
        session_state = copy.deepcopy(get_session_state(global_state, state_key))
        graph, session_state = parse_claude_increment(
            transcript_path,
            session_state,
            expected_session_id=session_id,
            flush=flush,
        )
        open_turn_id = None
        if not flush and isinstance(session_state.open_turn, dict):
            value = session_state.open_turn.get("user_row_uuid")
            open_turn_id = value if isinstance(value, str) else None
        _mark_open_trace_incomplete(graph, open_turn_id)

        candidates = []
        for trace in graph["traces"]:
            progress = session_state.turn_progress.get(trace["turn_id"], {})
            digest = _trace_digest(trace)
            if progress.get("trace_id") == trace["trace_id"] and progress.get("digest") == digest:
                continue
            candidates.append(trace)

        uploaded, failed = export_graph(graph, client, candidates)
        if failed:
            return {
                "parsed": len(graph["traces"]),
                "uploaded": len(uploaded),
                "skipped": len(graph["traces"]) - len(candidates),
                "failed": len(failed),
            }

        uploaded_set = set(uploaded)
        for trace in candidates:
            if trace["turn_id"] not in uploaded_set:
                continue
            session_state.turn_progress[trace["turn_id"]] = {
                "trace_id": trace["trace_id"],
                "digest": _trace_digest(trace),
                "state": trace["state"],
            }
        session_state.turn_count = max(session_state.turn_count, len(session_state.turn_numbers))
        if not save_session_state(global_state, state_key, session_state):
            # The spans are already accepted; deterministic IDs make the next
            # hook's replay a replacement, not a second logical span.
            return {
                "parsed": len(graph["traces"]),
                "uploaded": len(uploaded),
                "skipped": len(graph["traces"]) - len(candidates),
                "failed": 1,
            }
        return {
            "parsed": len(graph["traces"]),
            "uploaded": len(uploaded),
            "skipped": len(graph["traces"]) - len(candidates),
            "failed": 0,
        }


def import_claude_transcript(
    transcript_path: str | Path,
    *,
    expected_session_id: Optional[str] = None,
    output: str = "canonical",
    upload: bool = False,
    environment: Optional[Dict[str, str]] = None,
) -> Tuple[Dict[str, Any], str, list[str], list[str]]:
    graph = parse_claude_transcript(
        transcript_path,
        expected_session_id=expected_session_id,
    )
    if output == "otlp":
        rendered = (
            json.dumps(
                [trace_to_otlp(graph, trace) for trace in graph["traces"]],
                ensure_ascii=False,
                indent=2,
                sort_keys=True,
            )
            + "\n"
        )
    else:
        rendered = canonical_graph_json(graph)
    if not upload:
        return graph, rendered, [], []
    client = _client_from_environment(dict(os.environ if environment is None else environment))
    if client is None:
        raise ValueError("CATENA_API_KEY is required for historical upload")
    uploaded, failed = export_graph(graph, client)
    return graph, rendered, uploaded, failed
