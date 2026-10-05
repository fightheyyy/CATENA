"""Paired SDK evaluation in frozen read-only PowerShell fixtures."""
from __future__ import annotations

import argparse
import asyncio
import hashlib
import importlib.metadata
import json
import os
import shutil
import subprocess
import time
from datetime import datetime, timezone
from pathlib import Path

from agents import (
    Agent,
    ModelSettings,
    OpenAIChatCompletionsModel,
    RunConfig,
    Runner,
    function_tool,
    set_tracing_disabled,
)
from openai import AsyncOpenAI

set_tracing_disabled(True)


def digest(path: Path) -> str:
    result = hashlib.sha256()
    files = sorted(path.rglob("*")) if path.is_dir() else [path]
    for item in files:
        if item.is_file():
            result.update((item.relative_to(path).as_posix() if path.is_dir() else item.name).encode())
            result.update(item.read_bytes())
    return result.hexdigest()


def execute(command: str, workspace: Path, wrapper: Path, path: str) -> dict:
    env = os.environ.copy()
    env["PATH"] = path
    shell = Path(os.environ["SystemRoot"]) / "System32/WindowsPowerShell/v1.0/powershell.exe"
    started = time.monotonic()
    result = subprocess.run([str(shell), "-NoProfile", "-NonInteractive", "-File", str(wrapper), "-Command", command],
                            cwd=workspace, env=env, capture_output=True, text=True,
                            encoding="utf-8", errors="replace", timeout=20)
    return {"command": command, "exit_code": result.returncode, "stdout": result.stdout[:24000],
            "stderr": result.stderr[:6000], "elapsed_seconds": round(time.monotonic() - started, 3)}


