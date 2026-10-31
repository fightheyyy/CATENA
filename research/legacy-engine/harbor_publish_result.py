"""Export only experiment evidence and aggregate metrics, never credentials."""
from __future__ import annotations

import argparse
import json
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("job_id")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    root = repo / ".local/harbor-study"
    state = json.loads((root / "runs" / args.job_id / "catena-result.json").read_text(encoding="utf-8"))
    final = json.loads((root / "platform-response.json").read_text(encoding="utf-8"))["result"]
    initial = json.loads((root / "platform-response-before-stream-fix.json").read_text(encoding="utf-8"))["result"]
    if state["status"] != "completed" or final["status"] != "completed" or final["experiment"]["job_id"] != args.job_id:
        raise ValueError("completed, reconciled platform result required")
    summaries = {}
    for variant in ("baseline", "candidate"):
        rows = [t for t in state["trials"] if t["variant"] == variant]
        summaries[variant] = {"total": len(rows), "valid": sum(t["valid"] for t in rows),
            "passed": sum(t["valid"] and t["rewards"].get("reward") == 1 for t in rows),
            **{key: sum(t[key] for t in rows) for key in ("command_count", "missing_rg_errors", "policy_rejections", "input_tokens", "output_tokens")}}
    public = {"schema": "catena.harbor_experiment.v1", "date": "2026-10-03", "job_id": args.job_id,
              "runtime": "OpenAI Agents SDK 0.22.3", "model": "gpt-5.5 via local CLIProxyAPI",
              "target": "catena-sdk-search 1.0.0; not Codex CLI", "harbor_version": "0.23.0",
              "harbor_commit": "3b287b5cc2f0f30745eec73cc1ace578cbfae4c5", "worker_openai_version": "2.54.0",
              "environment": state["environment"], "verifier_mode": "shared",
              "image": "mcr.microsoft.com/powershell@sha256:62300a213a9293916333df2b014cd3a8f22fb0b0b65f2bb446aaf436bcf8c868",
              "status": state["status"], "tasks_unchanged": state["tasks_unchanged"],
              "activation": "explicit extra instruction; candidate only", "attempts_per_case_per_arm": 1,
              "summaries": summaries, "trials": state["trials"],
              "task_sha256": {k.replace("\\", "/"): v for k, v in state["task_sha256"].items()},
              "platform_submission_tool_events": initial["tool_events"],
              "platform_retrieval_tool_events": final["tool_events"],
              "platform_summary": json.loads(final["assistant"]["content"]),
              "notes": ["SDK model loop runs in host worker; filesystem and command tools run in Harbor containers.",
                        "Linux PowerShell 7 adaptation; not original Windows replay.",
                        "Metrics reconciled from raw tool logs because Docker merged stderr into stdout.",
                        "Re-reading the completed job did not execute new trials.",
                        "Six trials only; no statistical significance or overall success-rate gain established.",
                        "Private engine endpoint; not yet public Go API, frontend Case UI, or Postgres Experiment."]}
    target = repo / "evals/rg-missing-search/results/harbor-gpt55-20261003.json"
    target.write_text(json.dumps(public, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(summaries))


if __name__ == "__main__":
    main()
