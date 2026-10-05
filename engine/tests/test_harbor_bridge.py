import asyncio
import json

from catena_engine.harbor_bridge import harbor_request


def test_bridge_refuses_cloud_and_requires_owner_model(monkeypatch):
    monkeypatch.setenv("CATENA_HARBOR_WORKER_URL", "http://worker")
    monkeypatch.setenv("CATENA_HARBOR_WORKER_TOKEN", "private")
    for payload in ({"case_id": "rg-missing-search", "environment": "e2b"},
                    {"case_id": "rg-missing-search", "environment": "docker"}):
        status, body = asyncio.run(harbor_request("/v1/harbor/submit", payload))
        assert status == 400
        assert "private" not in json.dumps(body)


def test_worker_reloads_request_identity_and_marks_interruption(tmp_path):
    from catena_engine.harbor_worker import ExperimentWorker

    root = tmp_path / "job"
    root.mkdir()
    (root / "catena-result.json").write_text(json.dumps({"job_id": "job", "request_id": "request", "status": "running", "trials": []}))
    worker = ExperimentWorker(tmp_path, tmp_path, "image")
    assert worker.lookup("request")["status"] == "failed"
    assert worker.submit("rg-missing-search", "request")["job_id"] == "job"


def test_worker_owner_credentials_are_not_serialized(tmp_path, monkeypatch):
    from catena_engine.harbor_agent import JOB_MODELS
    from catena_engine.harbor_worker import ExperimentWorker

    class DeferredThread:
        def __init__(self, **kwargs):
            pass

        def start(self):
            pass

    monkeypatch.setattr("catena_engine.harbor_worker.threading.Thread", DeferredThread)
    monkeypatch.setenv("CATENA_EVAL_HOST_ALIAS", "127.0.0.1")
    worker = ExperimentWorker(tmp_path, tmp_path / "runs", "image")
    state = worker.submit("rg-missing-search", "owned-request", model={
        "model": "owner-model", "base_url": "http://host.docker.internal:8317/v1", "api_key": "private-owner-key"})
    try:
        worker.save(state)
        assert "private-owner-key" not in json.dumps(worker.lookup("owned-request"))
        assert "private-owner-key" not in (tmp_path / "runs" / state["job_id"] / "catena-result.json").read_text()
        assert JOB_MODELS[state["job_id"]]["base_url"] == "http://127.0.0.1:8317/v1"
        assert state["model"] == "owner-model"
    finally:
        JOB_MODELS.pop(state["job_id"], None)


def test_three_arm_completion_rejects_missing_or_duplicate_trials(tmp_path):
    from catena_engine.harbor_worker import reconcile
    root = tmp_path
    rows = []
    for arm in ("baseline", "memory", "candidate"):
        for attempt in (1, 2):
            for task in ("command-handler", "role-config", "absent-symbol"):
                name = f"{task}-{attempt}"
                directory = f"{arm}-{attempt}"
                log = root / "jobs" / directory / name / "agent/tool-calls.json"
                log.parent.mkdir(parents=True)
                log.write_text(json.dumps([{"command": "Get-Content src/file", "stdout": "ok", "stderr": "", "exit_code": 0}]))
                rows.append({"variant": arm, "task": task, "attempt": attempt, "job_directory": directory,
                             "trial_name": name, "rewards": {"reward": 0}})
    state = {"status": "completed", "arms": ["baseline", "memory", "candidate"], "attempts": 2,
             "tasks_unchanged": True, "trials": rows}
    assert reconcile(root, state)["status"] == "completed"  # Valid failures are still results.
    state["status"] = "completed"
    state["trials"] = rows[:-1] + [rows[0]]
    assert reconcile(root, state)["status"] == "invalid"


def test_evidence_is_read_only_bounded_and_rejects_path_escape(tmp_path):
    import pytest
    from catena_engine.harbor_worker import ExperimentWorker
    worker = ExperimentWorker(tmp_path, tmp_path / "runs", "image")
    worker.jobs["request"] = {"job_id": "job", "status": "completed", "trials": [
        {"task": "fixture", "variant": "memory", "job_directory": "memory-1", "trial_name": "trial"}]}
    root = tmp_path / "runs/job/jobs/memory-1/trial/agent"
    root.mkdir(parents=True)
    (root / "tool-calls.json").write_text(json.dumps([{"command": "Get-Content file", "stdout": "x" * 9000, "stderr": "", "exit_code": 0}]))
    (root / "agent.final.txt").write_text('{"matches":[]}')
    result = worker.evidence("job", 0)
    assert len(result["calls"][0]["stdout"]) == 3500
    assert result["answer"] == '{"matches":[]}'
    for index in (-1, 1, True):
        with pytest.raises(ValueError):
            worker.evidence("job", index)
    worker.jobs["request"]["trials"][0]["job_directory"] = "../../../../outside"
    with pytest.raises(ValueError, match="artifact path"):
        worker.evidence("job", 0)
