"""Read native Codex rollout evidence without replacing Harbor's installed adapter."""

import json
from pathlib import Path


def read_native(logs: Path) -> tuple[list, str]:
    sessions = sorted((logs / "sessions").rglob("*.jsonl"))
    if len(sessions) != 1:
        raise ValueError("exactly one native Codex session is required")
    calls, pending, answer = [], {}, ""
    for line in sessions[0].read_text(encoding="utf-8").splitlines():
        item = json.loads(line)
        p = item.get("payload", {})
        if item.get("type") != "response_item":
            continue
        if p.get("type") in ("function_call", "custom_tool_call"):
            raw = p.get("arguments", p.get("input", ""))
            try:
                args = json.loads(raw)
            except (ValueError, TypeError):
                args = {}
            command = args.get("cmd", args.get("command")) if isinstance(args, dict) else None
            pending[p["call_id"]] = {"tool": p.get("name", ""), "input": raw, "command": command or raw}
        elif p.get("type") in ("function_call_output", "custom_tool_call_output") and p.get("call_id") in pending:
            output = p.get("output", "")
            if not isinstance(output, str):
                output = json.dumps(output)
            import re

            exit_match = re.search(r"(?:Process exited with code|Exit code:)\s*(-?\d+)", output)
            calls.append(
                {
                    **pending.pop(p["call_id"]),
                    "exit_code": int(exit_match[1]) if exit_match else None,
                    "stdout": output,
                    "stderr": "",
                }
            )
        elif p.get("type") == "message" and p.get("role") == "assistant" and p.get("phase") != "commentary":
            answer = (
                "".join(c.get("text", "") for c in p.get("content", []) if c.get("type") == "output_text") or answer
            )
    if not answer:
        raise ValueError("native Codex final answer is missing")
    return calls, answer


def normalize(logs: Path) -> dict:
    calls, answer = read_native(logs)
    (logs / "tool-calls.json").write_text(json.dumps(calls, ensure_ascii=False, indent=2), encoding="utf-8")
    (logs / "agent.final.txt").write_text(answer, encoding="utf-8")
    return {"native_session_verified": True, "native_command_count": len(calls)}
