"""Publish this frozen anonymous study from actual Harbor verifier and tool artifacts."""

from __future__ import annotations

import argparse
import hashlib
import json
from importlib.metadata import version
from pathlib import Path


def publish(root: Path, target: Path) -> dict:
    state = json.loads((root / "catena-result.json").read_text(encoding="utf-8"))
    if state.get("study") != "memory_skill" or state["status"] != "completed" or not state.get("tasks_unchanged"):
        raise ValueError("completed frozen three-arm study is required")
    if len(state["trials"]) != 18 or not all(t["valid"] for t in state["trials"]):
        raise ValueError("18 valid trials are required")
    for relative, digest in state["task_sha256"].items():
        if hashlib.sha256((root / relative).read_bytes()).hexdigest() != digest:
            raise ValueError("frozen task changed")
    evidence = []
    totals = {}
    for arm in state["arms"]:
        trials = [t for t in state["trials"] if t["variant"] == arm]
        identities = {(t["task"], t["attempt"]) for t in trials}
        if len(trials) != 6 or len(identities) != 6:
            raise ValueError("missing or duplicate trial")
        totals[arm] = {
            "valid": len(trials),
            "passed": sum(t["rewards"].get("reward") == 1 for t in trials),
            **{
                key: sum(t[key] for t in trials)
                for key in ("missing_rg_errors", "command_count", "input_tokens", "output_tokens", "duration_seconds")
            },
        }
    for t in state["trials"]:
        trial_root = root / "jobs" / t["job_directory"] / t["trial_name"]
        agent = trial_root / "agent"
        reward = float((trial_root / "verifier/reward.txt").read_text().strip())
        if reward != t["rewards"]["reward"]:
            raise ValueError("saved verifier disagrees with trial reward")
        calls = json.loads((agent / "tool-calls.json").read_text(encoding="utf-8"))
        if len(calls) != t["command_count"]:
            raise ValueError("tool call count mismatch")
        evidence.append(
            {
                **{
                    key: t[key]
                    for key in (
                        "variant",
                        "task",
                        "attempt",
                        "valid",
                        "rewards",
                        "missing_rg_errors",
                        "command_count",
                        "input_tokens",
                        "output_tokens",
                        "duration_seconds",
                    )
                },
                "tool_calls": calls,
                "answer": (agent / "agent.final.txt").read_text(encoding="utf-8"),
            }
        )
    result = {
        "schema": "catena.memory_skill_study.v1",
        "date": "2026-10-04",
        "job_id": state["job_id"],
        "request_id": state["request_id"],
        "model": state["model"],
        "runtime_versions": {name: version(name) for name in ("openai-agents", "openai", "harbor")},
        "memory_query": "PowerShell 搜索缺少 rg 历史错误恢复",
        "status": "completed",
        "target": "OpenAI Agents SDK search Agent; not Codex CLI or Claude Code",
        "environment": state["environment"],
        "attempts_per_task_per_arm": 2,
        "tasks_unchanged": True,
        "task_sha256": {k.replace("\\", "/"): v for k, v in state["task_sha256"].items()},
        "skill_sha256": state["skill_sha256"],
        "memory_sha256": state["memory_sha256"],
        "memory_context": state["memory_context"],
        "summaries": totals,
        "trials": evidence,
        "limitations": [
            "Three anonymous reconstructed search tasks, not original Windows replay.",
            "Memory is a manually verified historical observation retrieved from OpenViking; no task answers.",
            "Interventions are explicit: memory only or Skill only; no automatic Skill discovery tested.",
            "Two repetitions per task per arm; no statistical significance or general success-rate claim.",
            "Duration includes model and search tools; excludes container build, retrieval and coordinator.",
            "Token totals are target usage, not billing amounts or coordinator usage.",
        ],
    }
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    return totals


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("job_id")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    print(
        json.dumps(
            publish(
                repo / ".local/harbor-study/runs" / args.job_id,
                repo / "evals/rg-missing-search/results/memory-skill-gpt55-20261004.json",
            )
        )
    )


if __name__ == "__main__":
    main()
