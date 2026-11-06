"""Read Harbor's ATIF trajectory and declared, pre-verifier artifact manifest."""

import json
from pathlib import Path

from harbor.models.trajectories.trajectory import Trajectory
from harbor.models.trial.artifact_manifest import ArtifactManifestEntry

from ..contracts import OptimizationError


def text(content) -> str:
    if isinstance(content, str):
        return content
    return "\n".join(p.text for p in (content or []) if p.type == "text" and p.text)


def read_trajectory(path: Path) -> tuple[str, str, int]:
    trajectory = Trajectory.model_validate_json(path.read_text(encoding="utf-8"))
    if (
        trajectory.continued_trajectory_ref
        or trajectory.subagent_trajectories
        or any(r.subagent_trajectory_ref for s in trajectory.steps if s.observation for r in s.observation.results)
    ):
        raise OptimizationError("CATENA requires a single complete Harbor trajectory")
    answer = next((text(s.message) for s in reversed(trajectory.steps) if s.source == "agent" and not s.tool_calls), "")
    return trajectory.model_dump_json(), answer, sum(len(s.tool_calls or []) for s in trajectory.steps)


def render_trajectory(body: str, obs_trunc: int) -> str:
    trajectory = Trajectory.model_validate_json(body)
    lines = []
    for step in trajectory.steps:
        if step.source != "agent":
            continue
        if text(step.message):
            lines.append("[ACTION] " + text(step.message))
        for call in step.tool_calls or []:
            lines.append("[ACTION] " + call.function_name + " " + json.dumps(call.arguments, ensure_ascii=False))
        for observation in step.observation.results if step.observation else []:
            body = text(observation.content)
            lines.append("[OBSERVATION] " + body[:obs_trunc] + ("\n[truncated]" if len(body) > obs_trunc else ""))
    return "\n".join(lines)


def read_deliverables(root: Path, sources: tuple[str, ...]) -> dict[str, str]:
    entries = [ArtifactManifestEntry.model_validate(item) for item in json.loads((root / "manifest.json").read_text())]
    files, total = {}, 0
    for source in sources:
        matched = [e for e in entries if e.source == source and e.service in (None, "main") and e.status != "skipped"]
        if len(matched) != 1:
            raise OptimizationError("Harbor did not record exactly one collection attempt per deliverable")
        entry = matched[0]
        if entry.status != "ok":
            continue  # Missing output is an observed non-delivery, never a fabricated answer file.
        path = root.parent / entry.destination
        if (
            entry.type != "file"
            or path.is_symlink()
            or not path.resolve().is_relative_to(root.resolve())
            or not path.is_file()
        ):
            raise OptimizationError("Harbor deliverable is not a confined regular text file")
        total += path.stat().st_size
        if total > 256_000:
            raise OptimizationError("Harbor deliverables exceed optimizer context limit")
        files[source] = path.read_bytes().decode("utf-8")
    return files
