#!/usr/bin/env python3
"""Bounded real OpenSandbox/Codex acceptance: 10, 20, then 50 concurrent submissions."""

from __future__ import annotations

import argparse
import ast
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
import json
import math
from pathlib import Path
import subprocess
import time
import uuid

import psycopg
from psycopg.rows import dict_row

from managed_smoke import artifact, http

GRADES = (10, 20, 50)
CAPACITY = 2
MAX_MODEL_REQUESTS = 4
MAX_OUTPUT_TOKENS = 1024


def percentile(values, quantile):
    if not values:
        return None
    ordered = sorted(values)
    return round(ordered[max(0, math.ceil(quantile * len(ordered)) - 1)], 3)


def distribution(values):
    return {
        "samples": len(values),
        "p50": percentile(values, 0.5),
        "p95": percentile(values, 0.95),
        "max": max(values) if values else None,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument(
        "--grade",
        type=int,
        choices=GRADES,
        help="rerun one grade without repeating the other model calls",
    )
    args = parser.parse_args()
    grades = (args.grade,) if args.grade else GRADES
    env = json.loads(args.environment.read_text())
    api = env["CATENA_MANAGED_AUTH_URL"].removesuffix("/internal/managed/authorize")
    connection = psycopg.connect(
        env["BARENA_DATABASE_URL"], autocommit=True, row_factory=dict_row
    )
    quota = http(api, "/v1/me/resources")["quota"]
    if quota and any(quota[key] for key in ("running", "queued", "reserved_sandboxes")):
        raise RuntimeError(
            "run only on an idle dedicated local tenant; existing work was not touched"
        )
    original = connection.execute(
        "SELECT * FROM tenant_quotas WHERE tenant_id='local'"
    ).fetchone()
    settings = http(api, "/v1/me/llm-config")
    if not settings.get("configured"):
        raise RuntimeError("a real tenant model must already be configured")
    agent = http(api, "/v1/agents", {"display_name": "Managed graded acceptance"})[
        "agent"
    ]["agent_id"]
    http(
        api,
        "/v1/managed-agents",
        {
            "agent_id": agent,
            "timeout_seconds": 120,
            "instructions": "You are a coding agent. Work in /workspace. Perform the single requested implementation using one shell tool call, run the named Python file, and finish with a brief result. Create files with a Python heredoc or shell cat; an apply_patch shell binary is not installed. Do not simulate command results.",
        },
    )
    connection.execute(
        "UPDATE tenant_quotas SET max_sandboxes=2,max_running=2,max_queued=100,max_model_requests=%s,max_output_tokens=%s,requests_per_minute=30 WHERE tenant_id='local'",
        (MAX_MODEL_REQUESTS, MAX_OUTPUT_TOKENS),
    )
    hardware = subprocess.check_output(
        [
            "sysctl",
            "-n",
            "hw.model",
            "hw.memsize",
            "hw.ncpu",
            "machdep.cpu.brand_string",
        ],
        text=True,
    ).splitlines()
    report = {
        "started_at": datetime.now(timezone.utc).isoformat(),
        "hardware": hardware,
        "model": settings["model"],
        "agent_id": agent,
        "sandbox_capacity": CAPACITY,
        "per_run_model_request_limit": MAX_MODEL_REQUESTS,
        "per_request_output_token_limit": MAX_OUTPUT_TOKENS,
        "total_run_limit": sum(grades),
        "total_model_attempt_limit": sum(grades) * MAX_MODEL_REQUESTS,
        "grades": [],
        "resource_samples": [],
        "scope": "real coding runs; concurrent submission is not concurrent sandbox count",
    }
    all_runs, all_sessions = [], []
    args.output.parent.mkdir(parents=True, exist_ok=True)

    def save():
        args.output.write_text(json.dumps(report, indent=2, default=str))

    try:
        for grade in grades:
            sessions = [
                http(api, "/v1/agent-sessions", {"agent_id": agent})["session_id"]
                for _ in range(CAPACITY)
            ]
            all_sessions.extend(sessions)
            batch = str(uuid.uuid4()).replace("-", "")[:10]
            jobs = []
            for index in range(grade):
                n = grade * 1000 + index
                name = f"bench_{batch}_{index}"
                nonce = str(uuid.uuid4())
                prompt = f"Create {name}.py implementing triangular(n), rejecting negative n with ValueError. Include assertions for triangular(0)==0 and triangular(5)==15 and the negative case. Then calculate triangular({n}), write {name}.json with keys n={n}, result, negative_rejected=true, nonce='{nonce}', and print that same JSON. Execute exactly python3 {name}.py and report briefly. Use one shell tool call for writing and running the code."
                jobs.append(
                    {
                        "session": sessions[index % CAPACITY],
                        "name": name,
                        "n": n,
                        "nonce": nonce,
                        "input": prompt,
                        "key": str(uuid.uuid4()),
                    }
                )
            started = time.monotonic()

            def submit(job):
                run = http(
                    api,
                    "/v1/agent-sessions/" + job["session"] + "/runs",
                    {"input": job["input"]},
                    headers={"Idempotency-Key": job["key"]},
                )
                return {**job, "run_id": run["run_id"]}

            with ThreadPoolExecutor(max_workers=grade) as pool:
                jobs = list(pool.map(submit, jobs))
            all_runs.extend(job["run_id"] for job in jobs)
            result = {
                "concurrent_submissions": grade,
                "sessions": sessions,
                "submission_seconds": round(time.monotonic() - started, 3),
                "runs": [],
                "samples": [],
            }
            report["grades"].append(result)
            print(
                f"submitted grade {grade}: {grade} real runs, at most 2 sandbox executions",
                flush=True,
            )
            pending = {job["run_id"]: job for job in jobs}
            next_sample = 0
            deadline = time.monotonic() + grade * 90 / CAPACITY + 120
            while pending and time.monotonic() < deadline:
                rows = connection.execute(
                    "SELECT * FROM agent_runs WHERE run_id=ANY(%s)", (list(pending),)
                ).fetchall()
                for run in rows:
                    if run["state"] in ("QUEUED", "RUNNING", "STOPPING"):
                        continue
                    job = pending.pop(run["run_id"])
                    record = {
                        key: run[key]
                        for key in (
                            "run_id",
                            "session_id",
                            "sandbox_id",
                            "state",
                            "attempt",
                            "trace_id",
                            "model_requests",
                            "input_tokens",
                            "output_tokens",
                            "error",
                        )
                    }
                    record["queue_wait_ms"] = (
                        (run["claimed_at"] - run["created_at"]).total_seconds() * 1000
                        if run["claimed_at"]
                        else None
                    )
                    record["execution_ms"] = (
                        (
                            run["finished_at"] - run["execution_started_at"]
                        ).total_seconds()
                        * 1000
                        if run["finished_at"] and run["execution_started_at"]
                        else None
                    )
                    record["command_exited_before_finalization"] = bool(
                        run["command_exited_at"]
                        and run["finished_at"]
                        and run["command_exited_at"] <= run["finished_at"]
                    )
                    allocation = connection.execute(
                        "SELECT data FROM agent_run_events WHERE run_id=%s AND kind='sandbox.ready'",
                        (run["run_id"],),
                    ).fetchone()
                    record["allocation"] = allocation["data"] if allocation else None
                    if run["state"] == "COMPLETED":
                        assert record["command_exited_before_finalization"], (
                            "terminal result preceded command exit"
                        )
                        files = artifact(api, run["run_id"])
                        proof = json.loads(files["workspace/" + job["name"] + ".json"])
                        tree = ast.parse(files["workspace/" + job["name"] + ".py"])
                        assert any(
                            isinstance(node, ast.FunctionDef)
                            and node.name == "triangular"
                            for node in ast.walk(tree)
                        )
                        assert (
                            sum(isinstance(node, ast.Assert) for node in ast.walk(tree))
                            >= 2
                        )
                        assert proof == {
                            "n": job["n"],
                            "result": job["n"] * (job["n"] + 1) // 2,
                            "negative_rejected": True,
                            "nonce": job["nonce"],
                        }
                        commands = connection.execute(
                            "SELECT data->'item' AS item FROM agent_run_events WHERE run_id=%s AND kind='runtime' AND data->>'type'='item.completed' AND data->'item'->>'type'='command_execution'",
                            (run["run_id"],),
                        ).fetchall()
                        assert any(
                            job["name"] + ".py" in command["item"].get("command", "")
                            and command["item"].get("exit_code") == 0
                            for command in commands
                        ), "missing real Python tool execution"
                        record["artifact_verified"] = True
                        record["source_sha256"] = hashlib.sha256(
                            files["workspace/" + job["name"] + ".py"].encode()
                        ).hexdigest()
                    result["runs"].append(record)
                    print(
                        f"grade {grade}: {len(result['runs'])}/{grade} {run['state']} {run['run_id']}",
                        flush=True,
                    )
                    save()
                if time.monotonic() >= next_sample:
                    resources = http(api, "/v1/me/resources")["quota"]
                    sample = {
                        "elapsed_seconds": round(time.monotonic() - started, 3),
                        "host_database_clock_offset_seconds": round(
                            time.time()
                            - float(
                                connection.execute(
                                    "SELECT extract(epoch FROM clock_timestamp()) AS now"
                                ).fetchone()["now"]
                            ),
                            3,
                        ),
                        **{
                            key: resources[key]
                            for key in ("running", "queued", "reserved_sandboxes")
                        },
                    }
                    assert (
                        sample["running"] <= CAPACITY
                        and sample["reserved_sandboxes"] <= CAPACITY
                    )
                    result["samples"].append(sample)
                    boxes = http(api, "/v1/sandboxes")["sandboxes"]
                    names = [
                        "sandbox-" + box["provider_sandbox_id"]
                        for box in boxes
                        if box["provider_sandbox_id"]
                        and box["state"] in ("BUSY", "IDLE")
                        and box["session_id"] in sessions
                    ]
                    if names:
                        stats = subprocess.run(
                            [
                                "docker",
                                "stats",
                                "--no-stream",
                                "--format",
                                "{{json .}}",
                                *names,
                            ],
                            text=True,
                            capture_output=True,
                            timeout=10,
                        )
                        report["resource_samples"].append(
                            {
                                "grade": grade,
                                "elapsed_seconds": sample["elapsed_seconds"],
                                "stats": [
                                    json.loads(line)
                                    for line in stats.stdout.splitlines()
                                    if line.strip()
                                ],
                                "observer_status": stats.returncode,
                            }
                        )
                    next_sample = time.monotonic() + 5
                    save()
                if pending:
                    blocked = {
                        row["session_id"]
                        for row in connection.execute(
                            "SELECT session_id FROM agent_sessions WHERE session_id=ANY(%s) AND state='NEEDS_REVIEW'",
                            (sessions,),
                        ).fetchall()
                    }
                    for job in pending.values():
                        if job["session"] in blocked:
                            http(api, "/v1/agent-runs/" + job["run_id"] + "/cancel", {})
                    time.sleep(1)
            if pending:
                raise TimeoutError(
                    f"grade {grade} did not finish within the bounded deadline"
                )
            result["elapsed_seconds"] = round(time.monotonic() - started, 3)
            result["success_rate"] = (
                sum(
                    run["state"] == "COMPLETED" and run.get("artifact_verified", False)
                    for run in result["runs"]
                )
                / grade
            )
            result["queue_wait_ms"] = distribution(
                [
                    run["queue_wait_ms"]
                    for run in result["runs"]
                    if run["queue_wait_ms"] is not None
                ]
            )
            result["execution_ms"] = distribution(
                [
                    run["execution_ms"]
                    for run in result["runs"]
                    if run["execution_ms"] is not None
                ]
            )
            result["cold_allocation_ms"] = distribution(
                [
                    run["allocation"]["allocation_ms"]
                    for run in result["runs"]
                    if run["allocation"] and not run["allocation"]["reused"]
                ]
            )
            result["model_usage"] = {
                key: sum(run[key] for run in result["runs"])
                for key in ("model_requests", "input_tokens", "output_tokens")
            }
            for session in sessions:
                http(api, "/v1/agent-sessions/" + session, method="DELETE")
            for _ in range(90):
                if not http(api, "/v1/me/resources")["quota"]["reserved_sandboxes"]:
                    result["cleanup_verified"] = True
                    break
                time.sleep(1)
            assert result.get("cleanup_verified"), "capacity not reclaimed"
            save()
            if result["success_rate"] != 1:
                raise RuntimeError(f"grade {grade} failed; larger grades not started")
        report["finished_at"] = datetime.now(timezone.utc).isoformat()
        report["all_runs_complete"] = True
        save()
    finally:
        for run in all_runs:
            try:
                state = http(api, "/v1/agent-runs/" + run)["state"]
                if state in ("QUEUED", "RUNNING", "STOPPING"):
                    http(api, "/v1/agent-runs/" + run + "/cancel", {})
            except OSError:
                pass
        for session in all_sessions:
            try:
                http(api, "/v1/agent-sessions/" + session, method="DELETE")
            except OSError:
                pass
        if original:
            connection.execute(
                "UPDATE tenant_quotas SET max_sandboxes=%s,max_running=%s,max_queued=%s,max_model_requests=%s,max_output_tokens=%s,requests_per_minute=%s WHERE tenant_id='local'",
                tuple(
                    original[key]
                    for key in (
                        "max_sandboxes",
                        "max_running",
                        "max_queued",
                        "max_model_requests",
                        "max_output_tokens",
                        "requests_per_minute",
                    )
                ),
            )
        connection.close()
        save()


if __name__ == "__main__":
    main()