async def evaluate(manifest_path: Path, output: Path, base_url: str, api_key: str) -> dict:
    root = manifest_path.resolve().parent
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if output.exists() and any(output.iterdir()):
        raise ValueError("output directory must be empty")
    output.mkdir(parents=True, exist_ok=True)
    wrapper = root / "read_only_shell.ps1"
    verifier = root / "verify.py"
    skill = root / manifest["skill"] / "SKILL.md"
    frozen = {str(p): digest(p) for p in [manifest_path, wrapper, verifier, skill]}
    tool_path = manifest["shell_environment"]["PATH"]
    # Verify the exact tool wrapper used by the model, not just a separate shell.
    probe = execute("Get-Command rg -ErrorAction SilentlyContinue", root / manifest["cases"][0]["fixture"], wrapper, tool_path)
    if probe["stdout"].strip() or "EVAL_POLICY" in probe["stderr"]:
        raise RuntimeError("target tool environment does not satisfy the absent-rg precondition")
    report = {"schema": "catena.sdk_skill_experiment.v1", "created_at": datetime.now(timezone.utc).isoformat(),
              "runtime": "OpenAI Agents SDK", "sdk_version": importlib.metadata.version("openai-agents"),
              "model": manifest["model"], "base_url": base_url, "activation": "explicit_skill_instructions",
              "environment_probe": probe, "frozen_sha256": frozen, "runs": []}
    (output / "manifest.snapshot.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
    client = AsyncOpenAI(base_url=base_url, api_key=api_key, timeout=90, max_retries=0)
    model = OpenAIChatCompletionsModel(model=manifest["model"], openai_client=client)
    try:
        for case in manifest["cases"]:
            fixture = root / case["fixture"]
            fixture_hash = digest(fixture)
            for repeat in range(manifest["repeats"]):
                for variant in (("baseline", "candidate") if repeat % 2 == 0 else ("candidate", "baseline")):
                    run_dir = output / case["id"] / f"repeat-{repeat + 1}" / variant
                    workspace = run_dir / "workspace"
                    shutil.copytree(fixture, workspace)
                    calls = []

                    @function_tool
                    async def shell(command: str) -> str:
                        """Run read-only literal PowerShell command pipelines in the current repository.

                        Available command names: rg, Get-ChildItem, Get-Content, Get-Command,
                        Select-String, Select-Object, Format-List, ConvertTo-Json.
                        Use relative paths. Scripts, variables, expressions, writes and other
                        executables are outside this tool's scope.
                        """
                        record = await asyncio.to_thread(execute, command, workspace, wrapper, tool_path)
                        calls.append(record)
                        return json.dumps(record, ensure_ascii=False)

                    instructions = "Inspect this read-only repository using the shell tool. Ground your answer in files. Return only the JSON requested by the user."
                    if variant == "candidate":
                        instructions += "\nApply this repository-search Skill:\n" + skill.read_text(encoding="utf-8")
                    agent = Agent(name="CatenaCaseAgent", instructions=instructions, model=model, tools=[shell],
                                  model_settings=ModelSettings(parallel_tool_calls=False))
                    started = time.monotonic()
                    final = ""
                    usage = {}
                    error = ""
                    try:
                        result = await asyncio.wait_for(Runner.run(agent, case["prompt"], max_turns=10,
                            run_config=RunConfig(tracing_disabled=True)), timeout=manifest["agent_timeout_seconds"])
                        final = result.final_output if isinstance(result.final_output, str) else ""
                        u = result.context_wrapper.usage
                        usage = {"requests": u.requests, "input_tokens": u.input_tokens, "output_tokens": u.output_tokens,
                                 "cached_input_tokens": u.input_tokens_details.cached_tokens}
                        (run_dir / "messages.json").write_text(json.dumps(result.to_input_list(), ensure_ascii=False,
                            indent=2, default=str), encoding="utf-8")
                    except Exception as exc:
                        error = (type(exc).__name__ + ": " + str(exc)).replace(api_key, "[redacted]")[:600]
                    (run_dir / "agent.final.txt").write_text(final, encoding="utf-8")
                    unchanged = all(digest(Path(p)) == h for p, h in frozen.items()) and digest(workspace) == fixture_hash
                    check = subprocess.run([os.sys.executable, str(verifier), case["id"]], cwd=workspace,
                        capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=20) if not error and unchanged else None
                    denied = sum("EVAL_POLICY" in c["stderr"] for c in calls)
                    row = {"case_id": case["id"], "repeat": repeat + 1, "variant": variant,
                           "fixture_sha256": fixture_hash, "prompt_sha256": hashlib.sha256(case["prompt"].encode()).hexdigest(),
                           "passed": bool(check and check.returncode == 0), "valid_for_comparison": not error and unchanged and not denied,
                           "error": error, "verifier_unchanged": unchanged, "policy_rejections": denied,
                           "missing_rg_errors": sum("CommandNotFoundException" in c["stderr"] and "rg" in c["stderr"] for c in calls),
                           "command_count": len(calls), "failed_commands": sum(c["exit_code"] != 0 for c in calls),
                           "elapsed_seconds": round(time.monotonic() - started, 3), "usage": usage,
                           "tool_calls": calls, "verifier_exit_code": check.returncode if check else None}
                    report["runs"].append(row)
                    (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
                    print(f"{case['id']} repeat {repeat + 1} {variant}: pass={row['passed']} valid={row['valid_for_comparison']} rg_errors={row['missing_rg_errors']}", flush=True)
                    if error or denied or not unchanged:
                        report["status"] = "stopped_invalid_trial"
                        (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
                        raise RuntimeError("trial preconditions failed; see private report")
    finally:
        await client.close()
    for variant in ("baseline", "candidate"):
        rows = [r for r in report["runs"] if r["variant"] == variant]
        report[variant] = {"passed": sum(r["passed"] for r in rows), "total": len(rows),
                          "valid_trials": sum(r["valid_for_comparison"] for r in rows),
                          "missing_rg_errors": sum(r["missing_rg_errors"] for r in rows),
                          "command_count": sum(r["command_count"] for r in rows),
                          "elapsed_seconds": round(sum(r["elapsed_seconds"] for r in rows), 3),
                          "input_tokens": sum(r["usage"].get("input_tokens", 0) for r in rows),
                          "output_tokens": sum(r["usage"].get("output_tokens", 0) for r in rows)}
    report["status"] = "completed"
    (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    return report


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--base-url", default="http://127.0.0.1:8317/v1")
    parser.add_argument("--api-key-file", type=Path, required=True)
    args = parser.parse_args()
    report = asyncio.run(evaluate(args.manifest.resolve(), args.output.resolve(), args.base_url,
                                 args.api_key_file.read_text(encoding="utf-8").strip()))
    print(json.dumps({v: report[v] for v in ("baseline", "candidate")}))


if __name__ == "__main__":
    main()
