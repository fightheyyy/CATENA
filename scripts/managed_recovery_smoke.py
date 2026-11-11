#!/usr/bin/env python3
"""Kill the local managed worker during a real sandbox tool call and verify safe recovery."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import time
import urllib.request

from managed_smoke import http


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    state = args.environment.resolve().parent
    env = os.environ | json.loads(args.environment.read_text())
    api = env["CATENA_MANAGED_AUTH_URL"].removesuffix("/internal/managed/authorize")
    agent = http(api, "/v1/agents", {"display_name": "Worker crash acceptance"})[
        "agent"
    ]["agent_id"]
    http(
        api,
        "/v1/managed-agents",
        {
            "agent_id": agent,
            "instructions": "Execute tasks in /workspace using shell tools. Do not simulate execution.",
        },
    )
    session = http(api, "/v1/agent-sessions", {"agent_id": agent})["session_id"]
    run = http(
        api,
        f"/v1/agent-sessions/{session}/runs",
        {
            "input": "Execute this exact shell command now: /usr/bin/python3 -c 'import time; time.sleep(90)'. Only after it finishes write completed-after-sleep.txt. Do not run the command in the background."
        },
        headers={"Idempotency-Key": session},
    )
    print("waiting for real tool execution", run["run_id"], flush=True)
    deadline = time.monotonic() + 150
    started = False
    with urllib.request.urlopen(
        api + "/v1/agent-runs/" + run["run_id"] + "/events", timeout=30
    ) as response:
        while time.monotonic() < deadline:
            line = response.readline(65536).decode()
            if not line:
                break
            if not line.startswith("data: "):
                continue
            item = json.loads(line.removeprefix("data: ")).get("item") or {}
            if (
                item.get("type") == "command_execution"
                and item.get("status") == "in_progress"
                and "time.sleep(90)" in item.get("command", "")
            ):
                started = True
                break
    assert started, "no real tool execution observed; worker was not killed"
    old = http(api, "/v1/agent-runs/" + run["run_id"])
    box = next(
        item
        for item in http(api, "/v1/sandboxes")["sandboxes"]
        if item["session_id"] == session
    )
    pid = int((state / "core.pid").read_text())
    expected_binary = str(state / "catena-server")
    command = subprocess.check_output(
        ["ps", "-p", str(pid), "-o", "command="], text=True
    ).strip()
    assert command == expected_binary, "refusing to kill an unrelated PID"
    crash_at = time.monotonic()
    os.kill(pid, signal.SIGKILL)
    with (state / "core.log").open("a") as log:
        child = subprocess.Popen(
            [expected_binary],
            cwd=Path(__file__).resolve().parents[1],
            env=env,
            stdout=log,
            stderr=log,
            start_new_session=True,
        )
    (state / "core.pid").write_text(str(child.pid))
    final = None
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        try:
            current = http(api, "/v1/agent-runs/" + run["run_id"])
            boxes = http(api, "/v1/sandboxes")["sandboxes"]
            observed_box = next(
                item for item in boxes if item["sandbox_id"] == box["sandbox_id"]
            )
            if (
                current["state"] == "NEEDS_REVIEW"
                and observed_box["state"] == "TERMINATED"
            ):
                final = current
                break
        except OSError:
            pass
        time.sleep(0.5)
    assert final is not None, "worker recovery or sandbox quarantine did not converge"
    assert final.get("isolation_confirmed_at") and final["finished_at"] >= final["isolation_confirmed_at"], "terminal state preceded confirmed isolation"
    assert final["attempt"] == 1 and final["fencing_token"] > old["fencing_token"], (
        "unsafe replay or missing fencing"
    )
    assert final["result_ref"] is None and not final["result"], (
        "uncertain execution fabricated a result"
    )
    record = {
        "run_id": final["run_id"],
        "trace_id": final["trace_id"],
        "session_id": session,
        "sandbox_id": box["sandbox_id"],
        "provider_id": box["provider_sandbox_id"],
        "before_fence": old["fencing_token"],
        "after_fence": final["fencing_token"],
        "state": final["state"],
        "attempt": final["attempt"],
        "crash_to_quarantine_seconds": round(time.monotonic() - crash_at, 3),
        "real_tool_execution_observed": True,
        "sandbox_terminated": True,
        "automatic_replay": False,
        "isolation_confirmed_at": final.get("isolation_confirmed_at"),
        "finished_at": final["finished_at"],
        "terminal_after_isolation": final.get("isolation_confirmed_at") is not None,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(record, indent=2))
    http(api, "/v1/agent-sessions/" + session, method="DELETE")
    print(json.dumps(record), flush=True)


if __name__ == "__main__":
    main()
