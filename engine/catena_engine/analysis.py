"""Analysis-only task: no Case, Harbor import, tools or automatic improvement."""

import hashlib
import json
import os
import uuid
from pathlib import Path

from .contracts import OptimizationError, safe_id
from .protocol import _model_config
from .runtime.codex import SKILL, CodexRuntime


def validate_report(value, refs):
    if (
        not isinstance(value, dict)
        or set(value) != {"summary", "problems"}
        or not isinstance(value["summary"], str)
        or not isinstance(value["problems"], list)
        or len(value["problems"]) > 32
    ):
        raise OptimizationError("invalid analysis report")
    for problem in value["problems"]:
        if not isinstance(problem, dict) or set(problem) != {
            "title",
            "evidence",
            "evidence_refs",
            "hypothesis",
            "uncertainty",
        }:
            raise OptimizationError("invalid analysis problem")
        if any(
            not isinstance(problem[k], str) or not problem[k].strip()
            for k in ("title", "evidence", "hypothesis", "uncertainty")
        ):
            raise OptimizationError("problem must separate evidence, hypothesis and uncertainty")
        citations = problem["evidence_refs"]
        if (
            not isinstance(citations, list)
            or not citations
            or any(not isinstance(r, str) or r not in refs for r in citations)
        ):
            raise OptimizationError("problem cites evidence outside the frozen input")
    if len(json.dumps(value).encode()) > 128 * 1024:
        raise OptimizationError("analysis report is too large")
    return value


def run_payload(payload, cancelled=None):
    if payload.get("schema") != "catena.analysis_request.v1" or set(payload) - {
        "schema",
        "request_id",
        "evidence",
        "evidence_refs",
        "model",
    }:
        raise OptimizationError("invalid analysis request")
    request_id = safe_id(payload.get("request_id"))
    evidence, refs = payload.get("evidence"), payload.get("evidence_refs")
    if (
        not isinstance(evidence, str)
        or len(evidence.encode()) > 512000
        or not isinstance(refs, list)
        or not 1 <= len(refs) <= 128
        or any(not isinstance(r, str) or len(r) > 256 for r in refs)
    ):
        raise OptimizationError("invalid frozen analysis evidence")
    root = os.environ.get("CATENA_RUNS_ROOT")
    if not root:
        raise OptimizationError("CATENA_RUNS_ROOT is required")
    output = Path(root) / (request_id + "-" + uuid.uuid4().hex)
    output.mkdir(parents=True, mode=0o700)
    runtime = CodexRuntime(_model_config(payload.get("model")))
    runtime.preflight()
    reply = runtime.run(
        json.dumps({"evidence": evidence, "evidence_refs": refs}),
        300,
        None,
        output / "agent",
        cancelled,
        mode="analyze",
    )
    report = validate_report(reply["value"], refs)
    skill = SKILL.parent.parent / "trace-analysis/SKILL.md"
    result = {
        "schema": "catena.analysis_result.v1",
        "status": "completed",
        "decision": "awaiting_human_decision",
        "request_id": request_id,
        **report,
        "evidence_refs": refs,
        "experiment_id": output.name,
        "strategy": {"name": "trace-analysis", "skill_sha256": hashlib.sha256(skill.read_bytes()).hexdigest()},
        "model": {
            "provider": runtime.model.provider,
            "model": runtime.model.model,
            "runtime": "codex-app-server@0.160.0",
        },
        "usage": {
            "agent_turns": 1,
            "input_tokens": reply["input_tokens"],
            "output_tokens": reply["output_tokens"],
            "attempted_trials": 0,
        },
    }
    with os.fdopen(os.open(output / "result.json", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    return result
