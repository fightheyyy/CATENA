"""Export a completed native Codex comparison from independently checked artifacts."""

from __future__ import annotations

import argparse
import hashlib
import json
from importlib.metadata import version
from pathlib import Path

from .codex_artifacts import read_native
from .harbor_worker import call_metrics


def publish(root: Path, target: Path) -> dict:
    state = json.loads((root / "catena-result.json").read_text(encoding="utf-8"))
    if state.get("target_agent") != "codex" or state["status"] != "completed" or not state["tasks_unchanged"]:
        raise ValueError("completed native Codex study required")
    trials = state["trials"]
    if len(trials) != 6 or len({(t["variant"], t["task"], t["attempt"]) for t in trials}) != 6:
        raise ValueError("six distinct trials required")
    for relative, digest in state["task_sha256"].items():
        if hashlib.sha256((root / relative).read_bytes()).hexdigest() != digest:
            raise ValueError("frozen task changed")
    evidence = []
    for t in trials:
        if not (t["valid"] and t["native_session_verified"] and t["environment_verified"]):
            raise ValueError("native evidence incomplete")
        trial_root = root / "jobs" / t["job_directory"] / t["trial_name"]
        agent = trial_root / "agent"
        calls, answer = read_native(agent)
        metrics = call_metrics(calls)
        if any(t[k] != v for k, v in metrics.items()):
            raise ValueError("native command metrics mismatch")
        probe = json.loads((trial_root / "verifier/environment.json").read_text(encoding="utf-8-sig"))
        if not probe["rg_absent"] or probe["codex_version"] != "codex-cli 0.160.0":
            raise ValueError("wrong environment")
        reward = float((trial_root / "verifier/reward.txt").read_text().strip())
        if reward != t["rewards"]["reward"]:
            raise ValueError("independent verifier reward mismatch")
        events = []
        for line in (agent / "codex.txt").read_text(encoding="utf-8").splitlines():
            try:
                events.append(json.loads(line))
            except ValueError:
                continue
        completions = [e for e in events if e.get("type") == "turn.completed"]
        if len(completions) != 1:
            raise ValueError("one completed CLI turn required")
        usage = completions[0]["usage"]
        if usage["input_tokens"] != t["input_tokens"] or usage["output_tokens"] != t["output_tokens"]:
            raise ValueError("CLI usage mismatch")
        evidence.append({
            **{k: t[k] for k in ("variant", "task", "attempt", "valid", "rewards", "input_tokens",
                                "output_tokens", "duration_seconds", "native_session_verified", "environment_verified")},
            **metrics,
            "cached_input_tokens": usage.get("cached_input_tokens", 0),
            "nonzero_command_exits": sum(c["exit_code"] is not None and c["exit_code"] != 0 for c in calls),
            "environment_probe": probe,
            "native_session_sha256": hashlib.sha256(next((agent / "sessions").rglob("*.jsonl")).read_bytes()).hexdigest(),
            "tool_calls": calls,
            "answer": answer,
        })
    totals = {}
    for arm in ("baseline", "candidate"):
        arm_trials = [t for t in evidence if t["variant"] == arm]
        if len(arm_trials) != 3:
            raise ValueError("three trials per arm required")
        totals[arm] = {
            "valid": 3,
            "passed": sum(t["rewards"]["reward"] == 1 for t in arm_trials),
            **{k: sum(t[k] for t in arm_trials) for k in ("missing_rg_errors", "command_count", "input_tokens",
                "output_tokens", "cached_input_tokens", "nonzero_command_exits", "duration_seconds")},
        }
    result = {
        "schema": "catena.native_codex_study.v1", "date": "2026-10-04",
        "experiment_id": state["request_id"], "job_id": state["job_id"], "model": state["model"],
        "target": "Harbor built-in Codex adapter / Codex CLI 0.160.0",
        "harbor_version": version("harbor"), "status": "completed", "environment": state["environment"],
        "attempts_per_task_per_arm": 1, "tasks_unchanged": True,
        "task_sha256": {k.replace("\\", "/"): v for k, v in state["task_sha256"].items()},
        "skill_sha256": state["skill_sha256"], "summaries": totals, "trials": evidence,
        "limitations": [
            "Three anonymous Linux/PowerShell fixtures, not an original Windows session replay.",
            "The native Agent runs as a non-root user with read-only repository fixtures.",
            "Skill is installed through Harbor and explicitly requested; automatic discovery benefit is not tested.",
            "One attempt per task per arm; no statistical significance or general success-rate claim.",
            "Input tokens include cached input. These are target usage counts, not billing amounts.",
            "Execution time excludes container builds and Catena coordinator usage.",
            "Claude Code is not evaluated in this experiment.",
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
    print(json.dumps(publish(repo / ".local/harbor-study/runs" / args.job_id,
                            repo / "evals/rg-missing-search/results/codex-gpt55-20261004.json")))


if __name__ == "__main__":
    main()
