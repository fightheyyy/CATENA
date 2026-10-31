import json

import pytest

pytest.importorskip("harness2")

from test_rsi import Evaluator, context

from catena_engine.rsi.contracts import Algorithm, Budget, Completion, OptimizationError
from catena_engine.rsi.host import Services, optimize
from catena_engine.rsi.registry import default_registry


class Editor:
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
def test_upstream_loop_uses_host_and_keeps_grades_out(mode, k):
    ctx, editor, evaluator = context(), Editor(), Evaluator()
    services = Services(ctx, editor, evaluator, Budget(20, 16, 10))
    result = optimize(ctx, [Algorithm("harness2", {"mode": mode, "k": k})], "sequential", default_registry(), services)
    candidate = result["candidates"][0]
    assert candidate["files"]["AGENTS.md"] == "Check available tools; use portable search."
    ledger = result["algorithms"][0]["diagnostics"]["ledger"]
    assert (ledger["R"], ledger["P"]) == (4 * k, 3 * k)
    assert len(evaluator.seen) == 4 * k
    assert services.calls == 3 * k + 1 + (1 if k > 1 else 0)
    assert candidate["parents"] == [ctx.baseline.id]
    assert candidate["id"] in [t["candidate_id"] for t in result["trials"]]
    assert result["decision"] == "evidence_insufficient"
    assert "Observed repeated search." in editor.prompts[0]


def test_harness2_adapters_can_be_composed_in_parallel():
    ctx = context()
    services = Services(ctx, Editor(), Evaluator(), Budget(10, 8, 10))
    result = optimize(ctx, [Algorithm("harness2"), Algorithm("harness2")], "parallel", default_registry(), services)
    assert len(result["candidates"]) == 2
    assert services.calls == services.attempted_trials == 8
    assert all(c["parents"] == [ctx.baseline.id] for c in result["candidates"])


def test_failure_cannot_publish_a_fallback_candidate():
    services = Services(context(), Editor(), Evaluator(), Budget(12, 3, 10))
    with pytest.raises(OptimizationError):
        optimize(context(), [Algorithm("harness2")], "sequential", default_registry(), services)
    assert services.attempted_trials == 0


def test_upstream_cannot_emit_scripts_or_hooks():
    class Unsafe(Editor):
        def complete(self, prompt, timeout, cancelled=None):
            return Completion({"text": "edit", "files": [{"path": ".codex/hooks.json", "content": "{}"}]}, 1, 1)

    services = Services(context(), Unsafe(), Evaluator(), Budget())
    with pytest.raises(OptimizationError):
        optimize(context(), [Algorithm("harness2")], "sequential", default_registry(), services)
    assert services.attempted_trials == 1


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
        (directory / "task.toml").write_text('schema_version = "1.4"\n')
        (directory / "instruction.md").write_text("Inspect signed amounts.")
        cases[name] = {"path": str(directory), "sha256": task_digest(directory)}
    manifest = tmp_path / "cases.json"
    manifest.write_text(json.dumps(cases))
    monkeypatch.setenv("CATENA_RSI_CASES_MANIFEST", str(manifest))
    monkeypatch.setenv("CATENA_RSI_RUNS_ROOT", str(tmp_path / "runs"))
    request = payload()
    request["algorithms"] = [{"name": "harness2", "options": {"k": 1}}]
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
        assert result["usage"]["attempted_trials"] == 6
        assert [t["phase"] for t in result["trials"]] == ["probe"] * 4 + ["validation"] * 2
        report = tmp_path / "runs" / result["experiment_id"] / "result.json"
        assert json.loads(report.read_text()) == result
        assert report.stat().st_mode & 0o777 == 0o600
        assert "APIKEY_DO_NOT_PERSIST" not in report.read_text()
        assert "trajectory" not in result["trials"][0]
        assert result["case_fingerprints"] == {k: v["sha256"] for k, v in cases.items()}
    finally:
        server.shutdown()
        server.server_close()
