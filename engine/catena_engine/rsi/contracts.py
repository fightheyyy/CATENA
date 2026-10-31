"""Versioned RSI contracts, independent of algorithms, runtimes and evaluators."""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass, field
from types import MappingProxyType
from typing import Any, Protocol

SCHEMA = "catena.rsi_request.v1"
RESULT_SCHEMA = "catena.rsi_result.v1"
_SAFE_ID = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,127}\Z")
_SKILL = re.compile(r"\.agents/skills/[a-z0-9][a-z0-9_-]*/SKILL\.md\Z")


class OptimizationError(ValueError):
    """Safe, credential-free operational error."""


def safe_id(value: Any) -> str:
    if not isinstance(value, str) or not _SAFE_ID.fullmatch(value):
        raise OptimizationError("a safe identifier is required")
    return value


def validate_files(files: dict[str, str]) -> dict[str, str]:
    if not isinstance(files, dict) or not 1 <= len(files) <= 32:
        raise OptimizationError("harness must contain 1–32 files")
    if not isinstance(files.get("AGENTS.md"), str) or not files["AGENTS.md"].strip():
        raise OptimizationError("nonempty AGENTS.md is required")
    for path, content in files.items():
        if not isinstance(path, str) or not (path == "AGENTS.md" or _SKILL.fullmatch(path)):
            raise OptimizationError("only AGENTS.md and .agents/skills/<name>/SKILL.md may be edited")
        if not isinstance(content, str) or len(content.encode()) > 32_768:
            raise OptimizationError("harness files must be text of at most 32 KiB each")
    if sum(len(v.encode()) for v in files.values()) > 128 * 1024:
        raise OptimizationError("harness exceeds 128 KiB")
    return dict(sorted(files.items()))


@dataclass(frozen=True)
class Candidate:
    files: dict[str, str]
    parents: tuple[str, ...] = ()
    algorithm: str = "baseline"
    notes: str = ""
    id: str = field(init=False)
    content_sha256: str = field(init=False)

    def __post_init__(self):
        files = validate_files(self.files)
        object.__setattr__(self, "files", MappingProxyType(files))
        content = hashlib.sha256(json.dumps(files, ensure_ascii=False, sort_keys=True).encode()).hexdigest()
        object.__setattr__(self, "content_sha256", content)
        digest = hashlib.sha256(json.dumps([content, self.parents, self.algorithm]).encode()).hexdigest()
        object.__setattr__(self, "id", digest)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "content_sha256": self.content_sha256,
            "parents": list(self.parents),
            "algorithm": self.algorithm,
            "notes": self.notes,
            "files": dict(self.files),
            "status": "unverified",
        }


@dataclass(frozen=True)
class Budget:
    model_calls: int = 12
    trials: int = 8
    seconds: int = 900

    def __post_init__(self):
        for value, limit in ((self.model_calls, 64), (self.trials, 64), (self.seconds, 3600)):
            if type(value) is not int or not 1 <= value <= limit:
                raise OptimizationError("budget requires positive model_calls/trials ≤64 and seconds ≤3600")


@dataclass(frozen=True)
class Algorithm:
    name: str
    options: dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True)
class Requirements:
    model_calls: int
    trials: int

    def __post_init__(self):
        if any(type(n) is not int or not 0 <= n <= 64 for n in (self.model_calls, self.trials)):
            raise OptimizationError("invalid adapter budget requirements")


@dataclass(frozen=True)
class Context:
    request_id: str
    case_id: str
    baseline: Candidate
    evidence: str = ""
    evidence_refs: tuple[str, ...] = ()


@dataclass(frozen=True)
class Completion:
    value: dict[str, Any]
    input_tokens: int
    output_tokens: int


@dataclass(frozen=True)
class Trial:
    id: str
    case_id: str
    candidate_id: str
    trajectory: str
    answer: str
    valid: bool
    rewards: dict[str, float] = field(default_factory=dict)
    input_tokens: int | None = None
    output_tokens: int | None = None
    seconds: float | None = None
    phase: str = "probe"


class OptimizationServices(Protocol):
    """Only host-mediated model calls and probes; no arbitrary paths or graders."""

    def complete(self, prompt: str) -> Completion: ...

    def probe(self, candidate: Candidate) -> Trial: ...

    def task_prompt(self) -> str: ...


@dataclass(frozen=True)
class OptimizationResult:
    candidates: tuple[Candidate, ...]
    diagnostics: dict[str, Any] = field(default_factory=dict)


class RSIAdapter(Protocol):
    name: str
    version: str

    def validate(self, options: dict[str, Any]) -> None: ...

    def requirements(self, options: dict[str, Any]) -> Requirements: ...

    def optimize(
        self, context: Context, services: OptimizationServices, options: dict[str, Any]
    ) -> OptimizationResult: ...
