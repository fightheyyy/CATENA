from dataclasses import replace

import pytest

from catena_engine.rsi.api import parse, run_and_validate
from catena_engine.rsi.contracts import (
    SCHEMA,
    Algorithm,
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
from catena_engine.rsi.registry import Registry


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


def registry():
    result = Registry()
    result.register(Strategy())
    return result


@pytest.mark.parametrize("composition", ["sequential", "parallel"])
def test_composition_shares_budgets_and_tracks_lineage(composition):
    ctx = context()
    services = Services(ctx, Runtime(), Evaluator(), Budget(2, 2, 5))
    result = optimize(ctx, [Algorithm("custom"), Algorithm("custom")], composition, registry(), services)
    first, second = result["candidates"]
    assert first["parents"] == [ctx.baseline.id]
    assert second["parents"] == ([first["id"]] if composition == "sequential" else [ctx.baseline.id])
    assert result["usage"]["model_calls"] == result["usage"]["attempted_trials"] == 2
    assert result["decision"] == "evidence_insufficient"
    assert all(c["status"] == "unverified" for c in result["candidates"])


def test_all_strategies_are_preflighted_before_model_calls():
    services = Services(context(), Runtime(), Evaluator(), Budget())
    with pytest.raises(OptimizationError, match="unknown RSI"):
        optimize(context(), [Algorithm("custom"), Algorithm("missing")], "sequential", registry(), services)
    assert services.calls == 0
    with pytest.raises(OptimizationError, match="duplicate"):
        registry().register(Strategy())


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


def test_validation_is_reserved_and_happens_after_candidate_freeze():
    ctx = context()
    services = Services(ctx, Runtime(), Evaluator(), Budget(1, 3, 5), reserved_trials=2)
    result = run_and_validate(ctx, [Algorithm("custom")], "sequential", registry(), services, ["heldout"])
    assert [t["case_id"] for t in result["trials"]] == ["training", "heldout", "heldout"]
    assert [t["phase"] for t in result["trials"]] == ["probe", "validation", "validation"]
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
        "algorithms": [{"name": "custom"}],
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
        {"algorithms": [{"name": "custom", "options": []}]},
        {"budget": {"trials": True}},
        {"budget": {"model_calls": 0}},
        {"validation_case_ids": ["training"]},
        {"validation_case_ids": ["heldout", "heldout"]},
        {"shell_command": "arbitrary"},
        {"algorithms": []},
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


def test_parallel_failure_cancels_siblings_before_pool_drain():
    import threading

    started = threading.Event()

    class Broken(Strategy):
        name = "broken"

        def requirements(self, options):
            return Requirements(0, 0)

        def optimize(self, ctx, services, options):
            assert started.wait(1)
            raise OptimizationError("rejected edit")

    class Waiting(Broken):
        name = "waiting"

        def optimize(self, ctx, services, options):
            started.set()
            assert services.cancelled.wait(1), "parallel sibling was not cancelled"
            raise OptimizationError("cancelled")

    reg = Registry()
    reg.register(Broken())
    reg.register(Waiting())
    services = Services(context(), Runtime(), Evaluator(), Budget())
    with pytest.raises(OptimizationError, match="rejected edit"):
        optimize(context(), [Algorithm("broken"), Algorithm("waiting")], "parallel", reg, services)
    assert services.calls == services.attempted_trials == 0
