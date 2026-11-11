import asyncio
import json
import threading
from dataclasses import replace

import pytest

from catena_engine.contracts import SCHEMA, Budget, Candidate, Context, OptimizationError, Trial
from catena_engine.improvement import Experiment, candidate, parse

ATIF = json.dumps(
    {
        "schema_version": "ATIF-v1.6",
        "agent": {"name": "fixture", "version": "1"},
        "steps": [{"step_id": 1, "source": "agent", "message": "inspected amounts"}],
    }
)


class Evaluator:
    def __init__(self, *args):
        self.seen = []

    def preflight(self):
        pass

    def task_prompt(self, case_id):
        return "Inspect the signed amounts."

    def probe(self, case_id, harness, timeout, cancelled=None):
        self.seen.append((case_id, harness.id))
        return Trial(
            str(len(self.seen)),
            case_id,
            harness.id,
            ATIF,
            "final reply",
            True,
            {"HIDDEN_VERIFIER_SENTINEL": 1},
            20,
            3,
            deliverables={"/workspace/result.json": "2"},
        )


class Runtime:
    def __init__(self, *args):
        self.prompts = []

    def preflight(self):
        pass

    def run(self, prompt, timeout, evaluate_candidate, output, cancelled):
        self.prompts.append(prompt)
        value = {
            "text": "Hypothesis: missing tool. Evidence: trace-one.",
            "files": [{"path": "AGENTS.md", "content": "Check tool availability; use portable search."}],
        }
        evidence = asyncio.run(evaluate_candidate(value))
        assert "HIDDEN_VERIFIER_SENTINEL" not in json.dumps(evidence)
        assert "heldout" not in prompt
        return {"value": value, "input_tokens": 10, "output_tokens": 2}


def context():
    return Context(
        "request",
        "training",
        Candidate({"AGENTS.md": "Follow the task."}),
        "Observed missing search tool.",
        ("trace-one",),
    )


def payload():
    return {
        "schema": SCHEMA,
        "request_id": "request",
        "case_id": "training",
        "baseline": {"AGENTS.md": "Follow the task."},
        "model": {
            "provider": "openai",
            "base_url": "https://example.test/v1",
            "model": "fixture",
            "api_key": "DO_NOT_PERSIST",
        },
    }


def test_workflow_freezes_candidate_then_compares_and_keeps_grades_out(tmp_path):
    ctx, runtime, evaluator = context(), Runtime(), Evaluator()
    experiment = Experiment(ctx, runtime, evaluator, Budget(trials=6), 4, tmp_path)
    result = experiment.run(["heldout"])
    assert [t["phase"] for t in result["trials"]] == ["probe"] * 2 + ["validation"] * 4
    assert [t[0] for t in evaluator.seen] == ["training"] * 4 + ["heldout"] * 2
    frozen = json.loads((tmp_path / "candidate.json").read_text())
    assert frozen == result["candidates"][0]
    assert all(pair["candidate"]["candidate_id"] == frozen["id"] for pair in result["compare"])
    assert frozen["parents"] == [ctx.baseline.id]
    assert result["usage"]["agent_turns"] == 1 and result["usage"]["attempted_trials"] == 6
    assert "HIDDEN_VERIFIER_SENTINEL" not in runtime.prompts[0]
    assert "trajectory" not in result["trials"][0] and result["trials"][0]["deliverable_sha256"]
    assert result["decision"] == "review_required"


@pytest.mark.parametrize(
    "path", ["../secret", "/AGENTS.md", "scripts/a.py", ".codex/hooks.json", ".agents/skills/../../x/SKILL.md"]
)
def test_candidate_cannot_expand_edit_surface(path):
    with pytest.raises(OptimizationError):
        Candidate({"AGENTS.md": "Task", path: "malicious"})


def test_duplicate_files_and_mutable_candidate_rejected():
    ctx = context()
    with pytest.raises(OptimizationError):
        candidate({"text": "x", "files": [{"path": "AGENTS.md", "content": "x"}] * 2}, ctx.baseline)
    with pytest.raises(TypeError):
        ctx.baseline.files["AGENTS.md"] = "changed"


def test_internal_analysis_skill_cannot_cross_into_target_harness(tmp_path):
    from catena_engine.runtime.codex import SKILL

    experiment = Experiment(context(), Runtime(), Evaluator(), Budget(), 2, tmp_path)
    for path, content in [
        (".agents/skills/harness-improvement/SKILL.md", "host analysis instructions"),
        (".agents/skills/copied-analysis/SKILL.md", SKILL.read_text()),
    ]:
        value = {
            "text": "proposal",
            "files": [{"path": "AGENTS.md", "content": "Inspect."}, {"path": path, "content": content}],
        }
        with pytest.raises(OptimizationError, match="internal analysis Skill"):
            asyncio.run(experiment.evaluate_candidate(value))
    assert experiment.attempted_trials == 0


