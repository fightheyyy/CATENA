"""Claude transcript parser derived from Langfuse's MIT-licensed hook.

Upstream: https://github.com/langfuse/claude-observability-plugin
Commit: 5b3d4323c49f3839545fad36883ed02420ebc0ba
Catena removes the Langfuse SDK/emitter and retains the incremental reader,
turn assembly, strict tool_use_id routing, and subagent discovery algorithms.
See ../third_party/langfuse-claude/SOURCE.md for the modification record.
"""

from __future__ import annotations

import hashlib
import json
import os
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

DEBUG = os.environ.get("CATENA_TRACE_DEBUG", "").strip().lower() in {"1", "true", "yes", "on"}
MAX_PENDING_TASK_NOTIFICATIONS = 50
try:
    MAX_CHARS = int(os.environ.get("CATENA_TRACE_MAX_CHARS", "20000"))
except ValueError:
    MAX_CHARS = 20000

STATE_DIR = Path(os.environ.get("CATENA_TRACE_STATE_DIR", "~/.catena/runtime/claude")).expanduser()
STATE_FILE = STATE_DIR / "state.json"
LOCK_FILE = STATE_DIR / "state.lock"


def debug(message: str) -> None:
    if DEBUG:
        print(f"[catena-runtime] {message}", file=sys.stderr)


def info(message: str) -> None:
    debug(message)


