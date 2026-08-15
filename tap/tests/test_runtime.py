from __future__ import annotations

import copy
import json
import shutil
from pathlib import Path

import catena_tap.claude_parser as parser_state
import catena_tap.runtime as runtime
from catena_tap.claude_graph import parse_claude_increment, parse_claude_transcript
from catena_tap.claude_parser import SessionState
from catena_tap.exporter import trace_to_otlp

ROOT = Path(__file__).resolve().parents[1]
FIXTURE_DIR = ROOT / "fixtures" / "claude"
SESSION_ID = "11111111-1111-4111-8111-111111111111"


class RecordingClient:
    def __init__(self, succeeds: bool = True):
        self.succeeds = succeeds
        self.payloads = []

    def send(self, payload):
        self.payloads.append(copy.deepcopy(payload))
        return self.succeeds


def configure_state(monkeypatch, state_dir: Path) -> None:
    monkeypatch.setattr(parser_state, "STATE_DIR", state_dir)
    monkeypatch.setattr(parser_state, "STATE_FILE", state_dir / "state.json")
    monkeypatch.setattr(parser_state, "LOCK_FILE", state_dir / "state.lock")
    monkeypatch.setattr(runtime, "LOCK_FILE", state_dir / "state.lock")


def spans_by_identity(payloads):
    result = {}
    for payload in payloads:
        for resource in payload["resourceSpans"]:
            for scope in resource["scopeSpans"]:
                for span in scope["spans"]:
                    result[(span["traceId"], span["spanId"])] = span
    return result


def test_session_end_parser_is_identical_to_historical_import():
    historical = parse_claude_transcript(FIXTURE_DIR / "transcript.jsonl")
    live, _ = parse_claude_increment(
        FIXTURE_DIR / "transcript.jsonl",
        SessionState(),
        expected_session_id=SESSION_ID,
        flush=True,
    )
    assert live == historical


def test_stop_is_incremental_duplicate_safe_and_converges_to_historical_import(tmp_path, monkeypatch):
    fixture = tmp_path / "claude"
    shutil.copytree(FIXTURE_DIR, fixture)
    transcript = fixture / "transcript.jsonl"
    configure_state(monkeypatch, tmp_path / "state")
    client = RecordingClient()
    monkeypatch.setattr(runtime, "_client_from_environment", lambda _environment: client)
    hook = {"session_id": SESSION_ID, "transcript_path": str(transcript), "hook_event_name": "Stop"}

    first = runtime.run_claude_hook(hook, {})
    duplicate = runtime.run_claude_hook(hook, {})
    final = runtime.run_claude_hook({**hook, "hook_event_name": "SessionEnd"}, {})

    assert first == {"parsed": 14, "uploaded": 14, "skipped": 0, "failed": 0}
    assert duplicate == {"parsed": 1, "uploaded": 0, "skipped": 1, "failed": 0}
    assert final == {"parsed": 1, "uploaded": 1, "skipped": 0, "failed": 0}

    historical = parse_claude_transcript(transcript)
    expected_payloads = [trace_to_otlp(historical, trace) for trace in historical["traces"]]
    assert spans_by_identity(client.payloads) == spans_by_identity(expected_payloads)


def test_failed_upload_does_not_consume_transcript(tmp_path, monkeypatch):
    fixture = tmp_path / "claude"
    shutil.copytree(FIXTURE_DIR, fixture)
    configure_state(monkeypatch, tmp_path / "state")
    client = RecordingClient(succeeds=False)
    monkeypatch.setattr(runtime, "_client_from_environment", lambda _environment: client)
    stats = runtime.run_claude_hook(
        {
            "session_id": SESSION_ID,
            "transcript_path": str(fixture / "transcript.jsonl"),
            "hook_event_name": "Stop",
        },
        {},
    )
    assert stats["failed"] == 14
    assert not (tmp_path / "state" / "state.json").exists()


def test_otlp_ids_are_stable_and_errors_are_not_success():
    graph = parse_claude_transcript(FIXTURE_DIR / "transcript.jsonl")
    all_payloads = [trace_to_otlp(graph, trace) for trace in graph["traces"]]
    golden = json.loads((ROOT / "fixtures" / "golden" / "claude.otlp.json").read_text())
    assert all_payloads == golden
    failed = next(trace for trace in graph["traces"] if trace["state"] == "aborted")
    first = trace_to_otlp(graph, failed)
    second = trace_to_otlp(graph, failed)
    assert first == second
    spans = first["resourceSpans"][0]["scopeSpans"][0]["spans"]
    assert spans[0]["status"]["code"] == 2