def test_explicit_empty_composition_keeps_only_the_successfully_probed_harness(tmp_path):
    class KeepProbed(Runtime):
        def run(self, *args):
            reply = super().run(*args)
            reply["value"] = {"text": "Keep the probed proposal; no additional edits.", "files": []}
            return reply

    experiment = Experiment(context(), KeepProbed(), Evaluator(), Budget(), 2, tmp_path)
    result = experiment.run([])
    assert result["candidates"][0]["files"]["AGENTS.md"] == "Check tool availability; use portable search."
    assert result["candidates"][0]["id"] == experiment.trials[1].candidate_id
    assert result["compare"][0]["candidate"]["id"] != experiment.trials[1].id


@pytest.mark.parametrize(
    "patch",
    [
        {"schema": "catena.rsi_request.v2"},
        {"harness2": {}},
        {"algorithms": []},
        {"composition": "parallel"},
        {"case_id": "../outside"},
        {"budget": {"model_calls": 1}},
        {"budget": {"agent_turns": True}},
        {"budget": {"agent_turns": 2}},
        {"validation_case_ids": ["training"]},
        {"validation_case_ids": ["heldout", "heldout"]},
    ],
)
def test_request_rejects_retired_options_and_unsafe_limits(patch):
    with pytest.raises(OptimizationError):
        parse({**payload(), **patch})


def test_probe_limit_and_failure_cannot_publish_candidate(tmp_path):
    experiment = Experiment(context(), Runtime(), Evaluator(), Budget(), 2, tmp_path)
    value = {"text": "proposal", "files": [{"path": "AGENTS.md", "content": "Inspect first."}]}
    asyncio.run(experiment.evaluate_candidate(value))
    with pytest.raises(OptimizationError, match="exactly one"):
        asyncio.run(experiment.evaluate_candidate(value))
    assert experiment.attempted_trials == 1

    class Bad(Evaluator):
        def probe(self, *args):
            return replace(super().probe(*args), valid=False)

    failed = Experiment(context(), Runtime(), Bad(), Budget(), 2, tmp_path)
    with pytest.raises(OptimizationError):
        failed.run([])
    assert not (tmp_path / "candidate.json").exists()


def test_cancellation_waits_for_harbor_cleanup(tmp_path):
    entered, cleaned = threading.Event(), threading.Event()

    class Slow(Evaluator):
        def probe(self, case, harness, timeout, cancelled):
            entered.set()
            assert cancelled.wait(3)
            cleaned.set()
            raise OptimizationError("cancelled")

    experiment = Experiment(context(), Runtime(), Slow(), Budget(), 2, tmp_path)

    async def run():
        work = asyncio.create_task(
            experiment.evaluate_candidate({"text": "x", "files": [{"path": "AGENTS.md", "content": "Inspect."}]})
        )
        await asyncio.to_thread(entered.wait, 2)
        work.cancel()
        with pytest.raises(asyncio.CancelledError):
            await work

    asyncio.run(run())
    assert cleaned.is_set()
    assert not experiment.cancelled.is_set()


def test_private_api_runs_new_workflow_and_persists_only_safe_report(monkeypatch, tmp_path):
    from test_server import call

    from catena_engine import improvement
    from catena_engine.evaluation.harbor import task_digest
    from catena_engine.server import create_server

    monkeypatch.setattr(improvement, "WorkflowHarness", Runtime)
    monkeypatch.setattr(improvement, "HarborEvaluator", Evaluator)
    task = tmp_path / "task"
    task.mkdir()
    (task / "task.toml").write_text('schema_version = "1.4"\nartifacts = ["/workspace/result.json"]\n')
    (task / "instruction.md").write_text("Inspect the amounts.")
    manifest = tmp_path / "cases.json"
    manifest.write_text(
        json.dumps({"training": {"path": str(task), "sha256": task_digest(task), "confirmed_by": "protocol-fixture"}})
    )
    monkeypatch.setenv("CATENA_CASES_MANIFEST", str(manifest))
    monkeypatch.setenv("CATENA_RUNS_ROOT", str(tmp_path / "runs"))
    server = create_server("127.0.0.1:0", token="private", receipt_root=tmp_path / "receipts")
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        base = f"http://127.0.0.1:{server.server_port}"
        status, receipt = call(base + "/v3/improve", payload(), "private")
        assert status == 200 and receipt["state"] == "completed"
        result = receipt["output"]
        assert result["status"] == "completed" and result["usage"]["attempted_trials"] == 4
        report = tmp_path / "runs" / result["experiment_id"] / "result.json"
        assert json.loads(report.read_text()) == result and report.stat().st_mode & 0o777 == 0o600
        assert "DO_NOT_PERSIST" not in report.read_text()
        assert call(base + "/v2/rsi", payload(), "private")[0] == 404
        assert call(base + "/v2/rsi/adapters", token="private")[0] == 404
    finally:
        server.shutdown()
        server.server_close()
