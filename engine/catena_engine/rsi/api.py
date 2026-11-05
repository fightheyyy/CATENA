"""Private RSI adapter invoked by the Go analysis queue."""

from __future__ import annotations

import json
import os
import uuid
from pathlib import Path

from ..evaluation.harbor import HarborCase, HarborEvaluator
from ..protocol import InvalidRequest, _model_config
from .contracts import SCHEMA, Budget, Candidate, Context, OptimizationError, safe_id
from .host import Services, optimize, preflight, trial_summary
from .runtime import CodexRuntime


def parse(payload: dict):
    if payload.get("schema") != SCHEMA:
        raise OptimizationError("unsupported RSI request schema")
    allowed = {
        "schema",
        "request_id",
        "case_id",
        "baseline",
        "evidence",
        "evidence_refs",
        "harness2",
        "budget",
        "model",
        "validation_case_ids",
    }
    if set(payload) - allowed:
        raise OptimizationError("unknown RSI request field")
    evidence = payload.get("evidence", "")
    refs = payload.get("evidence_refs", [])
    if not isinstance(evidence, str) or len(evidence.encode()) > 512_000:
        raise OptimizationError("evidence must be text of at most 512000 bytes")
    if not isinstance(refs, list) or len(refs) > 128 or any(not isinstance(v, str) or len(v) > 256 for v in refs):
        raise OptimizationError("invalid evidence references")
    context = Context(
        safe_id(payload.get("request_id")),
        safe_id(payload.get("case_id")),
        Candidate(payload.get("baseline")),
        evidence,
        tuple(refs),
    )
    options = payload.get("harness2", {})
    if not isinstance(options, dict):
        raise OptimizationError("harness2 options must be an object")
    limits = payload.get("budget", {})
    if not isinstance(limits, dict) or set(limits) - {"model_calls", "trials", "seconds"}:
        raise OptimizationError("unknown budget field")
    budget = Budget(**limits)
    cases = payload.get("validation_case_ids", [])
    if not isinstance(cases, list) or len(cases) > 4:
        raise OptimizationError("at most four validation Cases are allowed")
    cases = [safe_id(value) for value in cases]
    if len(set(cases)) != len(cases) or context.case_id in cases:
        raise OptimizationError("validation Cases must be distinct from optimization Case")
    try:
        model = _model_config(payload.get("model"))
    except InvalidRequest:
        raise OptimizationError("invalid owner model configuration") from None
    return context, options, budget, model, cases


def run_payload(payload: dict, cancelled=None) -> dict:
    context, options, budget, model, validation = parse(payload)
    manifest_path = os.environ.get("CATENA_RSI_CASES_MANIFEST")
    runs_root = os.environ.get("CATENA_RSI_RUNS_ROOT")
    if not manifest_path or not runs_root:
        raise OptimizationError("RSI is opt-in; configure CATENA_RSI_CASES_MANIFEST and CATENA_RSI_RUNS_ROOT")
    manifest = json.loads(Path(manifest_path).read_text())
    if not isinstance(manifest, dict):
        raise OptimizationError("invalid administrator Case manifest")
    required = [context.case_id, *validation]
    cases = {}
    for case_id in required:
        if case_id not in manifest:
            raise OptimizationError("Case is not registered by the operator")
        value = manifest[case_id]
        if not isinstance(value, dict) or not isinstance(value.get("confirmed_by"), str) or not value["confirmed_by"].strip():
            raise OptimizationError("Case requires operator human confirmation before model execution")
        cases[case_id] = HarborCase.load(value["path"], value["sha256"])
    reserved = 2 * (1 + len(validation))
    preflight(options, budget, reserved)
    runtime = CodexRuntime(model)
    runtime.preflight()
    output = Path(runs_root) / (context.request_id + "-" + uuid.uuid4().hex)
    output.mkdir(parents=True, mode=0o700)
    evaluator = HarborEvaluator(cases, output, model)
    evaluator.preflight()
    services = Services(context, runtime, evaluator, budget, reserved)
    if cancelled is not None:
        services.cancelled = cancelled
    try:
        result = run_and_validate(context, options, services, validation)
    except Exception:
        result = {
            "schema": "catena.rsi_result.v1",
            "request_id": context.request_id,
            "status": "failed",
            "error": services.failure or "RSI refinement rejected invalid output",
            "decision": "evidence_insufficient",
            "candidates": [],
            "usage": services.usage(),
            "trials": [trial_summary(t) for t in services.trials],
        }
    result["experiment_id"] = output.name
    result["case_confirmations"] = {name: manifest[name]["confirmed_by"] for name in cases}
    result["case_fingerprints"] = {name: case.sha256 for name, case in cases.items()}
    result["model"] = {
        "provider": model.provider,
        "model": model.model,
        "optimizer_reasoning_effort": os.environ.get("CATENA_CODEX_REASONING_EFFORT", "low"),
        "target_reasoning_effort": "low",
        "runtime": "codex-cli@0.160.0",
    }
    result["environment"] = {"backend": "docker", "cpus": 1, "memory_mb": 1024}
    with os.fdopen(os.open(output / "result.json", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as report:
        json.dump(result, report, ensure_ascii=False, indent=2, allow_nan=False)
    return result


def run_and_validate(context, options, services, validation):
    result = optimize(context, options, services)
    value = result["candidates"][0]
    candidate = Candidate(value["files"], tuple(value["parents"]), value["algorithm"], value["notes"])
    comparisons = []
    # Freeze the candidate before either comparison run; rewards never enter optimization.
    for case_id in [context.case_id, *validation]:
        baseline_trial = services.validate_candidate(case_id, context.baseline)
        candidate_trial = services.validate_candidate(case_id, candidate)
        comparisons.append({
            "case_id": case_id,
            "held_out": case_id != context.case_id,
            "baseline": trial_summary(baseline_trial),
            "candidate": trial_summary(candidate_trial),
        })
    result.update(
        trials=[trial_summary(t) for t in services.trials],
        usage=services.usage(),
        decision="review_required",
        compare=comparisons,
        validation_case_ids=validation,
        limitations=[
            "Optimizer selection is not proof of improvement; no automatic adoption.",
            "One pair per Case establishes no general or statistically significant improvement.",
            "Optimization Case is reused unless distinct held-out Cases are supplied.",
            "Token usage is measured, not a provider-enforced token or money cap.",
            "Missing provider cost is unknown, never zero.",
        ],
    )
    return result
