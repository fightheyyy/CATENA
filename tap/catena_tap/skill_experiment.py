"""Paired, local Codex Skill experiment with deterministic outcome checks.

The manifest and fixtures must be reviewed before running an agent. Raw traces
remain local because they can contain prompts, source files, and tool output.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import signal
import subprocess
import tempfile
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SCHEMA = "catena.skill_experiment.v1"


def _digest(path: Path) -> str:
    digest = hashlib.sha256()
    if path.is_file():
        digest.update(path.read_bytes())
    else:
        for item in sorted(path.rglob("*")):
            if item.is_file():
                digest.update(item.relative_to(path).as_posix().encode())
                digest.update(item.read_bytes())
    return digest.hexdigest()


def _relative_path(root: Path, value: str, *, directory: bool) -> Path:
    path = (root / value).resolve()
    if not path.is_relative_to(root.resolve()):
        raise ValueError(f"path escapes manifest directory: {value}")
    exists = path.is_dir() if directory else path.is_file()
    if not exists:
        raise ValueError(f"missing {'directory' if directory else 'file'}: {value}")
    return path


def load_manifest(path: Path) -> dict[str, Any]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if data.get("schema_version") != SCHEMA:
        raise ValueError("unsupported experiment schema")
    if not isinstance(data.get("model"), str) or not data["model"].strip():
        raise ValueError("model must be pinned")
    name = data.get("skill_name")
    if not isinstance(name, str) or not name or not all(c.isalnum() or c == "-" for c in name):
        raise ValueError("skill_name must contain only letters, digits, or hyphens")
    root = path.resolve().parent
    shell_environment = data.get("shell_environment", {})
    if not isinstance(shell_environment, dict) or any(
        key != "PATH" or not isinstance(value, str) or not value
        for key, value in shell_environment.items()
    ):
        raise ValueError("shell_environment accepts only a nonempty PATH string")
    if data.get("output_schema"):
        schema = _relative_path(root, data["output_schema"], directory=False)
        json.loads(schema.read_text(encoding="utf-8"))
    verifier_files = data.get("verifier_files", [])
    if not isinstance(verifier_files, list) or any(not isinstance(item, str) for item in verifier_files):
        raise ValueError("verifier_files must be a list of relative files")
    for item in verifier_files:
        _relative_path(root, item, directory=False)
    skill = _relative_path(root, data.get("skill", ""), directory=True)
    if not (skill / "SKILL.md").is_file():
        raise ValueError("skill must include SKILL.md")
    cases = data.get("cases")
    if not isinstance(cases, list) or not cases:
        raise ValueError("at least one case is required")
    seen: set[str] = set()
    for case in cases:
        case_id = case.get("id") if isinstance(case, dict) else None
        if not isinstance(case_id, str) or not case_id or not all(c.isalnum() or c in "-_" for c in case_id) or case_id in seen:
            raise ValueError("case IDs must be unique, nonempty, and filesystem-safe")
        seen.add(case_id)
        if not isinstance(case.get("prompt"), str) or not case["prompt"].strip():
            raise ValueError(f"case {case_id} needs a prompt")
        fixture = _relative_path(root, case.get("fixture", ""), directory=True)
        if (fixture / ".codex" / "skills").exists():
            raise ValueError(f"case {case_id} fixture already contains skills")
        verify = case.get("verify")
        if not isinstance(verify, list) or not verify or any(not isinstance(arg, str) or not arg for arg in verify):
            raise ValueError(f"case {case_id} needs a verifier argv list")
    repeats = data.get("repeats", 1)
    if not isinstance(repeats, int) or isinstance(repeats, bool) or not 1 <= repeats <= 20:
        raise ValueError("repeats must be 1..20")
    return data


def _run(argv: list[str], cwd: Path, timeout: int, *, env: dict[str, str] | None = None) -> dict[str, Any]:
    started = time.monotonic()
    with subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          text=True, encoding="utf-8", errors="replace",
                          start_new_session=os.name != "nt") as process:
        timed_out = False
        try:
            stdout, stderr = process.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            # Killing only codex.cmd leaves Node/Codex holding the pipes open.
            # Stop exactly this evaluation process tree before draining output.
            if os.name == "nt":
                subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"],
                               capture_output=True, timeout=10, check=False)
            else:
                os.killpg(process.pid, signal.SIGKILL)
            process.kill()
            stdout, stderr = process.communicate(timeout=10)
        return {"exit_code": None if timed_out else process.returncode,
                "stdout": stdout, "stderr": stderr, "timed_out": timed_out,
                "elapsed_seconds": round(time.monotonic() - started, 3)}


def _trace_metrics(jsonl: str) -> dict[str, Any]:
    completed = False
    command_count = 0
    usage: dict[str, Any] = {}
    malformed_lines = 0
    final_message = ""
    failed_commands = 0
    missing_rg_errors = 0
    skill_read_commands = 0
    policy_rejections = 0
    successful_rg_commands = 0
    for line in jsonl.splitlines():
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            malformed_lines += 1
            continue
        if event.get("type") == "item.completed" and (event.get("item") or {}).get("type") == "command_execution":
            command_count += 1
            item = event["item"]
            failed_commands += int(item.get("exit_code") not in (None, 0))
            output = str(item.get("aggregated_output", ""))
            missing_rg_errors += int("CommandNotFoundException" in output and (
                "'rg'" in output or "(rg:String)" in output
            ))
            command = str(item.get("command", ""))
            policy_rejections += int(item.get("status") == "declined" or "rejected: blocked by policy" in output)
            successful_rg_commands += int(item.get("exit_code") == 0 and bool(re.search(r"(?<![\w.-])rg(?:\.exe)?\s", command)))
            skill_read_commands += int("SKILL.md" in command and ("Get-Content" in command or "cat " in command))
        if event.get("type") == "item.completed" and (event.get("item") or {}).get("type") == "agent_message":
            final_message = str(event["item"].get("text", ""))
        if event.get("type") == "turn.completed":
            completed = True
            usage = event.get("usage") or {}
    return {"turn_completed": completed, "command_count": command_count, "usage": usage, "malformed_lines": malformed_lines,
            "final_message": final_message, "failed_commands": failed_commands,
            "missing_rg_errors": missing_rg_errors, "skill_read_commands": skill_read_commands,
            "policy_rejections": policy_rejections, "successful_rg_commands": successful_rg_commands}


def run_experiment(manifest_path: Path, output: Path, codex_binary: str = "codex") -> dict[str, Any]:
    manifest_path = manifest_path.resolve()
    data = load_manifest(manifest_path)
    root = manifest_path.parent
    skill = _relative_path(root, data["skill"], directory=True)
    verifier_hashes = {name: _digest(_relative_path(root, name, directory=False)) for name in data.get("verifier_files", [])}
    output = output.resolve()
    for case in data["cases"]:
        fixture = _relative_path(root, case["fixture"], directory=True)
        if output.is_relative_to(fixture) or output.is_relative_to(skill):
            raise ValueError("output directory cannot be inside a fixture or skill")
    if output.exists() and any(output.iterdir()):
        raise ValueError("output directory must be empty")
    output.mkdir(parents=True, exist_ok=True)
    codex_version = _run([codex_binary, "--version"], root, 30)
    if codex_version["exit_code"] != 0:
        raise RuntimeError(f"codex --version failed: {codex_version['stderr']}")
    report: dict[str, Any] = {
        "schema_version": SCHEMA,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "manifest_sha256": _digest(manifest_path),
        "skill_sha256": _digest(skill),
        "model": data["model"],
        "codex_version": codex_version["stdout"].strip(),
        "shell_environment": data.get("shell_environment", {}),
        "verifier_sha256": verifier_hashes,
        "runs": [],
    }
    (output / "manifest.snapshot.json").write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
    timeout = int(data.get("agent_timeout_seconds", 900))
    verify_timeout = int(data.get("verify_timeout_seconds", 120))
    if not 1 <= timeout <= 3600 or not 1 <= verify_timeout <= 900:
        raise ValueError("timeouts out of range")
    for case in data["cases"]:
        fixture = _relative_path(root, case["fixture"], directory=True)
        fixture_sha = _digest(fixture)
        for repeat in range(data.get("repeats", 1)):
            # Alternate order to reduce bias from transient service changes.
            variants = ("baseline", "candidate") if repeat % 2 == 0 else ("candidate", "baseline")
            for variant in variants:
                run_dir = output / case["id"] / f"repeat-{repeat + 1}" / variant
                workspace = run_dir / "workspace"
                shutil.copytree(fixture, workspace)
                if variant == "candidate":
                    shutil.copytree(skill, workspace / ".codex" / "skills" / data["skill_name"])
                argv = [codex_binary, "exec", "--json", "--ephemeral", "--full-auto", "--skip-git-repo-check", "-C", str(workspace), "-m", data["model"], case["prompt"]]
                if data.get("output_schema"):
                    schema = _relative_path(root, data["output_schema"], directory=False)
                    argv[-1:-1] = ["--output-schema", str(schema)]
                # Keep global plugins and personal skills out of both arms. The
                # temporary home is removed even if the agent fails or times out.
                with tempfile.TemporaryDirectory(prefix="catena-codex-") as isolated_home:
                    auth_source = Path(os.environ.get("CODEX_HOME", Path.home() / ".codex")) / "auth.json"
                    if auth_source.is_file():
                        auth_copy = Path(isolated_home) / "auth.json"
                        shutil.copy2(auth_source, auth_copy)
                        auth_copy.chmod(0o600)
                    agent_env = os.environ.copy()
                    agent_env["CODEX_HOME"] = isolated_home
                    if data.get("shell_environment"):
                        # Both arms use the same tool PATH; the CLI keeps its own
                        # launch environment, so Node/auth are not broken by it.
                        config = '[shell_environment_policy]\ninherit = "core"\n[shell_environment_policy.set]\n'
                        config += "".join(f"{key} = {json.dumps(value)}\n" for key, value in data["shell_environment"].items())
                        (Path(isolated_home) / "config.toml").write_text(config, encoding="utf-8")
                    agent = _run(argv, workspace, timeout, env=agent_env)
                (run_dir / "trace.jsonl").write_text(agent["stdout"], encoding="utf-8")
                (run_dir / "agent.stderr.txt").write_text(agent["stderr"], encoding="utf-8")
                trace = _trace_metrics(agent["stdout"])
                (run_dir / "agent.final.txt").write_text(trace.pop("final_message"), encoding="utf-8")
                verifier_unchanged = all(_digest(root / name) == digest for name, digest in verifier_hashes.items())
                verify_argv = [arg.replace("{manifest_dir}", str(root)) for arg in case["verify"]]
                verifier = _run(verify_argv, workspace, verify_timeout) if (
                    agent["exit_code"] == 0 and trace["turn_completed"]
                    and not trace["malformed_lines"] and verifier_unchanged
                ) else None
                if verifier is not None:
                    (run_dir / "verify.stdout.txt").write_text(verifier["stdout"], encoding="utf-8")
                    (run_dir / "verify.stderr.txt").write_text(verifier["stderr"], encoding="utf-8")
                row = {
                    "case_id": case["id"], "repeat": repeat + 1, "variant": variant,
                    "fixture_sha256": fixture_sha, "prompt_sha256": hashlib.sha256(case["prompt"].encode()).hexdigest(),
                    "agent_exit_code": agent["exit_code"], "agent_timed_out": agent["timed_out"],
                    "elapsed_seconds": agent.get("elapsed_seconds"),
                    "trace": trace,
                    "verifier_exit_code": verifier["exit_code"] if verifier else None,
                    "verifier_timed_out": verifier["timed_out"] if verifier else None,
                    "verifier_unchanged": verifier_unchanged,
                    "passed": bool(verifier and verifier["exit_code"] == 0),
                    "artifact_dir": str(run_dir),
                }
                invalid_reasons = []
                if trace["policy_rejections"]:
                    invalid_reasons.append("tool_policy_rejection")
                if data.get("requires_rg_absent") and trace["successful_rg_commands"]:
                    invalid_reasons.append("rg_available_in_target_agent")
                if agent["timed_out"] or not trace["turn_completed"] or agent["exit_code"] != 0:
                    invalid_reasons.append("agent_execution_incomplete")
                if trace["malformed_lines"] or not verifier_unchanged:
                    invalid_reasons.append("evidence_or_verifier_invalid")
                row["invalid_reasons"] = invalid_reasons
                row["valid_for_comparison"] = not invalid_reasons
                report["runs"].append(row)
                (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
                print(f"{case['id']} repeat {repeat + 1} {variant}: {'pass' if row['passed'] else 'fail'}", flush=True)
    for variant in ("baseline", "candidate"):
        rows = [row for row in report["runs"] if row["variant"] == variant]
        report[variant] = {
            "passed": sum(row["passed"] for row in rows),
            "total": len(rows),
            "command_count": sum(row["trace"]["command_count"] for row in rows),
            "input_tokens": sum(row["trace"]["usage"].get("input_tokens", 0) for row in rows),
            "output_tokens": sum(row["trace"]["usage"].get("output_tokens", 0) for row in rows),
            "failed_commands": sum(row["trace"]["failed_commands"] for row in rows),
            "missing_rg_errors": sum(row["trace"]["missing_rg_errors"] for row in rows),
            "skill_read_commands": sum(row["trace"]["skill_read_commands"] for row in rows),
            "elapsed_seconds": round(sum(row["elapsed_seconds"] or 0 for row in rows), 3),
            "valid_trials": sum(row["valid_for_comparison"] for row in rows),
            "valid_passed": sum(row["valid_for_comparison"] and row["passed"] for row in rows),
        }
    (output / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    return report


def main() -> None:
    parser = argparse.ArgumentParser(description="Run paired Codex Skill checks on isolated fixtures")
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--codex-binary", default="codex.cmd" if os.name == "nt" else "codex")
    args = parser.parse_args()
    result = run_experiment(args.manifest, args.output, args.codex_binary)
    print(json.dumps({key: result[key] for key in ("baseline", "candidate")}, ensure_ascii=False))


if __name__ == "__main__":
    main()
