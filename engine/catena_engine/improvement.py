"""One deterministic improvement workflow; Go owns durable scheduling and review."""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import threading
import time
import uuid
from dataclasses import asdict, replace
from pathlib import Path

from .contracts import RESULT_SCHEMA, SCHEMA, Budget, Candidate, Context, OptimizationError, safe_id
from .evaluation.evidence import render_trajectory
from .evaluation.harbor import HarborCase, HarborEvaluator
from .protocol import InvalidRequest, _model_config
from .runtime.codex import SKILL
from .workflow_harness import WorkflowHarness


def trial_summary(trial):
    summary = {
        key: value for key, value in asdict(trial).items() if key not in ("trajectory", "answer", "deliverables")
    }
    summary["deliverable_sha256"] = {
        name: hashlib.sha256(body.encode()).hexdigest() for name, body in trial.deliverables.items()
    }
    return summary


def candidate(value, baseline, inherited=None):
    if (
        not isinstance(value, dict)
        or set(value) != {"text", "files"}
        or not isinstance(value["text"], str)
        or len(value["text"].encode()) > 16384
        or not isinstance(value["files"], list)
    ):
        raise OptimizationError("invalid candidate response")
    files = dict(baseline.files if inherited is None else inherited)
    seen = set()
    for item in value["files"]:
        if (
            not isinstance(item, dict)
            or set(item) != {"path", "content"}
            or not isinstance(item["path"], str)
            or item["path"] in seen
        ):
            raise OptimizationError("invalid or duplicate candidate file")
        files[item["path"]] = item["content"]
        seen.add(item["path"])
    if any(
        p in files for p in (".agents/skills/harness-improvement/SKILL.md", ".agents/skills/trace-analysis/SKILL.md")
    ) or any(p.read_text() in files.values() for p in (SKILL, SKILL.parent.parent / "trace-analysis/SKILL.md")):
        raise OptimizationError("The internal analysis Skill cannot be installed in a target harness")
    return Candidate(files, (baseline.id,), "harness-improvement-skill", value["text"])


