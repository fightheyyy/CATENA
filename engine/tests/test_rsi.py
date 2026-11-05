from dataclasses import replace

import pytest

from catena_engine.rsi.api import parse, run_and_validate
from catena_engine.rsi.contracts import (
    SCHEMA,
    Budget,
    Candidate,
    Completion,
    Context,
    OptimizationError,
    OptimizationResult,
    Requirements,
    Trial,
)
from catena_engine.rsi.host import Services, optimize


class Runtime:
    def __init__(self):
        self.prompts = []

    def complete(self, prompt, timeout, cancelled=None):
        self.prompts.append(prompt)
        return Completion({"text": "ok", "files": []}, 10, 2)


class Evaluator:
    def __init__(self):
        self.seen = []

    def task_prompt(self, case_id):
        return "Return the sum of the signed amounts."

    def probe(self, case_id, candidate, timeout, cancelled=None):
        self.seen.append((case_id, candidate.id))
        return Trial(
            str(len(self.seen)),
            case_id,
            candidate.id,
            "[ACTION] inspect\n[OBSERVATION] sum=2",
            "2",
            True,
            {"HIDDEN_VERIFIER_SENTINEL": 1},
            20,
            3,
        )


class Strategy:
    name, version = "custom", "1"

    def validate(self, options):
        if options:
            raise OptimizationError("unknown option")

    def requirements(self, options):
        return Requirements(1, 1)

    def optimize(self, ctx, services, options):
        services.complete("analyze " + ctx.evidence)
        child = Candidate(
            {"AGENTS.md": ctx.baseline.files["AGENTS.md"] + "\nCheck inputs."}, (ctx.baseline.id,), self.name
        )
        services.probe(child)
        return OptimizationResult((child,))


def context():
    return Context("request", "training", Candidate({"AGENTS.md": "Follow the task."}), "Observed repeated search.")


@pytest.fixture
def single_harness(monkeypatch):
    from catena_engine.rsi import host
    monkeypatch.setattr(host, "Harness2Adapter", Strategy)


def test_single_candidate_keeps_lineage_and_limits(single_harness):
    ctx = context()
    services = Services(ctx, Runtime(), Evaluator(), Budget(1, 1, 5))
    result = optimize(ctx, {}, services)
    assert len(result["candidates"]) == 1
    assert result["candidates"][0]["parents"] == [ctx.baseline.id]
    assert result["candidates"][0]["status"] == "unverified"
    assert result["decision"] == "evidence_insufficient"
    assert result["usage"]["model_calls"] == result["usage"]["attempted_trials"] == 1


def test_preflight_rejects_before_paid_work(single_harness):
    services = Services(context(), Runtime(), Evaluator(), Budget())
    with pytest.raises(OptimizationError, match="unknown option"):
        optimize(context(), {"unknown": True}, services)
    assert services.calls == services.attempted_trials == 0


@pytest.mark.parametrize("kind", ["calls", "trials", "deadline", "cancel"])
def test_limits_stop_further_work(kind):
    services = Services(context(), Runtime(), Evaluator(), Budget(1, 1, 5))
    if kind == "calls":
        services.complete("one")
    elif kind == "trials":
        services.probe(context().baseline)
    elif kind == "deadline":
        services.deadline = 0
    else:
        services.cancelled.set()
    with pytest.raises(OptimizationError):
        services.probe(context().baseline) if kind == "trials" else services.complete("denied")
    assert len(services.runtime.prompts) <= 1 and len(services.evaluator.seen) <= 1


def test_failed_calls_count_and_do_not_leak_provider_errors():
    class Broken(Runtime):
        def complete(self, prompt, timeout, cancelled=None):
            raise RuntimeError("SECRET_PROVIDER_KEY")

    services = Services(context(), Broken(), Evaluator(), Budget())
    with pytest.raises(OptimizationError) as error:
        services.complete("test")
    assert "SECRET_PROVIDER_KEY" not in str(error.value)
    assert services.calls == 1


