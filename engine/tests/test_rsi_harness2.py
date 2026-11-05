import json

import pytest

pytest.importorskip("harness2")

from test_rsi import Evaluator, Runtime, context

from catena_engine.rsi.contracts import Budget, Completion, OptimizationError
from catena_engine.rsi.host import Services, optimize


class Editor:
    model = Runtime.model

    def __init__(self):
        self.prompts = []

    def complete(self, prompt, timeout, cancelled=None):
        self.prompts.append(prompt)
        data = json.loads(prompt)
        assert "HIDDEN_VERIFIER_SENTINEL" not in prompt
        if data["operation"] == "edit":
            value = {
                "text": "<notes>Use portable search after the observed missing tool.</notes>",
                "files": [{"path": "AGENTS.md", "content": "Check available tools; use portable search."}],
            }
        else:
            value = {
                "text": "```choice\nA\n```\n<experience>\ncue: missing tool\nlesson: inspect availability\n"
                "evidence: probe tool observations\nconfidence: low\n</experience>",
                "files": [],
            }
        return Completion(value, 100, 20)


@pytest.mark.parametrize("mode,k", [("parallel", 1), ("sequential", 1), ("parallel", 2), ("sequential", 2)])
def test_upstream_loop_uses_host_and_keeps_grades_out(mode, k, tmp_path):
    ctx, editor, evaluator = context(), Editor(), Evaluator()
    services = Services(ctx, editor, evaluator, Budget(20, 16, 10), output=tmp_path)
    result = optimize(ctx, {"mode": mode, "k": k}, services)
    candidate = result["candidates"][0]
    assert candidate["files"]["AGENTS.md"] == "Check available tools; use portable search."
    ledger = result["harness2"]["diagnostics"]["ledger"]
    assert (ledger["R"], ledger["P"]) == (4 * k, 3 * k)
    assert len(evaluator.seen) == 4 * k
    assert services.calls == 3 * k + 1 + (1 if k > 1 else 0)
    assert candidate["parents"] == [ctx.baseline.id]
    assert candidate["id"] in [t["candidate_id"] for t in result["trials"]]
    assert result["decision"] == "evidence_insufficient"
    assert "Observed repeated search." in editor.prompts[0]


def test_failure_cannot_publish_a_fallback_candidate(tmp_path):
    services = Services(context(), Editor(), Evaluator(), Budget(12, 3, 10), output=tmp_path)
    with pytest.raises(OptimizationError):
        optimize(context(), {}, services)
    assert services.attempted_trials == 0


def test_upstream_cannot_emit_scripts_or_hooks(tmp_path):
    class Unsafe(Editor):
        def complete(self, prompt, timeout, cancelled=None):
            return Completion({"text": "edit", "files": [{"path": ".codex/hooks.json", "content": "{}"}]}, 1, 1)

    services = Services(context(), Unsafe(), Evaluator(), Budget(), output=tmp_path)
    with pytest.raises(OptimizationError):
        optimize(context(), {}, services)
    assert services.attempted_trials == 1


def test_retained_upstream_runs_replay_without_repaying_and_keep_real_files(monkeypatch, tmp_path):
    import hashlib

    from harness2.core import schedule

    original, captured = schedule.run_task, {}

    def capture(*args, **kwargs):
        captured["bench"] = kwargs["adapter"]
        return original(*args, **kwargs)

    monkeypatch.setattr(schedule, "run_task", capture)
    ctx = context()
    services = Services(ctx, Editor(), Evaluator(), Budget(), output=tmp_path)
    result = optimize(ctx, {}, services)
    bench = captured["bench"]
    record = next((tmp_path / "harness2/executions").glob("*/evidence.json"))
    value = json.loads(record.read_text())
    assert "HIDDEN_VERIFIER_SENTINEL" not in record.read_text()
    assert bench.deliverable_files(record.parent) == {"/workspace/result.json": "2"}
    assert "answer.txt" not in bench.deliverable_files(record.parent)
    before = services.attempted_trials
    # Invoke the identical upstream rollout twice, with the matching stored harness.
    baseline = bench.execute(ctx.case_id, None, "idempotency-check", timeout=2)
    assert bench.execute(ctx.case_id, None, "idempotency-check") == baseline
    assert services.attempted_trials == before + 1
    pending = tmp_path / "harness2/executions" / hashlib.sha256(b"uncertain").hexdigest()
    pending.mkdir()
    with pytest.raises(OptimizationError, match="incomplete"):
        bench.execute(ctx.case_id, None, "uncertain")
    assert services.attempted_trials == before + 1
    assert result["harness2"]["diagnostics"]["proposer_channel"] == "catena-codex-json"
    audits = list((tmp_path / "harness2/proposer").glob("*.json"))
    assert len(audits) == services.calls == 4
    assert all(json.loads(p.read_text())["input_tokens"] == 100 for p in audits)
    assert list((tmp_path / "harness2/library").rglob("*"))
    assert value["run_id"]