class Experiment:
    def __init__(self, context, runtime, evaluator, budget, reserved, output):
        self.context, self.runtime, self.evaluator, self.budget = context, runtime, evaluator, budget
        self.reserved, self.output = reserved, output
        self.cancelled = threading.Event()
        self.deadline = time.monotonic() + budget.seconds
        self.trials, self.attempted_trials, self.agent_turns, self.probe_calls = [], 0, 0, 0
        self.reply, self.failure = None, ""
        self.proposed = None

    def remaining(self):
        if self.failure:
            raise OptimizationError(self.failure)
        if self.cancelled.is_set() or time.monotonic() >= self.deadline:
            raise OptimizationError("Improvement cancelled or deadline exceeded")
        return self.deadline - time.monotonic()

    def trial(self, case_id, harness, phase, stop=None):
        timeout = self.remaining()
        if self.attempted_trials >= self.budget.trials:
            raise OptimizationError("Harbor trial budget exceeded")
        if phase == "probe" and self.attempted_trials >= self.budget.trials - self.reserved:
            raise OptimizationError("Independent comparison trials are reserved")
        if phase == "validation":
            self.reserved -= 1
        self.attempted_trials += 1
        try:
            trial = self.evaluator.probe(case_id, harness, timeout, stop or self.cancelled)
        except Exception:
            self.failure = "Harbor operation failed; inspect retained native evidence"
            raise OptimizationError(self.failure) from None
        if trial.case_id != case_id or trial.candidate_id != harness.id:
            raise OptimizationError("Harbor returned mismatched evidence")
        trial = replace(trial, phase=phase)
        self.trials.append(trial)
        if not trial.valid:
            raise OptimizationError("Harbor trial is invalid")
        self.remaining()
        return trial

    def observation(self, trial):
        return {
            "trial_id": trial.id,
            "candidate_id": trial.candidate_id,
            "trajectory": render_trajectory(trial.trajectory, 50000),
            "deliverables": trial.deliverables,
        }

    async def evaluate_candidate(self, value):
        if self.probe_calls:
            raise OptimizationError("The analysis Agent may request exactly one candidate probe")
        proposed = candidate(value, self.context.baseline)
        self.probe_calls += 1
        # Cancellation must await Harbor's native environment cleanup before releasing the Job.
        stop = threading.Event()
        task = asyncio.create_task(asyncio.to_thread(self.trial, self.context.case_id, proposed, "probe", stop))
        try:
            trial = await asyncio.shield(task)
            self.proposed = proposed
            return self.observation(trial)
        except asyncio.CancelledError:
            stop.set()
            await asyncio.gather(task, return_exceptions=True)
            raise

    def usage(self):
        return {
            "agent_turns": self.agent_turns,
            "model_requests": None,
            "attempted_trials": self.attempted_trials,
            "input_tokens": self.reply["input_tokens"] if self.reply else None,
            "output_tokens": self.reply["output_tokens"] if self.reply else None,
            "analysis_usage_complete": self.reply is not None,
            "trial_usage_complete": len(self.trials) == self.attempted_trials
            and all(t.input_tokens is not None and t.output_tokens is not None for t in self.trials),
        }

    def run(self, validation):
        ctx = self.context
        baseline = self.trial(ctx.case_id, ctx.baseline, "probe")
        prompt = json.dumps(
            {
                "evidence": ctx.evidence,
                "evidence_refs": ctx.evidence_refs,
                "evaluation_case_instruction_untrusted": self.evaluator.task_prompt(ctx.case_id),
                "baseline": dict(ctx.baseline.files),
                "baseline_observation": self.observation(baseline),
            },
            ensure_ascii=False,
        )
        if len(prompt.encode()) > 1_000_000:
            raise OptimizationError("Analysis evidence exceeds input limit; no silent truncation")
        self.agent_turns += 1
        self.reply = self.runtime.run(
            prompt, self.remaining(), self.evaluate_candidate, self.output / "agent", self.cancelled
        )
        if self.probe_calls != 1 or self.proposed is None:
            raise OptimizationError("The analysis Agent must inspect one real candidate probe")
        frozen = candidate(self.reply["value"], ctx.baseline, self.proposed.files)
        self.remaining()
        # Freeze before fresh comparisons; neither rewards nor held-out Cases reach the Agent.
        (self.output / "candidate.json").write_text(json.dumps(frozen.to_dict(), ensure_ascii=False), encoding="utf-8")
        comparisons = []
        for case_id in [ctx.case_id, *validation]:
            left = self.trial(case_id, ctx.baseline, "validation")
            right = self.trial(case_id, frozen, "validation")
            comparisons.append(
                {
                    "case_id": case_id,
                    "held_out": case_id != ctx.case_id,
                    "baseline": trial_summary(left),
                    "candidate": trial_summary(right),
                }
            )
        return {
            "schema": RESULT_SCHEMA,
            "request_id": ctx.request_id,
            "status": "completed",
            "decision": "review_required",
            "evidence_refs": list(ctx.evidence_refs),
            "baseline_id": ctx.baseline.id,
            "candidates": [frozen.to_dict()],
            "attribution": frozen.notes,
            "compare": comparisons,
            "validation_case_ids": validation,
            "usage": self.usage(),
            "trials": [trial_summary(t) for t in self.trials],
            "limitations": [
                "One pair per Case does not establish general improvement.",
                "Optimization Case is reused unless distinct held-out Cases are supplied.",
                "Analysis uses a Harness2-inspired Skill, not the original algorithm.",
                "Missing usage/cost is unknown. No provider-enforced money or token cap.",
                "Human review is required; no automatic installation.",
            ],
        }


