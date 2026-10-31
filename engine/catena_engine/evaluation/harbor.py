"""Evaluator adapter for administrator-registered Harbor tasks (Docker only)."""

from __future__ import annotations

import asyncio
import hashlib
import json
import shutil
import subprocess
import time
import uuid
from dataclasses import dataclass
from pathlib import Path

from ..protocol import ModelConfig
from ..rsi.cancellation import cancellable
from ..rsi.contracts import Candidate, OptimizationError, Trial, safe_id


def task_digest(root: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            raise OptimizationError("registered Harbor tasks must not contain symlinks")
        if path.is_file():
            digest.update(path.relative_to(root).as_posix().encode() + b"\0")
            digest.update(hashlib.sha256(path.read_bytes()).digest())
    return digest.hexdigest()


@dataclass(frozen=True)
class HarborCase:
    path: Path
    sha256: str

    @classmethod
    def load(cls, path: str, sha256: str):
        root = Path(path).resolve(strict=True)
        if not root.is_dir() or any(not (root / name).is_file() for name in ("instruction.md", "task.toml")):
            raise OptimizationError("registered Harbor task is incomplete")
        if task_digest(root) != sha256:
            raise OptimizationError("registered Harbor task fingerprint differs from its manifest")
        return cls(root, sha256)


class HarborEvaluator:
    def __init__(self, cases: dict[str, HarborCase], output: Path, model: ModelConfig):
        self.cases, self.output, self.model = cases, output, model

    def preflight(self):
        try:
            from harbor.job import Job  # noqa: F401
        except ImportError:
            raise OptimizationError("RSI Harbor environment is not installed") from None
        if shutil.which("docker") is None:
            raise OptimizationError("RSI Harbor requires a Linux Docker host")
        try:
            subprocess.run(
                ["docker", "info"], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10
            )
        except (OSError, subprocess.SubprocessError):
            raise OptimizationError("RSI Harbor Docker preflight failed") from None

    def task_prompt(self, case_id):
        if case_id not in self.cases:
            raise OptimizationError("unknown registered Harbor Case")
        return (self.cases[case_id].path / "instruction.md").read_text(encoding="utf-8")

    def probe(self, case_id: str, candidate: Candidate, timeout: float, cancelled=None) -> Trial:
        return asyncio.run(cancellable(self._probe(case_id, candidate, timeout), cancelled))

    async def _probe(self, case_id, candidate, timeout):
        from harbor.job import Job
        from harbor.models.job.config import JobConfig
        from harbor.models.trial.config import AgentConfig, EnvironmentConfig, TaskConfig

        from ..codex_artifacts import read_native
        from .harbor_agent import CREDENTIALS

        started = time.monotonic()
        case = self.cases[case_id]
        if task_digest(case.path) != case.sha256:
            raise OptimizationError("Harbor Case changed before execution")
        trial_id = uuid.uuid4().hex
        # No default/shared provider: use the owner model, pinned runtime and low effort.
        CREDENTIALS[trial_id] = {"api_key": self.model.api_key, "base_url": self.model.base_url}
        config = JobConfig(
            job_name=trial_id,
            jobs_dir=self.output,
            n_concurrent_trials=1,
            n_attempts=1,
            quiet=True,
            environment=EnvironmentConfig(type="docker", delete=True, override_cpus=1, override_memory_mb=1024),
            agents=[
                AgentConfig(
                    import_path="catena_engine.evaluation.harbor_agent:RSICodex",
                    model_name=self.model.model,
                    kwargs={
                        "credential_ref": trial_id,
                        "harness_files": dict(candidate.files),
                        "version": "0.160.0",
                        "reasoning_effort": "low",
                        "web_search": "disabled",
                    },
                )
            ],
            tasks=[TaskConfig(path=case.path)],
        )
        try:
            async with asyncio.timeout(timeout):
                job = await Job.create(config)
                await job.run()
            if task_digest(case.path) != case.sha256:
                raise OptimizationError("Harbor Case changed during execution")
            results = list((self.output / trial_id).glob("*/result.json"))
            if len(results) != 1:
                raise OptimizationError("Harbor did not produce exactly one trial")
            data = json.loads(results[0].read_text())
            if data.get("exception_info"):
                raise OptimizationError("Harbor trial is invalid")
            calls, answer = read_native(results[0].parent / "agent")
            agent = data.get("agent_result") or {}
            rewards = (data.get("verifier_result") or {}).get("rewards") or {}
            return Trial(
                safe_id(trial_id),
                case_id,
                candidate.id,
                json.dumps(calls, ensure_ascii=False),
                answer,
                bool(rewards) and bool(answer.strip()),
                rewards,
                agent.get("n_input_tokens"),
                agent.get("n_output_tokens"),
                time.monotonic() - started,
            )
        finally:
            CREDENTIALS.pop(trial_id, None)