def source_event_id(row: Dict[str, Any], transcript_path: Path, raw_line: str = "") -> str:
    row_uuid = row.get("uuid")
    identity = str(row_uuid) if isinstance(row_uuid, str) and row_uuid else ""
    if not identity:
        message = row.get("message")
        message_id = message.get("id") if isinstance(message, dict) else None
        if isinstance(message_id, str) and message_id:
            identity = message_id
    if not identity:
        stable = raw_line or json.dumps(row, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
        identity = hashlib.sha256(stable.encode("utf-8")).hexdigest()[:20]
    # Distinguish identically named workflow-agent transcripts without using
    # an absolute machine-local path (fixtures and imports must be portable).
    parts = transcript_path.parts
    if "subagents" in parts:
        subagents_index = parts.index("subagents")
        source_scope = "/".join(parts[max(0, subagents_index - 1) :])
    else:
        source_scope = transcript_path.name
    scope_key = hashlib.sha256(source_scope.encode("utf-8")).hexdigest()[:12]
    return f"{transcript_path.name}:{scope_key}:row:{identity}"


class FileLock:
    def __init__(self, path: Path, timeout_s: float = 2.0):
        self.path = path
        self.timeout_s = timeout_s
        self._fh = None

    def __enter__(self):
        STATE_DIR.mkdir(parents=True, exist_ok=True)
        self._fh = open(self.path, "a+", encoding="utf-8")
        self.acquired = False
        try:
            import fcntl  # Unix only
        except ImportError:
            # No fcntl available (e.g. Windows) — proceed without lock.
            return self
        deadline = time.time() + self.timeout_s
        try:
            while True:
                try:
                    fcntl.flock(self._fh.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                    self.acquired = True
                    return self
                except BlockingIOError:
                    if time.time() > deadline:
                        raise TimeoutError(f"could not acquire {self.path} within {self.timeout_s}s")
                    time.sleep(0.05)
        except BaseException:
            # __exit__ is not called when __enter__ raises — close the fh
            # we just opened so it doesn't leak.
            try:
                self._fh.close()
            except Exception:
                pass
            raise

    def __exit__(self, exc_type, exc, tb):
        try:
            import fcntl

            fcntl.flock(self._fh.fileno(), fcntl.LOCK_UN)
        except Exception:
            pass
        try:
            self._fh.close()
        except Exception:
            pass


# ----------------- State file reading and writing -----------------
def load_hook_state() -> Dict[str, Any]:
    try:
        if not STATE_FILE.exists():
            return {}
        return json.loads(STATE_FILE.read_text(encoding="utf-8"))
    except Exception:
        return {}


def get_session_state_key(session_id: str, transcript_path: str) -> str:
    # stable key even if session_id collides
    raw = f"{session_id}::{transcript_path}"
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


@dataclass
class SessionState:
    offset: int = 0  # Last byte read from the transcript file.
    buffer: str = ""  # Partial JSONL line kept between hook runs.
    turn_count: int = 0  # Turns already emitted for this session.
    pending_agent_turns: List[Dict[str, Any]] = field(default_factory=list)
    # Task-notification rows whose tool_use_id could not be resolved yet
    # (task-id-only and the subagent meta.json not on disk); retried each run.
    pending_task_notifications: List[Dict[str, Any]] = field(default_factory=list)
    # Trailing turn kept while it may still continue; see build_open_turn
    # for the structure (rows plus emission cursor).
    open_turn: Dict[str, Any] = field(default_factory=dict)
    # Turn numbers assigned when a turn is first seen (keyed by its user-row
    # uuid), so a turn keeps its number regardless of when it is emitted.
    turn_numbers: Dict[str, int] = field(default_factory=dict)
    # Per-turn emission progress (keyed by user-row uuid): trace_id,
    # root_span_id and the keys of already-emitted observations. Carries a
    # partially emitted turn across firings and across the open -> closed ->
    # deferred transitions; entries are dropped once the turn is finalized.
    turn_progress: Dict[str, Dict[str, Any]] = field(default_factory=dict)


def get_session_state(global_state: Dict[str, Any], key: str) -> SessionState:
    s = global_state.get(key, {})
    pending_agent_turns = s.get("pending_agent_turns")
    if not isinstance(pending_agent_turns, list):
        pending_agent_turns = []
    pending_task_notifications = s.get("pending_task_notifications")
    if not isinstance(pending_task_notifications, list):
        pending_task_notifications = []
    open_turn = s.get("open_turn")
    if not isinstance(open_turn, dict):
        open_turn = {}
    turn_numbers = s.get("turn_numbers")
    if not isinstance(turn_numbers, dict):
        turn_numbers = {}
    turn_progress = s.get("turn_progress")
    if not isinstance(turn_progress, dict):
        turn_progress = {}
    return SessionState(
        offset=int(s.get("offset", 0)),
        buffer=str(s.get("buffer", "")),
        turn_count=int(s.get("turn_count", 0)),
        pending_agent_turns=pending_agent_turns,
        pending_task_notifications=pending_task_notifications,
        open_turn=open_turn,
        turn_numbers=turn_numbers,
        turn_progress=turn_progress,
    )


def update_session_state(global_state: Dict[str, Any], key: str, session_state: SessionState) -> None:
    global_state[key] = {
        "offset": session_state.offset,
        "buffer": session_state.buffer,
        "turn_count": session_state.turn_count,
        "pending_agent_turns": session_state.pending_agent_turns or [],
        "pending_task_notifications": session_state.pending_task_notifications or [],
        "open_turn": session_state.open_turn or {},
        "turn_numbers": session_state.turn_numbers or {},
        "turn_progress": session_state.turn_progress or {},
        "updated": datetime.now(timezone.utc).isoformat(),
    }


def save_hook_state(state: Dict[str, Any]) -> bool:
    try:
        # Drop session entries older than 30 days to keep the file bounded.
        cutoff = datetime.now(timezone.utc) - timedelta(days=30)
        for k in list(state.keys()):
            entry = state.get(k)
            if not isinstance(entry, dict):
                continue
            updated = entry.get("updated")
            if not isinstance(updated, str):
                continue
            try:
                ts = datetime.fromisoformat(updated.replace("Z", "+00:00"))
            except Exception:
                continue
            if ts < cutoff:
                del state[k]
        STATE_DIR.mkdir(parents=True, exist_ok=True)
        tmp = STATE_FILE.with_suffix(".tmp")
        tmp.write_text(json.dumps(state, indent=2, sort_keys=True), encoding="utf-8")
        os.replace(tmp, STATE_FILE)
        return True
    except Exception as e:
        debug(f"save_hook_state failed: {e}")
        return False


def save_session_state(global_state: Dict[str, Any], key: str, session_state: SessionState) -> bool:
    update_session_state(global_state, key, session_state)
    return save_hook_state(global_state)


# ----------------- Transcript row parsing -----------------
def get_content_from_row(row: Dict[str, Any]) -> Any:
    if not isinstance(row, dict):
        return None
    message = row.get("message")
    if isinstance(message, dict):
        return message.get("content")
    return row.get("content")


def get_user_or_assistant_role_from_row(row: Dict[str, Any]) -> Optional[str]:
    # Claude Code transcript row format is internal. Prefer top-level row.type
    # when it marks a chat row, then fall back to nested message.role.
    row_type = row.get("type")
    if row_type in ("user", "assistant"):
        return row_type

    message = row.get("message")
    if isinstance(message, dict):
        role = message.get("role")
        if role in ("user", "assistant"):
            return role
    return None


def get_message_id(row: Dict[str, Any]) -> Optional[str]:
    m = row.get("message")
    if isinstance(m, dict):
        mid = m.get("id")
        if isinstance(mid, str) and mid:
            return mid
    return None


def get_model(row: Dict[str, Any]) -> str:
    m = row.get("message")
    if isinstance(m, dict):
        return m.get("model") or "claude"
    return "claude"


def get_usage_details_from_row(row: Dict[str, Any]) -> Optional[Dict[str, int]]:
    """Extract Anthropic token usage from an assistant message, if present."""
    m = row.get("message")
    if not isinstance(m, dict):
        return None
    u = m.get("usage")
    if not isinstance(u, dict):
        return None
    details: Dict[str, int] = {}
    for src, dst in (
        ("input_tokens", "input"),
        ("output_tokens", "output"),
        ("cache_read_input_tokens", "cache_read_input_tokens"),
        ("cache_creation_input_tokens", "cache_creation_input_tokens"),
    ):
        v = u.get(src)
        if isinstance(v, int) and v > 0:
            details[dst] = v
    return details or None


def get_speed_from_row(row: Dict[str, Any]) -> Optional[str]:
    """Extract the Anthropic request speed ("standard"/"fast") from an assistant message."""
    m = row.get("message")
    if not isinstance(m, dict):
        return None
    u = m.get("usage")
    if not isinstance(u, dict):
        return None
    speed = u.get("speed")
    if isinstance(speed, str) and speed:
        return speed
    return None


def parse_timestamp(value: Any) -> Optional[datetime]:
    """Parse a Claude Code jsonl row timestamp (ISO 8601 with trailing Z)."""
    if isinstance(value, dict):
        value = value.get("timestamp")
    if not isinstance(value, str) or not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except Exception:
        return None


def extract_text_from_content(content: Any) -> str:
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts: List[str] = []
        for x in content:
            if isinstance(x, dict) and x.get("type") == "text":
                parts.append(x.get("text", ""))
            elif isinstance(x, str):
                parts.append(x)
        return "\n".join([p for p in parts if p])
    return ""


def truncate_text(s: str, max_chars: int = MAX_CHARS) -> Tuple[str, Dict[str, Any]]:
    if s is None:
        return "", {"truncated": False, "orig_len": 0}
    orig_len = len(s)
    if orig_len <= max_chars:
        return s, {"truncated": False, "orig_len": orig_len}
    head = s[:max_chars]
    return head, {
        "truncated": True,
        "orig_len": orig_len,
        "kept_len": len(head),
        "sha256": hashlib.sha256(s.encode("utf-8")).hexdigest(),
    }


def get_tool_use_blocks(content: Any) -> List[Dict[str, Any]]:
    out: List[Dict[str, Any]] = []
    if isinstance(content, list):
        for x in content:
            if isinstance(x, dict) and x.get("type") == "tool_use":
                out.append(x)
    return out


def get_tool_result_blocks(content: Any) -> List[Dict[str, Any]]:
    out: List[Dict[str, Any]] = []
    if isinstance(content, list):
        for x in content:
            if isinstance(x, dict) and x.get("type") == "tool_result":
                out.append(x)
    return out


def is_tool_result(row: Dict[str, Any]) -> bool:
    role = get_user_or_assistant_role_from_row(row)
    if role != "user":
        return False
    content = get_content_from_row(row)
    if isinstance(content, list):
        return any(isinstance(x, dict) and x.get("type") == "tool_result" for x in content)
    return False


def is_interruption_row(row: Dict[str, Any]) -> bool:
    """Recognize Claude Code's structured user-interruption marker.

    The marker is a runtime control row, not a new prompt. Treating it as a
    user turn would both invent a turn and lose the abort state of the turn it
    terminates.
    """
    if get_user_or_assistant_role_from_row(row) != "user":
        return False
    content = extract_text_from_content(get_content_from_row(row)).strip().lower()
    return content.startswith("[request interrupted by user") or content.startswith("[request cancelled by user")


# ----------------- Incremental transcript reading -----------------
def read_new_jsonl(transcript_path: Path, session_state: SessionState) -> Tuple[List[Dict[str, Any]], SessionState]:
    """
    Reads only new bytes since session_state.offset. Keeps session_state.buffer for partial last line.
    Returns parsed JSON lines and updated state.
    """
    if not transcript_path.exists():
        return [], session_state

    try:
        file_size = transcript_path.stat().st_size
        if file_size < session_state.offset:
            # Transcript was rotated or truncated — restart from the beginning.
            debug(f"transcript shrank ({file_size} < {session_state.offset}); restarting")
            session_state.offset = 0
            session_state.buffer = ""
            # The held rows refer to the replaced file; re-reading from byte 0
            # would emit those turns a second time (and mix old rows into the
            # new stream), so drop all persisted turn state along with the offset.
            session_state.pending_agent_turns = []
            session_state.pending_task_notifications = []
            session_state.open_turn = {}
            session_state.turn_numbers = {}
            # Known limitation: rotation drops emission progress, so re-read
            # turns re-emit from scratch; an already-exported root keeps the
            # output/end time it was emitted with.
            session_state.turn_progress = {}
        with open(transcript_path, "rb") as f:
            f.seek(session_state.offset)
            chunk = f.read()
            new_offset = f.tell()
    except Exception as e:
        debug(f"read_new_jsonl failed: {e}")
        return [], session_state

    if not chunk:
        return [], session_state

    try:
        text = chunk.decode("utf-8", errors="replace")
    except Exception:
        text = chunk.decode(errors="replace")

    combined = session_state.buffer + text
    lines = combined.split("\n")
    # last element may be incomplete
    session_state.buffer = lines[-1]
    session_state.offset = new_offset

    msgs: List[Dict[str, Any]] = []
    for line in lines[:-1]:
        line = line.strip()
        if not line:
            continue
        try:
            row = json.loads(line)
        except Exception:
            continue
        if not isinstance(row, dict):
            continue
        row["__catena_source_event_id"] = source_event_id(row, transcript_path, line)
        msgs.append(row)

    return msgs, session_state


# ----------------- Turn assembly -----------------
@dataclass
class Turn:
    user_msg: Dict[str, Any]
    assistant_msgs: List[Dict[str, Any]]
    tool_results_by_id: Dict[str, Any]
    tool_use_timestamps_by_id: Dict[str, Any]
    # Injected context (e.g. skill instructions) keyed by the tool_use id it
    # belongs to, taken from isMeta rows carrying sourceToolUseID.
    injected_by_tool_id: Dict[str, str]
    rows: List[Dict[str, Any]]


@dataclass
class TurnAssemblyState:
    current_turn_user_row: Optional[Dict[str, Any]] = None
    assistant_message_ids: List[str] = field(default_factory=list)
    assistant_rows_by_message_id: Dict[str, List[Dict[str, Any]]] = field(default_factory=dict)
    tool_results_by_id: Dict[str, Any] = field(default_factory=dict)
    tool_use_timestamps_by_id: Dict[str, Any] = field(default_factory=dict)
    injected_by_tool_id: Dict[str, str] = field(default_factory=dict)
    current_rows: List[Dict[str, Any]] = field(default_factory=list)


def _extract_xml_tag_value(text: str, tag: str) -> Optional[str]:
    start = f"<{tag}>"
    end = f"</{tag}>"
    i = text.find(start)
    if i < 0:
        return None
    j = text.find(end, i + len(start))
    if j < 0:
        return None
    return text[i + len(start) : j]


def is_task_notification_row(row: Dict[str, Any]) -> bool:
    origin = row.get("origin")
    if isinstance(origin, dict) and origin.get("kind") == "task-notification":
        return True

    notification_text = extract_text_from_content(get_content_from_row(row)).lstrip()
    return notification_text.startswith("<task-notification>")


def get_tool_use_id_from_task_notification(row: Dict[str, Any]) -> Optional[str]:
    notification_text = extract_text_from_content(get_content_from_row(row))
    tool_use_id = _extract_xml_tag_value(notification_text, "tool-use-id")
    return tool_use_id.strip() if isinstance(tool_use_id, str) and tool_use_id.strip() else None


def get_task_id_from_task_notification(row: Dict[str, Any]) -> Optional[str]:
    notification_text = extract_text_from_content(get_content_from_row(row))
    task_id = _extract_xml_tag_value(notification_text, "task-id")
    return task_id.strip() if isinstance(task_id, str) and task_id.strip() else None


def get_tool_use_id_for_task_notification(
    row: Dict[str, Any],
    task_id_to_tool_use_id: Optional[Dict[str, str]] = None,
) -> Optional[str]:
    if not is_task_notification_row(row):
        return None

    tool_use_id = get_tool_use_id_from_task_notification(row)
    if tool_use_id:
        return tool_use_id

    task_id = get_task_id_from_task_notification(row)
    if task_id and task_id_to_tool_use_id:
        return task_id_to_tool_use_id.get(task_id)
    return None


def get_result_from_task_notification(row: Dict[str, Any]) -> str:
    notification_text = extract_text_from_content(get_content_from_row(row))
    result = _extract_xml_tag_value(notification_text, "result")
    return result if result is not None else notification_text


def task_notification_is_error(row: Dict[str, Any]) -> bool:
    notification_text = extract_text_from_content(get_content_from_row(row))
    status = _extract_xml_tag_value(notification_text, "status")
    return isinstance(status, str) and status.strip().lower() in {
        "error",
        "failed",
        "cancelled",
        "canceled",
    }


def _find_pending_agent_turn(
    session_state: SessionState,
    tool_use_id: str,
) -> Optional[Dict[str, Any]]:
    for pending_turn in session_state.pending_agent_turns:
        if not isinstance(pending_turn, dict):
            continue
        if not isinstance(pending_turn.get("rows"), list):
            continue
        pending_tool_use_ids = pending_turn.get("pending_tool_use_ids")
        resolved_tool_use_ids = pending_turn.get("resolved_tool_use_ids")
        # Notifications can arrive more than once per tool_use_id, so ids that
        # already received one keep matching until the whole turn resolves.
        if isinstance(pending_tool_use_ids, list) and tool_use_id in pending_tool_use_ids:
            return pending_turn
        if isinstance(resolved_tool_use_ids, list) and tool_use_id in resolved_tool_use_ids:
            return pending_turn
    return None


def resolve_deferred_agent_turns(
    rows: List[Dict[str, Any]],
    session_state: SessionState,
    task_id_to_tool_use_id: Optional[Dict[str, str]] = None,
) -> Tuple[List[List[Dict[str, Any]]], List[Dict[str, Any]]]:
    """Move task-notification rows from the batch to their deferred turns.

    Deferred rows are never spliced into the batch (a user row mid-batch would
    cut the current turn in half); resolved turns are returned for isolated
    assembly. Notifications matching a tool_use in the batch stay there, and
    ones that cannot be attributed yet (task-id-only, subagent meta.json not
    on disk) are stashed in the session state and retried on later runs
    instead of being swallowed by the turn assembly.
    """
    remaining_rows: List[Dict[str, Any]] = []
    stashed_notifications: List[Dict[str, Any]] = []

    def route_to_pending_turn(pending_turn: Dict[str, Any], row: Dict[str, Any], tool_use_id: str) -> None:
        pending_turn["rows"].append(row)
        pending_tool_use_ids = pending_turn.get("pending_tool_use_ids")
        if isinstance(pending_tool_use_ids, list) and tool_use_id in pending_tool_use_ids:
            pending_tool_use_ids.remove(tool_use_id)
            pending_turn.setdefault("resolved_tool_use_ids", []).append(tool_use_id)

    # Retry stashed notifications from earlier runs first (they are older than
    # anything in the batch); their task-id may resolve now. Entries matching
    # no deferred turn stay stashed: their owning turn may still be open and
    # only defer once a new user row closes it. Leftovers are cleared at
    # session end and the stash is size-capped.
    for row in session_state.pending_task_notifications:
        tool_use_id = get_tool_use_id_for_task_notification(row, task_id_to_tool_use_id)
        pending_turn = _find_pending_agent_turn(session_state, tool_use_id) if tool_use_id else None
        if pending_turn is None:
            stashed_notifications.append(row)
            continue
        route_to_pending_turn(pending_turn, row, tool_use_id)

    for row in rows:
        if not is_task_notification_row(row):
            remaining_rows.append(row)
            continue
        tool_use_id = get_tool_use_id_for_task_notification(row, task_id_to_tool_use_id)
        if tool_use_id is None:
            stashed_notifications.append(row)
            continue
        pending_turn = _find_pending_agent_turn(session_state, tool_use_id)
        if pending_turn is None:
            remaining_rows.append(row)
            continue
        route_to_pending_turn(pending_turn, row, tool_use_id)

    session_state.pending_task_notifications = stashed_notifications[-MAX_PENDING_TASK_NOTIFICATIONS:]

    # Pop fully resolved turns in deferral (i.e. chronological) order.
    resolved_turn_row_lists: List[List[Dict[str, Any]]] = []
    still_pending: List[Dict[str, Any]] = []
    for pending_turn in session_state.pending_agent_turns:
        if not isinstance(pending_turn, dict) or not isinstance(pending_turn.get("rows"), list):
            continue
        if pending_turn.get("pending_tool_use_ids"):
            still_pending.append(pending_turn)
            continue
        resolved_turn_row_lists.append(pending_turn["rows"])
    session_state.pending_agent_turns = still_pending

    return resolved_turn_row_lists, remaining_rows


def pop_all_deferred_agent_turn_row_lists(
    session_state: SessionState,
) -> List[List[Dict[str, Any]]]:
    row_lists: List[List[Dict[str, Any]]] = []
    for pending_turn in session_state.pending_agent_turns:
        if not isinstance(pending_turn, dict):
            continue
        rows = pending_turn.get("rows")
        if isinstance(rows, list) and rows:
            row_lists.append(rows)
    session_state.pending_agent_turns = []
    return row_lists


def get_tool_result_text(tool_result_entry: Any) -> str:
    if not isinstance(tool_result_entry, dict):
        return ""
    tool_result_content = tool_result_entry.get("content")
    if isinstance(tool_result_content, str):
        return tool_result_content
    return json.dumps(tool_result_content, ensure_ascii=False)


def get_async_launch_flag_from_row(row: Dict[str, Any]) -> Optional[bool]:
    """Read the structured async marker Claude Code puts on tool_result rows.

    Returns None when the row carries no toolUseResult (older Claude Code
    versions), so callers can fall back to the launch-text heuristic.
    """
    tool_use_result = row.get("toolUseResult")
    if not isinstance(tool_use_result, dict):
        return None
    return tool_use_result.get("status") == "async_launched" or tool_use_result.get("isAsync") is True


def get_workflow_launch_marker_from_row(row: Dict[str, Any]) -> Optional[Dict[str, Any]]:
    """Read the structured Workflow launch marker from a tool_result row.

    Workflow launches carry toolUseResult.taskType == "local_workflow" plus
    the runId that names the run's transcript directory
    (<transcript_stem>/subagents/workflows/<runId>/). Only the structured
    marker is trusted, the launch text is never parsed.
    """
    tool_use_result = row.get("toolUseResult")
    if not isinstance(tool_use_result, dict):
        return None
    if tool_use_result.get("taskType") != "local_workflow":
        return None
    run_id = tool_use_result.get("runId")
    if not isinstance(run_id, str) or not run_id:
        return None
    workflow_launch_marker: Dict[str, Any] = {"run_id": run_id}
    workflow_name = tool_use_result.get("workflowName")
    if isinstance(workflow_name, str) and workflow_name:
        workflow_launch_marker["workflow_name"] = workflow_name
    return workflow_launch_marker


def is_async_agent_launch_result(tool_result_entry: Any) -> bool:
    if not isinstance(tool_result_entry, dict):
        return False
    # Prefer the structured toolUseResult marker: launch-text matching also
    # fires on tool results that merely quote it (e.g. reading this file).
    is_async_launch = tool_result_entry.get("is_async_launch")
    if is_async_launch is not None:
        return bool(is_async_launch)
    tool_result_text = get_tool_result_text(tool_result_entry)
    return "Async agent launched successfully" in tool_result_text or (
        "agentId:" in tool_result_text
        and "output_file:" in tool_result_text
        and "You will be notified automatically" in tool_result_text
    )


def get_pending_agent_tool_use_ids(turn: Turn) -> List[str]:
    tool_use_ids: List[str] = []
    for assistant_message in turn.assistant_msgs:
        for tool_use_block in get_tool_use_blocks(get_content_from_row(assistant_message)):
            # Workflows resolve via task notifications too, so they hold the turn open.
            if tool_use_block.get("name") not in ("Agent", "Task", "Workflow"):
                continue
            tool_use_id = str(tool_use_block.get("id") or "")
            if not tool_use_id:
                continue
            tool_result_entry = turn.tool_results_by_id.get(tool_use_id)
            if isinstance(tool_result_entry, dict) and tool_result_entry.get("final_content") is not None:
                continue
            # Defer only explicit async launches: sync agents also write a
            # subagent transcript but never notify, so deferring on transcript
            # existence would strand their turns.
            if is_async_agent_launch_result(tool_result_entry):
                tool_use_ids.append(tool_use_id)
    return tool_use_ids


def get_undelivered_queued_notification_ids(rows: List[Dict[str, Any]]) -> List[str]:
    """Tool-use ids of task notifications that were enqueued (queue-operation
    rows) but not yet delivered as a user row.

    A queued result already fills the launch entry's final_content, so the
    pending-agents check goes clean — yet the turn provably continues: the
    delivery row and Claude's follow-up response are still outstanding.
    Queue remove rows carry no notification content and cannot be matched, so
    a removed notification keeps the gate closed until the turn ends — the
    safe direction (close-time emission is always correct).
    """
    queued: List[str] = []
    delivered = set()
    for row in rows:
        if not is_task_notification_row(row):
            continue
        tool_use_id = get_tool_use_id_from_task_notification(row)
        if not tool_use_id:
            continue
        if row.get("type") == "queue-operation":
            queued.append(tool_use_id)
        else:
            delivered.add(tool_use_id)
    return [tool_use_id for tool_use_id in queued if tool_use_id not in delivered]


def turn_has_unresolved_async_activity(turn: Turn) -> bool:
    """True while the turn's emitted form can provably still change: an async
    agent has not delivered its final result, or a notification is queued but
    not yet delivered. Exported roots are immutable, so emission must wait
    for this gate (or for the turn to close)."""
    return bool(get_pending_agent_tool_use_ids(turn) or get_undelivered_queued_notification_ids(turn.rows))


def get_turns_to_emit(
    turns: List[Turn],
    session_state: SessionState,
    *,
    flush_deferred_agent_turns: bool = False,
) -> List[Turn]:
    turns_to_emit: List[Turn] = []
    for turn in turns:
        pending_agent_tool_use_ids = get_pending_agent_tool_use_ids(turn)
        if pending_agent_tool_use_ids:
            if flush_deferred_agent_turns:
                debug(f"Emitting async agent turn without task notification: {pending_agent_tool_use_ids}")
                turns_to_emit.append(turn)
                continue
            session_state.pending_agent_turns.append(
                {
                    "pending_tool_use_ids": pending_agent_tool_use_ids,
                    "rows": turn.rows,
                }
            )
            debug(f"Deferred agent turn until task notification: {pending_agent_tool_use_ids}")
            continue
        turns_to_emit.append(turn)
    return turns_to_emit


def add_injected_context_row(row: Dict[str, Any], state: TurnAssemblyState) -> bool:
    # Injected user rows (slash-command expansions, caveats, skill instructions)
    # carry isMeta=true. They are not real prompts, so they must not start turns.
    if not row.get("isMeta"):
        return False

    # Skill invocations link their injected instructions to the originating
    # tool_use via sourceToolUseID; keep the text so emit can optionally attach
    # it to that tool span.
    source_tool_use_id = row.get("sourceToolUseID")
    if source_tool_use_id:
        text = extract_text_from_content(get_content_from_row(row))
        if text:
            state.injected_by_tool_id[str(source_tool_use_id)] = text
    if state.current_turn_user_row is not None:
        state.current_rows.append(row)
    return True


def add_tool_result_row(row: Dict[str, Any], state: TurnAssemblyState) -> bool:
    # tool_result rows show up as role=user with content blocks of type tool_result.
    if not is_tool_result(row):
        return False

    state.current_rows.append(row)
    row_timestamp = row.get("timestamp")
    is_async_launch = get_async_launch_flag_from_row(row)
    workflow_launch_marker = get_workflow_launch_marker_from_row(row)
    for tool_result_block in get_tool_result_blocks(get_content_from_row(row)):
        tool_use_id = tool_result_block.get("tool_use_id")
        if tool_use_id:
            tool_result_entry: Dict[str, Any] = {
                "content": tool_result_block.get("content"),
                "timestamp": row_timestamp,
                "is_error": tool_result_block.get("is_error") is True,
            }
            if is_async_launch is not None:
                tool_result_entry["is_async_launch"] = is_async_launch
            if workflow_launch_marker is not None:
                # Links the launching tool_use to its workflow run so emission
                # can attach the run's agent transcripts (which have no
                # toolUseId of their own) under this tool's span.
                tool_result_entry["workflow_run_id"] = workflow_launch_marker["run_id"]
                if "workflow_name" in workflow_launch_marker:
                    tool_result_entry["workflow_name"] = workflow_launch_marker["workflow_name"]
            state.tool_results_by_id[str(tool_use_id)] = tool_result_entry
    return True


def add_task_notification_row(
    row: Dict[str, Any],
    state: TurnAssemblyState,
    task_id_to_tool_use_id: Optional[Dict[str, str]] = None,
    closed_turns: Optional[List[Turn]] = None,
) -> bool:
    if not is_task_notification_row(row):
        return False

    tool_use_id = get_tool_use_id_for_task_notification(row, task_id_to_tool_use_id)
    if not tool_use_id:
        if state.current_turn_user_row is not None:
            state.current_rows.append(row)
        return True

    if state.current_turn_user_row is not None:
        existing_result = state.tool_results_by_id.get(tool_use_id)
        if isinstance(existing_result, dict):
            existing_result["final_content"] = get_result_from_task_notification(row)
            existing_result["final_timestamp"] = row.get("timestamp")
            if task_notification_is_error(row):
                existing_result["is_error"] = True
            state.current_rows.append(row)
            return True

    # The launching turn may have been closed earlier in this same batch (a
    # new user row arrived before the notification did).
    for closed_turn in reversed(closed_turns or []):
        closed_result = closed_turn.tool_results_by_id.get(tool_use_id)
        if isinstance(closed_result, dict):
            closed_result["final_content"] = get_result_from_task_notification(row)
            closed_result["final_timestamp"] = row.get("timestamp")
            if task_notification_is_error(row):
                closed_result["is_error"] = True
            closed_turn.rows.append(row)
            return True

    if state.current_turn_user_row is None:
        return True
    state.tool_results_by_id[tool_use_id] = {
        "content": get_result_from_task_notification(row),
        "timestamp": row.get("timestamp"),
        "is_error": task_notification_is_error(row),
    }
    state.current_rows.append(row)
    return True


def merge_assistant_rows(rows: List[Dict[str, Any]]) -> Dict[str, Any]:
    """
    Claude Code can split one assistant message across multiple JSONL rows that
    share message.id. Merge them back into one logical message by concatenating
    content blocks in row order.
    """
    base: Dict[str, Any] = dict(rows[-1])
    last_message = rows[-1].get("message")
    merged_message: Dict[str, Any] = dict(last_message) if isinstance(last_message, dict) else {}

    merged_content: List[Any] = []
    for row in rows:
        message_obj = row.get("message")
        if not isinstance(message_obj, dict):
            continue

        content_blocks = message_obj.get("content")
        if isinstance(content_blocks, list):
            merged_content.extend(content_blocks)
        elif isinstance(content_blocks, str) and content_blocks:
            merged_content.append({"type": "text", "text": content_blocks})

    merged_message["content"] = merged_content
    base["message"] = merged_message
    return base


def build_turn_from_state(state: TurnAssemblyState) -> Optional[Turn]:
    if state.current_turn_user_row is None:
        return None

    # Rebuild one assistant message per message.id, in the order the ids
    # first appeared. assistant_rows_by_message_id[message_id] holds all raw
    # rows that shared that id; merge_assistant_rows concatenates their content
    # blocks into one.
    merged_assistant_rows: List[Dict[str, Any]] = []
    for message_id in state.assistant_message_ids:
        rows_for_id = state.assistant_rows_by_message_id.get(message_id)
        if not rows_for_id:
            continue
        merged_assistant_rows.append(merge_assistant_rows(rows_for_id))

    return Turn(
        user_msg=state.current_turn_user_row,
        assistant_msgs=merged_assistant_rows,
        tool_results_by_id=dict(state.tool_results_by_id),
        tool_use_timestamps_by_id=dict(state.tool_use_timestamps_by_id),
        injected_by_tool_id=dict(state.injected_by_tool_id),
        rows=list(state.current_rows),
    )


def start_new_turn(row: Dict[str, Any], state: TurnAssemblyState) -> None:
    state.current_turn_user_row = row
    state.assistant_message_ids = []
    state.assistant_rows_by_message_id = {}
    state.tool_results_by_id = {}
    state.tool_use_timestamps_by_id = {}
    state.injected_by_tool_id = {}
    state.current_rows = [row]


def add_assistant_row(row: Dict[str, Any], state: TurnAssemblyState) -> None:
    if state.current_turn_user_row is None:
        # Ignore assistant rows until we see a user message.
        return

    message_id = get_message_id(row) or f"noid:{len(state.assistant_message_ids)}"
    if message_id not in state.assistant_rows_by_message_id:
        state.assistant_message_ids.append(message_id)
        state.assistant_rows_by_message_id[message_id] = []
    state.assistant_rows_by_message_id[message_id].append(row)

    for tool_use_block in get_tool_use_blocks(get_content_from_row(row)):
        tool_use_id = tool_use_block.get("id")
        if tool_use_id:
            state.tool_use_timestamps_by_id.setdefault(str(tool_use_id), row.get("timestamp"))
    state.current_rows.append(row)


def assemble_turns(
    rows: List[Dict[str, Any]],
    task_id_to_tool_use_id: Optional[Dict[str, str]] = None,
) -> Tuple[List[Turn], Optional[Turn], List[Dict[str, Any]]]:
    """
    Groups incremental transcript rows into turns:
    user (non-tool-result) -> assistant messages -> (tool_result rows, possibly interleaved)
    Uses:
    - assistant rows merged by message.id (all content blocks concatenated)
    - tool results dedupe by tool_use_id (latest wins)

    Returns (closed_turns, trailing_turn, trailing_turn_rows). The trailing
    turn is the one still open at the end of the rows: only a following user
    row proves a turn is complete, so incremental callers keep its raw rows
    and re-attach them to the next batch instead of emitting it right away.
    """
    turns: List[Turn] = []
    state = TurnAssemblyState()

    for row in rows:
        if is_interruption_row(row):
            if state.current_turn_user_row is not None:
                state.current_rows.append(row)
            continue

        if add_injected_context_row(row, state):
            continue

        if add_tool_result_row(row, state):
            continue

        if add_task_notification_row(row, state, task_id_to_tool_use_id, closed_turns=turns):
            continue

        role = get_user_or_assistant_role_from_row(row)

        if role == "user":
            turn = build_turn_from_state(state)
            if turn is not None:
                turns.append(turn)

            start_new_turn(row, state)
            continue

        if role == "assistant":
            add_assistant_row(row, state)
            continue

        # Preserve runtime control/metadata rows for context compaction,
        # retries, abort accounting, and future contract extensions.
        if state.current_turn_user_row is not None:
            state.current_rows.append(row)

    trailing_turn = build_turn_from_state(state)
    trailing_turn_rows = list(state.current_rows) if state.current_turn_user_row is not None else []
    return turns, trailing_turn, trailing_turn_rows


def build_turns(
    rows: List[Dict[str, Any]],
    task_id_to_tool_use_id: Optional[Dict[str, str]] = None,
) -> List[Turn]:
    """Group a complete row list into turns, including the trailing one."""
    turns, trailing_turn, _ = assemble_turns(rows, task_id_to_tool_use_id)
    if trailing_turn is not None:
        turns.append(trailing_turn)
    return turns


def build_open_turn(trailing_turn: Optional[Turn], trailing_turn_rows: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Package the trailing turn for the session state. Emission progress is
    NOT kept here: it lives in session_state.turn_progress (keyed by the
    user-row uuid) so it survives the open -> closed -> deferred transitions."""
    if not trailing_turn_rows:
        return {}
    if trailing_turn is not None:
        user_row_uuid = trailing_turn.user_msg.get("uuid")
    else:
        user_row_uuid = trailing_turn_rows[0].get("uuid")
    return {
        "user_row_uuid": user_row_uuid,
        "rows": trailing_turn_rows,
    }


def assign_turn_numbers(turns: List[Turn], trailing_turn: Optional[Turn], session_state: SessionState) -> None:
    """Assigns each turn its number the first time it is seen (in transcript
    order). Keyed by the turn's user-row uuid. Numbering seeds past
    max(turn_count, highest assigned) so a cleared or missing turn_numbers
    dict (rotation, legacy state files) cannot restart numbering at 1."""
    trailing = [trailing_turn] if trailing_turn is not None else []
    next_turn_number = 1 + max(
        session_state.turn_count,
        max(session_state.turn_numbers.values(), default=0),
    )
    for turn in turns + trailing:
        user_row_uuid = turn.user_msg.get("uuid")
        if not isinstance(user_row_uuid, str) or not user_row_uuid:
            continue
        if user_row_uuid not in session_state.turn_numbers:
            session_state.turn_numbers[user_row_uuid] = next_turn_number
            next_turn_number += 1


def get_new_turns_from_transcript(
    transcript_path: Path,
    session_state: SessionState,
    subagent_transcripts_by_tool_use_id: Optional[Dict[str, Dict[str, Any]]] = None,
    *,
    flush_deferred_agent_turns: bool = False,
) -> Tuple[List[Turn], SessionState]:
    rows, session_state = read_new_jsonl(transcript_path, session_state)
    if flush_deferred_agent_turns and session_state.buffer.strip():
        raw_line = session_state.buffer.strip()
        try:
            final_row = json.loads(raw_line)
        except Exception:
            final_row = None
        if isinstance(final_row, dict):
            final_row["__catena_source_event_id"] = source_event_id(final_row, transcript_path, raw_line)
            rows.append(final_row)
        session_state.buffer = ""
    task_id_to_tool_use_id = get_task_id_to_tool_use_id(subagent_transcripts_by_tool_use_id)

    # Re-attach the trailing open turn from the previous run. Stop fires
    # multiple times within one logical turn, so a batch can begin with
    # user-less continuation rows (task notifications, assistant rows); with
    # the open turn's rows in front they attach to it instead of being
    # dropped by turn assembly.
    previous_open_turn = session_state.open_turn if isinstance(session_state.open_turn, dict) else {}
    held_rows = previous_open_turn.get("rows")
    if isinstance(held_rows, list) and held_rows:
        rows = held_rows + rows
        session_state.open_turn = {}

    deferred_turn_row_lists, rows = resolve_deferred_agent_turns(rows, session_state, task_id_to_tool_use_id)

    if flush_deferred_agent_turns and session_state.pending_agent_turns:
        flushed_row_lists = pop_all_deferred_agent_turn_row_lists(session_state)
        if flushed_row_lists:
            debug(f"Flushing {len(flushed_row_lists)} deferred agent turn(s) without task notification")
            deferred_turn_row_lists = deferred_turn_row_lists + flushed_row_lists

    if flush_deferred_agent_turns and session_state.pending_task_notifications:
        # Last chance: appended to the row stream, a stashed notification can
        # still attach to the (reattached) open turn or a turn closed in this
        # batch; anything unmatched is discarded with the session.
        debug(f"Replaying {len(session_state.pending_task_notifications)} stashed task notification(s) at session end")
        rows = rows + session_state.pending_task_notifications
        session_state.pending_task_notifications = []

    # Each deferred row list is a complete turn from an earlier hook run, so
    # it is rebuilt in isolation and emitted before the current batch (its
    # rows are always chronologically older than anything in the batch).
    turns: List[Turn] = []
    for deferred_turn_rows in deferred_turn_row_lists:
        turns.extend(build_turns(deferred_turn_rows, task_id_to_tool_use_id))

    batch_turns, trailing_turn, trailing_turn_rows = assemble_turns(rows, task_id_to_tool_use_id)
    turns.extend(batch_turns)

    if flush_deferred_agent_turns:
        # SessionEnd: nothing can continue the trailing turn anymore.
        if trailing_turn is not None:
            turns.append(trailing_turn)
    else:
        session_state.open_turn = build_open_turn(trailing_turn, trailing_turn_rows)
        if trailing_turn_rows:
            debug(f"Holding trailing open turn ({len(trailing_turn_rows)} row(s)) until a new user row closes it")

    assign_turn_numbers(turns, trailing_turn, session_state)
    return turns, session_state


def resolve_agent_jsonl_and_id(meta_path: Path) -> Optional[Tuple[Path, str]]:
    """Derive an agent's transcript path and agent id from its meta.json path.

    Returns None when the sibling .jsonl is missing (metas without a
    transcript identify nothing worth emitting)."""
    jsonl_path = meta_path.with_name(meta_path.name[: -len(".meta.json")] + ".jsonl")
    if not jsonl_path.exists():
        return None
    agent_id = meta_path.name[: -len(".meta.json")]
    if agent_id.startswith("agent-"):
        agent_id = agent_id[len("agent-") :]
    return jsonl_path, agent_id


def get_subagent_transcripts_by_tool_use_id(transcript_path: Path) -> Dict[str, Dict[str, Any]]:
    """Map launching Agent/Task tool_use ids to their subagent transcripts."""
    subagent_dir = transcript_path.with_suffix("") / "subagents"
    if not subagent_dir.is_dir():
        return {}

    subagent_transcripts_by_tool_use_id: Dict[str, Dict[str, Any]] = {}
    for meta_path in subagent_dir.glob("*.meta.json"):
        try:
            metadata = json.loads(meta_path.read_text(encoding="utf-8"))
        except Exception:
            continue

        tool_use_id = metadata.get("toolUseId")
        if not isinstance(tool_use_id, str) or not tool_use_id:
            continue

        resolved = resolve_agent_jsonl_and_id(meta_path)
        if resolved is None:
            continue
        jsonl_path, agent_id = resolved

        subagent_transcripts_by_tool_use_id[tool_use_id] = {
            "path": jsonl_path,
            "agent_id": agent_id,
            "agent_type": metadata.get("agentType"),
            "description": metadata.get("description"),
        }
    return subagent_transcripts_by_tool_use_id


def get_workflow_journal_results(run_dir: Path) -> Dict[str, Any]:
    """Per-agent return values from a workflow run's journal.jsonl.

    The journal carries {"type":"result","agentId",...,"result":{...}} rows,
    one per completed agent; unparseable lines are skipped."""
    journal_path = run_dir / "journal.jsonl"
    try:
        lines = journal_path.read_text(encoding="utf-8").splitlines()
    except Exception:
        return {}
    results_by_agent_id: Dict[str, Any] = {}
    for line in lines:
        line = line.strip()
        if not line:
            continue
        try:
            journal_row = json.loads(line)
        except Exception:
            continue
        if not isinstance(journal_row, dict) or journal_row.get("type") != "result":
            continue
        agent_id = journal_row.get("agentId")
        if isinstance(agent_id, str) and agent_id:
            results_by_agent_id[agent_id] = journal_row.get("result")
    return results_by_agent_id


def get_workflow_agent_transcripts_by_run_id(transcript_path: Path) -> Dict[str, List[Dict[str, Any]]]:
    """Map Workflow run ids to the workflow-spawned agent transcripts on disk.

    Workflow-tool agents live under <stem>/subagents/workflows/wf_<runId>/;
    their meta.json carries agentType=="workflow-subagent" and — unlike
    classic subagents — no toolUseId, so they are keyed by the run id (the
    directory name) instead. The launching tool_use is linked via
    toolUseResult.runId on the parent transcript's tool_result row
    (see get_workflow_launch_marker_from_row).
    """
    workflows_dir = transcript_path.with_suffix("") / "subagents" / "workflows"
    if not workflows_dir.is_dir():
        return {}

    workflow_agents_by_run_id: Dict[str, List[Dict[str, Any]]] = {}
    for run_dir in sorted(workflows_dir.glob("wf_*")):
        if not run_dir.is_dir():
            continue
        journal_results = get_workflow_journal_results(run_dir)
        agents: List[Dict[str, Any]] = []
        for meta_path in sorted(run_dir.glob("agent-*.meta.json")):
            try:
                metadata = json.loads(meta_path.read_text(encoding="utf-8"))
            except Exception:
                continue
            if not isinstance(metadata, dict) or metadata.get("agentType") != "workflow-subagent":
                continue

            resolved = resolve_agent_jsonl_and_id(meta_path)
            if resolved is None:
                continue
            jsonl_path, agent_id = resolved

            agents.append(
                {
                    "path": jsonl_path,
                    "agent_id": agent_id,
                    "agent_type": metadata.get("agentType"),
                    "result": journal_results.get(agent_id),
                }
            )
        if agents:
            workflow_agents_by_run_id[run_dir.name] = agents
    return workflow_agents_by_run_id


def get_task_id_to_tool_use_id(
    subagent_transcripts_by_tool_use_id: Optional[Dict[str, Dict[str, Any]]],
) -> Dict[str, str]:
    task_id_to_tool_use_id: Dict[str, str] = {}
    if not subagent_transcripts_by_tool_use_id:
        return task_id_to_tool_use_id

    for tool_use_id, subagent in subagent_transcripts_by_tool_use_id.items():
        agent_id = subagent.get("agent_id")
        if isinstance(agent_id, str) and agent_id:
            task_id_to_tool_use_id[agent_id] = tool_use_id
    return task_id_to_tool_use_id


def read_subagent_jsonl(path: Path) -> Optional[List[Dict[str, Any]]]:
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except Exception as e:
        info(f"subagent transcript read failed ({path}): {type(e).__name__}: {e}")
        return None

    rows: List[Dict[str, Any]] = []
    for line_number, line in enumerate(lines, start=1):
        line = line.strip()
        if not line:
            continue
        try:
            row = json.loads(line)
        except Exception as e:
            info(f"subagent transcript line skipped ({path}:{line_number}): {type(e).__name__}: {e}")
            continue
        if not isinstance(row, dict):
            info(f"subagent transcript line skipped ({path}:{line_number}): expected JSON object")
            continue
        row["__catena_source_event_id"] = source_event_id(row, path, line)
        rows.append(row)
    return rows
