"""Private RSI adapter invoked by the Go analysis queue."""

from __future__ import annotations

import json
import os
import uuid
from pathlib import Path

from ..protocol import InvalidRequest, _model_config
from .contracts import SCHEMA, Algorithm, Budget, Candidate, Context, OptimizationError, safe_id
from ..evaluation.harbor import HarborCase, HarborEvaluator
from .host import Services, optimize, preflight, trial_summary
from .registry import default_registry
from .runtime import CodexRuntime


def parse(payload: dict):
    if payload.get("schema") != SCHEMA:
        raise OptimizationError("unsupported RSI request schema")
    allowed = {"schema", "request_id", "case_id", "baseline", "evidence", "evidence_refs", "algorithms",
               "composition", "budget", "model", "validation_case_ids"}
    if set(payload) - allowed:
        raise OptimizationError("unknown RSI request field")
    evidence = payload.get("evidence", "")
    refs = payload.get("evidence_refs", [])
    if not isinstance(evidence, str) or len(evidence.encode()) > 512_000:
        raise OptimizationError("evidence must be text of at most 512000 bytes")
    if not isinstance(refs, list) or len(refs) > 128 or any(not isinstance(v, str) or len(v) > 256 for v in refs):
        raise OptimizationError("invalid evidence references")
    context = Context(safe_id(payload.get("request_id")), safe_id(payload.get("case_id")),
                      Candidate(payload.get("baseline")), evidence, tuple(refs))
    specs = payload.get("algorithms")
    if not isinstance(specs, list) or not 1 <= len(specs) <= 4:
        raise OptimizationError("1–4 algorithms are required")
    algorithms = []
    for spec in specs:
        if (not isinstance(spec, dict) or set(spec) - {"name", "options"}
                or not isinstance(spec.get("options", {}), dict)):
            raise OptimizationError("invalid algorithm specification")
        algorithms.append(Algorithm(safe_id(spec.get("name")), spec.get("options", {})))
    composition = payload.get("composition", "sequential")
    if composition not in ("sequential", "parallel"):
        raise OptimizationError("invalid composition")
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
    return context, algorithms, composition, budget, model, cases


def run_payload(payload: dict, cancelled=None) -> dict:
    context, algorithms, composition, budget, model, validation = parse(payload)
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
        cases[case_id] = HarborCase.load(value["path"], value["sha256"])
    # Every strategy is resolved before directories or paid operations are created.
    registry = default_registry()
    reserved = len(validation) * (2 if composition == "sequential" else len(algorithms) + 1)
    preflight(algorithms, composition, registry, budget, reserved)
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
        result = run_and_validate(context, algorithms, composition, registry, services, validation)
    except Exception:
        result = {"schema": "catena.rsi_result.v1", "request_id": context.request_id, "status": "failed",
                  "error": services.failure or "RSI refinement rejected invalid output",
                  "decision": "evidence_insufficient", "candidates": [], "usage": services.usage(),
                  "trials": [trial_summary(t) for t in services.trials]}
    result["experiment_id"] = output.name
    result["case_fingerprints"] = {name: case.sha256 for name, case in cases.items()}
    result["model"] = {"provider": model.provider, "model": model.model,
                       "optimizer_reasoning_effort": os.environ.get("CATENA_CODEX_REASONING_EFFORT", "low"),
                       "target_reasoning_effort": "low",
                       "runtime": "codex-cli@0.160.0"}
    result["environment"] = {"backend": "docker", "cpus": 1, "memory_mb": 1024}
    with os.fdopen(os.open(output / "result.json", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as report:
        json.dump(result, report, ensure_ascii=False, indent=2, allow_nan=False)
    return result


def run_and_validate(context, algorithms, composition, registry, services, validation):
    result = optimize(context, algorithms, composition, registry, services)
    if validation:
        terminal = result["candidates"][-1:] if composition == "sequential" else result["candidates"]
        for case_id in validation:
            services.validate_candidate(case_id, context.baseline)
            for value in terminal:
                candidate = Candidate(value["files"], tuple(value["parents"]), value["algorithm"], value["notes"])
                services.validate_candidate(case_id, candidate)
        result["trials"] = [trial_summary(t) for t in services.trials]
        result["usage"] = services.usage()
        result["decision"] = "review_required"
    result["validation_case_ids"] = validation
    result["limitations"] = [
        "Optimizer selection is not proof of improvement; no automatic adoption.",
        "Token usage is measured, not a provider-enforced token or money cap.",
    ]
    return result
