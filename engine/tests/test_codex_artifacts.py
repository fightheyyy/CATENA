import json
from pathlib import Path

import pytest

from catena_engine.codex_artifacts import read_native
from catena_engine.harbor_worker import build_tasks


def test_native_rollout_keeps_failed_command_and_final_answer(tmp_path):
    sessions = tmp_path / "sessions"
    sessions.mkdir()
    rows = [
        {"type": "response_item", "payload": {"type": "function_call", "call_id": "one", "arguments": json.dumps({"cmd": "pwsh -Command 'rg query'"})}},
        {"type": "response_item", "payload": {"type": "function_call_output", "call_id": "one", "output": "Process exited with code 1\nrg not recognized"}},
        {"type": "response_item", "payload": {"type": "message", "role": "assistant", "phase": "final_answer", "content": [{"type": "output_text", "text": '{"matches":[]}'}]}},
    ]
    (sessions / "rollout.jsonl").write_text("\n".join(json.dumps(r) for r in rows))
    calls, answer = read_native(tmp_path)
    assert calls[0]["exit_code"] == 1
    assert answer == '{"matches":[]}'
    (sessions / "other.jsonl").write_text("")
    with pytest.raises(ValueError):
        read_native(tmp_path)


def test_native_task_has_independent_verifier_and_non_root_agent(tmp_path):
    suite = Path(__file__).resolve().parents[2] / "evals/rg-missing-search"
    task = build_tasks(suite, tmp_path / "tasks", "codex-image", native=True)[0]
    assert 'user = "catena-eval"' in (task / "task.toml").read_text()
    assert not (task / "environment/expected.json").exists()
    assert "codex.txt" in (task / "tests/extract.py").read_text()
    assert "pwsh -NoProfile" in (task / "instruction.md").read_text(encoding="utf-8")
