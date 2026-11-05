import json

import pytest

pytest.importorskip("harbor")

from test_rsi import ATIF

from catena_engine.evaluation.evidence import read_deliverables, read_trajectory, render_trajectory
from catena_engine.rsi.contracts import OptimizationError


def test_atif_preserves_custom_tool_data_and_observation_truncation(tmp_path):
    data = json.loads(ATIF)
    code = 'text(await tools.exec_command({cmd:"pwd"}));'
    data["steps"] = [
        {
            "step_id": 1,
            "source": "agent",
            "message": "inspect",
            "tool_calls": [{"tool_call_id": "one", "function_name": "exec", "arguments": {"input": code}}],
            "observation": {"results": [{"source_call_id": "one", "content": "abcde"}]},
        },
        {"step_id": 2, "source": "agent", "message": "final reply is not a file"},
    ]
    path = tmp_path / "trajectory.json"
    path.write_text(json.dumps(data))
    body, answer, count = read_trajectory(path)
    assert count == 1 and answer == "final reply is not a file"
    rendered = render_trajectory(body, 3)
    assert "[ACTION] exec" in rendered and json.dumps(code) in rendered
    assert "[OBSERVATION] abc\n[truncated]" in rendered


def test_declared_delivery_uses_collected_file_not_final_reply(tmp_path):
    root = tmp_path / "artifacts"
    root.mkdir()
    (root / "result.json").write_text('{"actual":true}')
    entry = {"source": "/workspace/result.json", "destination": "artifacts/result.json", "type": "file", "status": "ok"}
    manifest = root / "manifest.json"
    manifest.write_text(json.dumps([entry]))
    assert read_deliverables(root, (entry["source"],)) == {entry["source"]: '{"actual":true}'}
    manifest.write_text(json.dumps([{**entry, "status": "failed"}]))
    assert read_deliverables(root, (entry["source"],)) == {}
    manifest.write_text(json.dumps([{**entry, "destination": "../secret"}]))
    with pytest.raises(OptimizationError, match="confined"):
        read_deliverables(root, (entry["source"],))
    manifest.write_text(json.dumps([entry]))
    (root / "result.json").unlink()
    (root / "result.json").symlink_to(tmp_path / "secret")
    with pytest.raises(OptimizationError, match="confined"):
        read_deliverables(root, (entry["source"],))