def test_final_reply_without_requested_artifact_cannot_supply_selection(tmp_path):
    from dataclasses import replace

    class NoFiles(Evaluator):
        def probe(self, *args):
            return replace(super().probe(*args), deliverables={})

    services = Services(context(), Editor(), NoFiles(), Budget(), output=tmp_path)
    with pytest.raises(OptimizationError):
        optimize(context(), {}, services)
    assert services.trials and all(t.answer == "2" for t in services.trials)


def test_custom_proposer_forwards_system_and_deadline_and_reports_actual_attempts(tmp_path):
    from types import SimpleNamespace

    from catena_engine.rsi.harness2 import HostProposer

    captured = {}

    def complete(prompt, timeout):
        captured.update(prompt=json.loads(prompt), timeout=timeout)
        return Completion({"text": "done", "files": []}, 3, 2)

    directory = tmp_path / "context"
    directory.mkdir()
    prop = HostProposer(
        SimpleNamespace(complete=complete), SimpleNamespace(kind="catena-codex-json"), tmp_path / "audit"
    )
    reply = prop.call("inspect", directory, "inspect", system="system instruction", timeout=7, max_attempts=2)
    assert captured["prompt"]["system"] == "system instruction" and captured["timeout"] == 7
    assert reply.attempts == 1 and reply.wall_s > 0 and reply.audit.is_file()


def test_private_http_runs_registered_algorithm_and_persists_private_report(monkeypatch, tmp_path):
    import threading

    from test_rsi import payload
    from test_server import call

    from catena_engine.evaluation.harbor import task_digest
    from catena_engine.rsi import api
    from catena_engine.server import create_server

    class LocalEditor(Editor):
        def __init__(self, model):
            super().__init__()

        def preflight(self):
            pass

    class FixtureEvaluator(Evaluator):
        def __init__(self, cases, output, model):
            super().__init__()

        def preflight(self):
            pass

    monkeypatch.setattr(api, "CodexRuntime", LocalEditor)
    monkeypatch.setattr(api, "HarborEvaluator", FixtureEvaluator)
    cases = {}
    for name in ("training", "heldout"):
        directory = tmp_path / name
        directory.mkdir()
        (directory / "task.toml").write_text('schema_version = "1.4"\nartifacts = ["/workspace/result.json"]\n')
        (directory / "instruction.md").write_text("Inspect signed amounts.")
        cases[name] = {"path": str(directory), "sha256": task_digest(directory), "confirmed_by": "protocol-fixture"}
    manifest = tmp_path / "cases.json"
    manifest.write_text(json.dumps(cases))
    monkeypatch.setenv("CATENA_RSI_CASES_MANIFEST", str(manifest))
    monkeypatch.setenv("CATENA_RSI_RUNS_ROOT", str(tmp_path / "runs"))
    request = payload()
    request["harness2"] = {"k": 1}
    request["validation_case_ids"] = ["heldout"]
    request["model"]["api_key"] = "APIKEY_DO_NOT_PERSIST"
    server = create_server("127.0.0.1:0", token="private", receipt_root=tmp_path / "receipts")
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        status, result = call(f"http://127.0.0.1:{server.server_port}/v2/rsi", request, "private")
        assert status == 200 and result["state"] == "completed"
        result = result["output"]
        assert result["status"] == "completed"
        assert result["decision"] == "review_required"
        assert result["usage"]["model_calls"] == 4
        assert result["usage"]["attempted_trials"] == 8
        assert [t["phase"] for t in result["trials"]] == ["probe"] * 4 + ["validation"] * 4
        report = tmp_path / "runs" / result["experiment_id"] / "result.json"
        assert json.loads(report.read_text()) == result
        assert report.stat().st_mode & 0o777 == 0o600
        assert "APIKEY_DO_NOT_PERSIST" not in report.read_text()
        assert "trajectory" not in result["trials"][0]
        assert result["case_fingerprints"] == {k: v["sha256"] for k, v in cases.items()}
    finally:
        server.shutdown()
        server.server_close()
