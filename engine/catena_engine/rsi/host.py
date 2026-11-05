"""Budget, cancellation and trial evidence for the single Harness² integration."""

from __future__ import annotations

import threading
import time
from dataclasses import asdict, replace

from .contracts import (
    RESULT_SCHEMA,
    Budget,
    Candidate,
    Completion,
    Context,
    OptimizationError,
    Trial,
)
from .harness2 import Harness2Adapter


def trial_summary(trial: Trial) -> dict:
    return {key: value for key, value in asdict(trial).items() if key not in ("trajectory", "answer")}


class Services:
    """Enforce limits at paid-operation boundaries, including failed attempts."""

    def __init__(
        self, context: Context, runtime, evaluator, budget: Budget, reserved_trials: int = 0
    ):
        self.context, self.runtime, self.evaluator, self.budget = context, runtime, evaluator, budget
        self.deadline = time.monotonic() + budget.seconds
        self.lock = threading.RLock()
        self.cancelled = threading.Event()
        self.calls = 0
        self.completed_calls = 0
        self.trials: list[Trial] = []
        self.attempted_trials = 0
        self.input_tokens = self.output_tokens = 0
        self.failure = ""
        self.reserved_trials = reserved_trials

    def remaining(self) -> float:
        if self.failure:
            raise OptimizationError(self.failure)
        if self.cancelled.is_set() or self.deadline <= time.monotonic():
            self.failure = "RSI cancelled or deadline exceeded"
            raise OptimizationError(self.failure)
        return self.deadline - time.monotonic()

    def task_prompt(self) -> str:
        return self.evaluator.task_prompt(self.context.case_id)

    def complete(self, prompt: str) -> Completion:
        with self.lock:
            timeout = self.remaining()
            if self.calls >= self.budget.model_calls:
                self.failure = "RSI model-call budget exceeded"
                raise OptimizationError(self.failure)
            if len(prompt.encode()) > 1_000_000:
                raise OptimizationError("RSI context exceeds model input limit; no evidence was silently truncated")
            self.calls += 1  # Failed calls consume budget too.
            try:
                reply = self.runtime.complete(prompt, timeout, self.cancelled)
            except Exception:
                self.failure = "RSI model operation failed"
                raise OptimizationError(self.failure) from None
            if any(type(n) is not int or n < 0 for n in (reply.input_tokens, reply.output_tokens)):
                raise OptimizationError("invalid model token usage")
            self.input_tokens += reply.input_tokens
            self.output_tokens += reply.output_tokens
            self.completed_calls += 1
            self.remaining()
            return reply

    def probe(self, candidate: Candidate) -> Trial:
        with self.lock:
            if self.attempted_trials >= self.budget.trials - self.reserved_trials:
                self.failure = "RSI probe budget exceeded; validation trials are reserved"
                raise OptimizationError(self.failure)
            return self._trial(self.context.case_id, candidate, "probe")

    def validate_candidate(self, case_id: str, candidate: Candidate) -> Trial:
        with self.lock:
            if self.reserved_trials <= 0:
                raise OptimizationError("no validation trial budget was reserved")
            self.reserved_trials -= 1
            return self._trial(case_id, candidate, "validation")

    def _trial(self, case_id: str, candidate: Candidate, phase: str) -> Trial:
        with self.lock:
            timeout = self.remaining()
            if self.attempted_trials >= self.budget.trials:
                self.failure = "RSI trial budget exceeded"
                raise OptimizationError(self.failure)
            self.attempted_trials += 1
            try:
                trial = self.evaluator.probe(case_id, candidate, timeout, self.cancelled)
            except Exception:
                self.failure = "RSI evaluator operation failed"
                raise OptimizationError(self.failure) from None
            if trial.case_id != case_id or trial.candidate_id != candidate.id:
                raise OptimizationError("evaluator returned mismatched evidence")
            trial = replace(trial, phase=phase)
            self.trials.append(trial)
            if not trial.valid:
                self.failure = "RSI probe is invalid; no candidate may be published"
                raise OptimizationError(self.failure)
            self.remaining()
            return trial

    def usage(self) -> dict:
        return {
            "model_calls": self.calls,
            "attempted_trials": self.attempted_trials,
            "input_tokens": self.input_tokens,
            "output_tokens": self.output_tokens,
            "optimizer_usage_complete": self.calls == self.completed_calls,
            "trial_usage_complete": len(self.trials) == self.attempted_trials
            and all(t.input_tokens is not None and t.output_tokens is not None for t in self.trials),
            "trial_input_tokens": [t.input_tokens for t in self.trials],
            "trial_output_tokens": [t.output_tokens for t in self.trials],
        }


def preflight(options: dict, budget: Budget, reserved_trials: int = 0):
    harness = Harness2Adapter()
    harness.validate(options)
    required = harness.requirements(options)
    if required.model_calls > budget.model_calls:
        raise OptimizationError("model-call budget does not cover Harness2 requirements")
    if required.trials + reserved_trials > budget.trials:
        raise OptimizationError("trial budget does not cover Harness2 and comparison")


def optimize(context: Context, options: dict, services: Services) -> dict:
    preflight(options, services.budget, services.reserved_trials)
    harness = Harness2Adapter()
    services.remaining()
    result = harness.optimize(context, services, options)
    services.remaining()
    if len(result.candidates) != 1 or context.baseline.id not in result.candidates[0].parents:
        raise OptimizationError("Harness2 must return one candidate citing its baseline")
    return {
        "schema": RESULT_SCHEMA,
        "request_id": context.request_id,
        "status": "completed",
        "decision": "evidence_insufficient",
        "evidence_refs": list(context.evidence_refs),
        "baseline_id": context.baseline.id,
        "candidates": [c.to_dict() for c in result.candidates],
        "usage": services.usage(),
        "trials": [trial_summary(t) for t in services.trials],
        "harness2": {"revision": harness.version, "diagnostics": result.diagnostics},
    }
