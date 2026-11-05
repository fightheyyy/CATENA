import json

import pytest

from catena_engine.codex_artifacts import read_native


def test_native_rollout_keeps_failed_command_and_final_answer(tmp_path):
    sessions = tmp_path / "sessions"
    sessions.mkdir()
    rows = [
        {
            "type": "response_item",
            "payload": {
                "type": "function_call",
                "call_id": "one",
                "arguments": json.dumps({"cmd": "pwsh -Command 'rg query'"}),
            },
        },
        {
            "type": "response_item",
            "payload": {
                "type": "function_call_output",
                "call_id": "one",
                "output": "Process exited with code 1\nrg not recognized",
            },
        },
        {
            "type": "response_item",
            "payload": {
                "type": "message",
                "role": "assistant",
                "phase": "final_answer",
                "content": [{"type": "output_text", "text": '{"matches":[]}'}],
            },
        },
    ]
    (sessions / "rollout.jsonl").write_text("\n".join(json.dumps(r) for r in rows))
    calls, answer = read_native(tmp_path)
    assert calls[0]["exit_code"] == 1
    assert answer == '{"matches":[]}'
    (sessions / "other.jsonl").write_text("")
    with pytest.raises(ValueError):
        read_native(tmp_path)


def test_native_custom_exec_keeps_code_as_evidence_and_unknown_exit(tmp_path):
    sessions = tmp_path / "sessions"
    sessions.mkdir()
    code = 'const r = await tools.exec_command({cmd:"pwd"}); text(r.output);'
    payloads = [
        {"type": "custom_tool_call", "call_id": "one", "name": "exec", "input": code},
        {"type": "custom_tool_call_output", "call_id": "one", "output": "/workspace/case"},
        {"type": "function_call", "call_id": "two", "arguments": '{"cmd":"test"}'},
        {"type": "function_call_output", "call_id": "two", "output": "Process exited with code -9"},
        {"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "done"}]},
    ]
    (sessions / "rollout.jsonl").write_text("\n".join(json.dumps({"type": "response_item", "payload": p}) for p in payloads))
    calls, answer = read_native(tmp_path)
    assert len(calls) == 2 and answer == "done"
    assert calls[0]["input"] == code and calls[0]["command"] == code
    assert calls[0]["exit_code"] is None and calls[1]["exit_code"] == -9
