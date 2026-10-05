import json

import httpx
import pytest

harbor_worker = pytest.importorskip("catena_engine.harbor_worker", exc_type=ImportError)
ExperimentWorker = harbor_worker.ExperimentWorker
build_tasks = harbor_worker.build_tasks


def test_harbor_merged_streams_do_not_hide_tool_errors():
    calls = [{"command": "rg query", "exit_code": 1, "stdout": "The term 'rg' is not recognized", "stderr": ""},
             {"command": "Get-Content ../file", "exit_code": 1, "stdout": "EVAL_POLICY: rejected", "stderr": ""},
             {"command": "Select-String absent", "exit_code": 0, "stdout": "", "stderr": ""}]
    assert harbor_worker.call_metrics(calls) == {"command_count": 3, "missing_rg_errors": 1, "policy_rejections": 1}


def test_task_conversion_preserves_fixtures_and_labels_environment(tmp_path):
    from pathlib import Path

    suite = Path(__file__).resolve().parents[2] / "evals/rg-missing-search"
    tasks = build_tasks(suite, tmp_path / "tasks", "example@sha256:abc")
    manifest = json.loads((suite / "manifest.json").read_text(encoding="utf-8"))
    assert len(tasks) == 3
    for task, case in zip(tasks, manifest["cases"], strict=True):
        for original in (suite / case["fixture"]).rglob("*"):
            if original.is_file():
                assert (task / "environment/repo" / original.relative_to(suite / case["fixture"])).read_bytes() == original.read_bytes()
        assert "Linux 上的 PowerShell 7" in (task / "instruction.md").read_text(encoding="utf-8")
        assert not (task / "environment/expected.json").exists()


def test_worker_rejects_unknown_suite(tmp_path):
    worker = ExperimentWorker(tmp_path, tmp_path, "image")
    with pytest.raises(ValueError):
        worker.submit("../../arbitrary", "request")


def test_e2b_without_credentials_does_not_start_job(tmp_path, monkeypatch):
    monkeypatch.delenv("E2B_API_KEY", raising=False)
    worker = ExperimentWorker(tmp_path, tmp_path / "output", "image")
    state = worker.submit("rg-missing-search", "cloud-request", "e2b")
    assert state["status"] == "blocked"
    assert state["reason_code"] == "e2b_credentials_missing"
    assert state["provider"] == "e2b"
    assert state["trials"] == []
    assert worker.submit("rg-missing-search", "cloud-request", "e2b")["job_id"] == state["job_id"]
    with pytest.raises(ValueError, match="another environment"):
        worker.submit("rg-missing-search", "cloud-request", "docker")


def test_unknown_environment_is_rejected(tmp_path):
    worker = ExperimentWorker(tmp_path, tmp_path, "image")
    with pytest.raises(ValueError, match="unsupported execution"):
        worker.submit("rg-missing-search", "request", "arbitrary-host")


def test_unknown_resume_never_launches_a_new_experiment(tmp_path):
    worker = ExperimentWorker(tmp_path, tmp_path, "image")
    with pytest.raises(ValueError, match="no new experiment"):
        worker.submit("rg-missing-search", "resume-unknown", "e2b")
    assert worker.jobs == {}


def test_worker_idempotency(tmp_path):
    worker = ExperimentWorker(tmp_path, tmp_path, "image")
    worker.jobs["request"] = {"job_id": "one", "status": "running"}
    assert worker.submit("rg-missing-search", "request")["job_id"] == "one"
    with pytest.raises(ValueError, match="another experiment"):
        worker.submit("rg-missing-search", "two")


def test_endpoint_is_authenticated():
    import threading

    from catena_engine.server import create_server

    server = create_server("127.0.0.1:0", token="test-secret")
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        response = httpx.post(f"http://127.0.0.1:{server.server_port}/v1/experiment", json={})
        assert response.status_code == 401
    finally:
        server.shutdown()
        server.server_close()


def test_model_cannot_claim_completion_without_harbor_result(monkeypatch, turn_payload):
    import asyncio

    from conftest import FakeModel, factory_for

    from catena_engine.experiment_runtime import run_experiment_turn
    from catena_engine.protocol import parse_turn_request

    monkeypatch.setenv("CATENA_HARBOR_WORKER_URL", "http://127.0.0.1:1")
    monkeypatch.setenv("CATENA_HARBOR_WORKER_TOKEN", "worker-secret")
    factory = factory_for(FakeModel('{"status":"completed","reward":1}'))
    result = asyncio.run(run_experiment_turn(parse_turn_request(turn_payload), factory))
    assert result["status"] == "failed"
    assert result["experiment"] is None
    assert result["tool_events"] == []
    assert factory.closed == [True]


def test_experiment_error_redacts_both_credentials(monkeypatch, turn_payload):
    import asyncio

    from conftest import FakeModel, factory_for

    from catena_engine.experiment_runtime import run_experiment_turn
    from catena_engine.protocol import parse_turn_request

    monkeypatch.setenv("CATENA_HARBOR_WORKER_URL", "http://127.0.0.1:1")
    monkeypatch.setenv("CATENA_HARBOR_WORKER_TOKEN", "worker-secret")
    factory = factory_for(FakeModel(error=RuntimeError("worker-secret sk-test-secret-value")))
    result = asyncio.run(run_experiment_turn(parse_turn_request(turn_payload), factory))
    assert "worker-secret" not in result["detail"]
    assert "sk-test-secret-value" not in result["detail"]
    assert factory.closed == [True]
