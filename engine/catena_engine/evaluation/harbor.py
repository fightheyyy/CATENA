"""Evaluator adapter for administrator-registered Harbor tasks (Docker only)."""

from __future__ import annotations

import asyncio
import hashlib
import os
import shutil
import subprocess
import traceback
import uuid
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from urllib.parse import unquote, urlparse

from ..contracts import Candidate, OptimizationError, Trial, safe_id
from ..protocol import ModelConfig
from ..runtime.cancellation import cancellable


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
    artifacts: tuple[str, ...]

    @classmethod
    def load(cls, path: str, sha256: str, artifacts=None):
        from harbor.models.task.artifacts import normalize_artifact_entries
        from harbor.models.task.config import TaskConfig as HarborTaskConfig

        root = Path(path).resolve(strict=True)
        if not root.is_dir() or any(not (root / name).is_file() for name in ("instruction.md", "task.toml")):
            raise OptimizationError("registered Harbor task is incomplete")
        if task_digest(root) != sha256:
            raise OptimizationError("registered Harbor task fingerprint differs from its manifest")
        config = HarborTaskConfig.model_validate_toml((root / "task.toml").read_text())
        if config.steps:
            raise OptimizationError("CATENA currently supports single-step Harbor Cases only")
        declared = normalize_artifact_entries(config.artifacts)
        if any(a.service not in (None, "main") or a.exclude for a in declared):
            raise OptimizationError("CATENA deliverables must be text files from the main Harbor service")
        extras = artifacts if artifacts is not None else []
        if not isinstance(extras, list) or any(not isinstance(p, str) for p in extras):
            raise OptimizationError("Case artifacts must be absolute container file paths")
        sources = tuple(dict.fromkeys([a.source for a in declared] + extras))
        if not 1 <= len(sources) <= 32 or any(
            not PurePosixPath(p).is_absolute()
            or len(p) > 1024
            or "\0" in p
            or ".." in PurePosixPath(p).parts
            or p.startswith(("/tests/", "/solution/", "/logs/"))
            or p in ("/", "/tests", "/solution", "/logs")
            for p in sources
        ):
            raise OptimizationError("Case requires 1–32 explicit deliverable files outside grader and log paths")
        return cls(root, sha256, sources)


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
        from harbor.models.task.artifacts import normalize_artifact_entries
        from harbor.models.task.config import TaskConfig as HarborTaskConfig
        from harbor.models.trial.config import AgentConfig, EnvironmentConfig, TaskConfig
        from harbor.models.trial.paths import TrialPaths

        from .evidence import read_deliverables, read_trajectory
        from .harbor_agent import CREDENTIALS

        case = self.cases[case_id]
        if task_digest(case.path) != case.sha256:
            raise OptimizationError("Harbor Case changed before execution")
        trial_id = uuid.uuid4().hex
        declared_sources = {
            a.source
            for a in normalize_artifact_entries(
                HarborTaskConfig.model_validate_toml((case.path / "task.toml").read_text()).artifacts
            )
        }
        # No default/shared provider: use the owner model, pinned runtime and low effort.
        config = JobConfig(
            job_name=trial_id,
            jobs_dir=self.output,
            n_concurrent_trials=1,
            n_attempts=1,
            quiet=True,
            artifacts=[p for p in case.artifacts if p not in declared_sources],
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
        CREDENTIALS[trial_id] = {"api_key": self.model.api_key, "base_url": self.model.base_url}
        try:
            async with asyncio.timeout(timeout):
                job = await Job.create(config)
                result = await job.run()
            if task_digest(case.path) != case.sha256:
                raise OptimizationError("Harbor Case changed during execution")
            if len(result.trial_results) != 1:
                raise OptimizationError("Harbor did not produce exactly one trial")
            native = result.trial_results[0]
            uri = urlparse(native.trial_uri)
            if uri.scheme != "file" or uri.netloc:
                raise OptimizationError("Harbor trial must have local artifacts")
            directory = Path(unquote(uri.path)).resolve()
            if not directory.is_relative_to((self.output / trial_id).resolve()):
                raise OptimizationError("Harbor artifact path escaped its job")
            paths = TrialPaths(directory)
            if native.exception_info:
                raise OptimizationError("Harbor trial is invalid")
            trajectory, answer, tool_calls = read_trajectory(paths.agent_dir / "trajectory.json")
            deliverables = read_deliverables(paths.artifacts_dir, case.artifacts)
            rewards = native.verifier_result.rewards if native.verifier_result else {}
            n_input, _, n_output, cost = native.compute_token_cost_totals()
            return Trial(
                safe_id(trial_id),
                case_id,
                candidate.id,
                trajectory,
                answer,
                bool(rewards),
                rewards or {},
                n_input,
                n_output,
                (native.finished_at - native.started_at).total_seconds()
                if native.finished_at and native.started_at
                else None,
                artifact_dir=directory.relative_to(self.output).as_posix(),
                cost_usd=cost,
                tool_calls=tool_calls,
                # ATIF does not promise per-shell exit codes, especially inside custom tools.
                nonzero_command_exits=None,
                result_sha256=hashlib.sha256(paths.result_path.read_bytes()).hexdigest(),
                deliverables=deliverables,
            )
        except Exception:
            directory = self.output / trial_id
            try:
                directory.mkdir(parents=True, exist_ok=True)
                with os.fdopen(
                    os.open(directory / "private-error.txt", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w"
                ) as log:
                    log.write(traceback.format_exc().replace(self.model.api_key, "[REDACTED]"))
            except OSError:
                pass  # Diagnostic failure must not replace the original execution error.
            raise
        finally:
            CREDENTIALS.pop(trial_id, None)