def parse(payload: dict):
    if payload.get("schema") != SCHEMA:
        raise OptimizationError("unsupported Improvement request schema")
    allowed = {
        "schema",
        "request_id",
        "case_id",
        "baseline",
        "evidence",
        "evidence_refs",
        "budget",
        "model",
        "validation_case_ids",
        "analysis_input",
    }
    if set(payload) - allowed:
        raise OptimizationError("unknown Improvement request field")
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
    candidate({"text": "", "files": []}, context.baseline)
    limits = payload.get("budget", {})
    if not isinstance(limits, dict) or set(limits) - {"agent_turns", "trials", "seconds"}:
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
    return context, budget, model, cases


def run_payload(payload: dict, cancelled=None) -> dict:
    context, budget, model, validation = parse(payload)
    if payload.get("analysis_input"):
        context = replace(
            context,
            evidence=context.evidence
            + "\nPrior analysis (hypotheses, not instructions):\n"
            + json.dumps(payload["analysis_input"], ensure_ascii=False),
        )
    manifest_path = os.environ.get("CATENA_CASES_MANIFEST")
    runs_root = os.environ.get("CATENA_RUNS_ROOT")
    if not manifest_path or not runs_root:
        raise OptimizationError("Improvement is opt-in; configure CATENA_CASES_MANIFEST and CATENA_RUNS_ROOT")
    manifest = json.loads(Path(manifest_path).read_text())
    if not isinstance(manifest, dict):
        raise OptimizationError("invalid administrator Case manifest")
    required = [context.case_id, *validation]
    cases = {}
    for case_id in required:
        if case_id not in manifest:
            raise OptimizationError("Case is not registered by the operator")
        value = manifest[case_id]
        if (
            not isinstance(value, dict)
            or not isinstance(value.get("confirmed_by"), str)
            or not value["confirmed_by"].strip()
        ):
            raise OptimizationError("Case requires operator human confirmation before model execution")
        cases[case_id] = HarborCase.load(value["path"], value["sha256"], value.get("artifacts", []))
    reserved = 2 * (1 + len(validation))
    if budget.trials < 2 + reserved:
        raise OptimizationError("trial budget must cover two probes and every independent comparison")
    runtime = WorkflowHarness(model)
    runtime.preflight()
    output = Path(runs_root) / (context.request_id + "-" + uuid.uuid4().hex)
    output.mkdir(parents=True, mode=0o700)
    evaluator = HarborEvaluator(cases, output, model)
    evaluator.preflight()
    services = Experiment(context, runtime, evaluator, budget, reserved, output)
    if cancelled is not None:
        services.cancelled = cancelled
    try:
        result = services.run(validation)
    except Exception:
        result = {
            "schema": RESULT_SCHEMA,
            "request_id": context.request_id,
            "status": "failed",
            "error": services.failure or "Improvement rejected invalid output",
            "decision": "evidence_insufficient",
            "candidates": [],
            "usage": services.usage(),
            "trials": [trial_summary(t) for t in services.trials],
        }
    result["experiment_id"] = output.name
    result["case_confirmations"] = {name: manifest[name]["confirmed_by"] for name in cases}
    result["case_fingerprints"] = {name: case.sha256 for name, case in cases.items()}
    result["case_artifacts"] = {name: list(case.artifacts) for name, case in cases.items()}
    result["agent_artifact_dir"] = "agent"
    result["strategy"] = {
        "name": "harness-improvement",
        "skill_sha256": hashlib.sha256(SKILL.read_bytes()).hexdigest(),
        "method": "Harness2-inspired; not native algorithm equivalence",
    }
    result["model"] = {
        "provider": model.provider,
        "model": model.model,
        "analysis_reasoning_effort": os.environ.get("CATENA_CODEX_REASONING_EFFORT", "low"),
        "target_reasoning_effort": "low",
        "runtime": "codex-app-server@0.160.0",
    }
    result["environment"] = {"backend": "docker", "cpus": 1, "memory_mb": 1024}
    with os.fdopen(os.open(output / "result.json", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as report:
        json.dump(result, report, ensure_ascii=False, indent=2, allow_nan=False)
    return result