def test_validation_is_reserved_and_happens_after_candidate_freeze(single_harness):
    ctx = context()
    services = Services(ctx, Runtime(), Evaluator(), Budget(1, 5, 5), reserved_trials=4)
    result = run_and_validate(ctx, {}, services, ["heldout"])
    assert [t["case_id"] for t in result["trials"]] == ["training", "training", "training", "heldout", "heldout"]
    assert [t["phase"] for t in result["trials"]] == ["probe"] + ["validation"] * 4
    assert result["decision"] == "review_required"
    assert "heldout" not in "".join(services.runtime.prompts)


def test_probes_cannot_spend_validation_reserve():
    services = Services(context(), Runtime(), Evaluator(), Budget(1, 2, 5), reserved_trials=2)
    with pytest.raises(OptimizationError, match="reserved"):
        services.probe(context().baseline)
    assert services.attempted_trials == 0


@pytest.mark.parametrize(
    "path",
    [
        "../secret",
        "/AGENTS.md",
        "scripts/a.py",
        ".codex/hooks.json",
        ".agents/skills/../../x/SKILL.md",
        "AGENTS.md\x00",
    ],
)
def test_candidate_cannot_expand_edit_surface(path):
    with pytest.raises(OptimizationError):
        Candidate({"AGENTS.md": "Task", path: "malicious"})


def test_candidate_is_immutable_and_versions_keep_content_hash():
    base = Candidate({"AGENTS.md": "Task"})
    with pytest.raises(TypeError):
        base.files["AGENTS.md"] = "changed"
    child = Candidate(dict(base.files), (base.id,), "custom")
    assert child.id != base.id and child.content_sha256 == base.content_sha256


def payload():
    return {
        "schema": SCHEMA,
        "request_id": "rsi-1",
        "case_id": "training",
        "baseline": {"AGENTS.md": "Follow the task."},
        "harness2": {},
        "model": {
            "provider": "openai",
            "base_url": "https://example.test/v1",
            "model": "fixture",
            "api_key": "synthetic-test-key",
        },
    }


@pytest.mark.parametrize(
    "patch",
    [
        {"case_id": "../outside"},
        {"harness2": []},
        {"budget": {"trials": True}},
        {"budget": {"model_calls": 0}},
        {"validation_case_ids": ["training"]},
        {"validation_case_ids": ["heldout", "heldout"]},
        {"shell_command": "arbitrary"},
        {"algorithms": []},
        {"composition": "parallel"},
    ],
)
def test_invalid_requests_fail_before_execution(patch):
    with pytest.raises(OptimizationError):
        parse({**payload(), **patch})


def test_invalid_trials_fail_closed():
    class Bad(Evaluator):
        def probe(self, *args):
            return replace(super().probe(*args), valid=False)

    services = Services(context(), Runtime(), Bad(), Budget())
    with pytest.raises(OptimizationError, match="invalid"):
        services.probe(context().baseline)


def test_inflight_cancellation_awaits_operation_cleanup():
    import asyncio
    import threading

    from catena_engine.rsi.cancellation import cancellable

    stopped = threading.Event()
    cancelled = threading.Event()

    async def operation():
        cancelled.set()
        try:
            await asyncio.sleep(60)
        finally:
            stopped.set()

    with pytest.raises(OptimizationError, match="cancelled"):
        asyncio.run(cancellable(operation(), cancelled))
    assert stopped.is_set()


def test_compare_without_heldout_is_explicit_about_limits(single_harness):
    ctx = context()
    services = Services(ctx, Runtime(), Evaluator(), Budget(1, 3, 5), reserved_trials=2)
    result = run_and_validate(ctx, {}, services, [])
    assert result["decision"] == "review_required"
    pair, = result["compare"]
    assert pair["held_out"] is False
    assert pair["baseline"]["candidate_id"] == ctx.baseline.id
    assert pair["candidate"]["candidate_id"] == result["candidates"][0]["id"]
    assert pair["baseline"]["cost_usd"] is None
    assert "HIDDEN_VERIFIER_SENTINEL" not in "".join(services.runtime.prompts)
