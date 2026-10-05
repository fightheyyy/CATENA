import json
import sys
import time
from pathlib import Path
from unittest.mock import patch

import pytest

from catena_tap.skill_experiment import SCHEMA, _run, _trace_metrics, load_manifest, run_experiment


def _manifest(tmp_path: Path) -> Path:
    fixture = tmp_path / "fixture"
    fixture.mkdir()
    (fixture / "input.txt").write_text("task", encoding="utf-8")
    skill = tmp_path / "skill"
    skill.mkdir()
    (skill / "SKILL.md").write_text("---\nname: fix-check\ndescription: Check a task\n---\nDo it.", encoding="utf-8")
    path = tmp_path / "manifest.json"
    path.write_text(json.dumps({
        "schema_version": SCHEMA,
        "model": "test-model",
        "skill_name": "fix-check",
        "skill": "skill",
        "repeats": 2,
        "cases": [{"id": "case-1", "prompt": "Fix the task", "fixture": "fixture", "verify": ["verify", "input.txt"]}],
    }), encoding="utf-8")
    return path


def test_paired_runs_copy_skill_only_to_candidate(tmp_path: Path) -> None:
    manifest = _manifest(tmp_path)
    seen = []

    def fake_run(argv, cwd, timeout, *, env=None):
        if argv[1:] == ["--version"]:
            return {"exit_code": 0, "stdout": "codex test\n", "stderr": "", "timed_out": False}
        has_skill = (cwd / ".codex" / "skills" / "fix-check" / "SKILL.md").exists()
        if argv[1] == "exec":
            assert env and Path(env["CODEX_HOME"]).is_dir()
            seen.append((argv[-1], has_skill))
            return {"exit_code": 0, "stdout": '{"type":"turn.completed"}\n', "stderr": "", "timed_out": False}
        assert argv == ["verify", "input.txt"]
        return {"exit_code": 0 if has_skill else 1, "stdout": "", "stderr": "", "timed_out": False}

    with patch("catena_tap.skill_experiment._run", side_effect=fake_run):
        report = run_experiment(manifest, tmp_path / "output", "fake-codex")
    assert seen == [("Fix the task", False), ("Fix the task", True), ("Fix the task", True), ("Fix the task", False)]
    assert report["baseline"]["passed"] == 0 and report["baseline"]["total"] == 2
    assert report["candidate"]["passed"] == 2 and report["candidate"]["total"] == 2
    assert report["baseline"]["missing_rg_errors"] == 0
    assert (tmp_path / "output" / "case-1" / "repeat-1" / "candidate" / "trace.jsonl").exists()
    assert not (tmp_path / "fixture" / ".codex").exists()


def test_manifest_rejects_escape_and_preexisting_skill(tmp_path: Path) -> None:
    manifest = _manifest(tmp_path)
    data = json.loads(manifest.read_text(encoding="utf-8"))
    data["cases"][0]["fixture"] = "../elsewhere"
    manifest.write_text(json.dumps(data), encoding="utf-8")
    with pytest.raises(ValueError, match="escapes"):
        load_manifest(manifest)
    data["cases"][0]["fixture"] = "fixture"
    manifest.write_text(json.dumps(data), encoding="utf-8")
    (tmp_path / "fixture" / ".codex" / "skills").mkdir(parents=True)
    with pytest.raises(ValueError, match="already contains skills"):
        load_manifest(manifest)


def test_output_cannot_be_nested_inside_fixture(tmp_path: Path) -> None:
    manifest = _manifest(tmp_path)
    with pytest.raises(ValueError, match="output directory"):
        run_experiment(manifest, tmp_path / "fixture" / "results", "fake-codex")


def test_trace_metrics_require_completed_turn() -> None:
    metrics = _trace_metrics('{"type":"item.completed","item":{"type":"command_execution"}}\n{"type":"turn.completed","usage":{"input_tokens":12}}')
    assert metrics == {"turn_completed": True, "command_count": 1, "usage": {"input_tokens": 12}, "malformed_lines": 0, "final_message": "",
                       "failed_commands": 0, "missing_rg_errors": 0, "skill_read_commands": 0,
                       "policy_rejections": 0, "successful_rg_commands": 0}
    assert _trace_metrics("not json")["malformed_lines"] == 1


def test_missing_command_differs_from_search_with_no_matches() -> None:
    events = [
        {"type": "item.completed", "item": {"type": "command_execution", "exit_code": 1,
         "aggregated_output": "rg : The term 'rg' is not recognized. CommandNotFoundException"}},
        {"type": "item.completed", "item": {"type": "command_execution", "exit_code": 1, "aggregated_output": ""}},
    ]
    metrics = _trace_metrics("\n".join(json.dumps(event) for event in events))
    assert metrics["failed_commands"] == 2
    assert metrics["missing_rg_errors"] == 1


def test_rg_success_and_policy_rejection_are_environment_evidence() -> None:
    events = [
        {"type": "item.completed", "item": {"type": "command_execution", "command": "rg --files",
         "exit_code": 0, "aggregated_output": "src/code.ts"}},
        {"type": "item.completed", "item": {"type": "command_execution", "command": "Get-Command rg",
         "exit_code": -1, "status": "declined", "aggregated_output": "rejected: blocked by policy"}},
    ]
    metrics = _trace_metrics("\n".join(json.dumps(event) for event in events))
    assert metrics["successful_rg_commands"] == 1
    assert metrics["policy_rejections"] == 1


def test_verifier_change_invalidates_trial(tmp_path: Path) -> None:
    manifest = _manifest(tmp_path)
    verifier_file = tmp_path / "verify.py"
    verifier_file.write_text("original", encoding="utf-8")
    data = json.loads(manifest.read_text())
    data["verifier_files"] = ["verify.py"]
    manifest.write_text(json.dumps(data))

    def fake_run(argv, cwd, timeout, *, env=None):
        if argv[1:] == ["--version"]:
            return {"exit_code": 0, "stdout": "test", "stderr": "", "timed_out": False}
        assert argv[1] == "exec"  # Changed verifier must never execute.
        verifier_file.write_text("changed", encoding="utf-8")
        return {"exit_code": 0, "stdout": '{"type":"turn.completed"}\n', "stderr": "", "timed_out": False}

    with patch("catena_tap.skill_experiment._run", side_effect=fake_run):
        report = run_experiment(manifest, tmp_path / "output", "fake-codex")
    assert all(not row["passed"] and not row["verifier_unchanged"] for row in report["runs"])


def test_timeout_stops_child_process_holding_stdout(tmp_path: Path) -> None:
    code = "import subprocess,sys,time; subprocess.Popen([sys.executable,'-c','import time; time.sleep(30)']); print('started',flush=True); time.sleep(30)"
    started = time.monotonic()
    result = _run([sys.executable, "-c", code], tmp_path, 1)
    assert result["timed_out"] and result["exit_code"] is None
    assert "started" in result["stdout"]
    assert time.monotonic() - started < 15
