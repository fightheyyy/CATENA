#!/usr/bin/env python3
"""Real Coding Agent acceptance; requires a configured tenant model and running local stack."""

from __future__ import annotations

import argparse
import hashlib
import io
import json
from pathlib import Path
import tarfile
import time
import urllib.request
import uuid


def http(base, path, body=None, method=None, headers=None):
    request = urllib.request.Request(
        base + path,
        data=json.dumps(body).encode() if body is not None else None,
        method=method,
        headers={"Content-Type": "application/json", **(headers or {})},
    )
    with urllib.request.urlopen(request, timeout=20) as response:
        return json.load(response)


def wait_run(api, run_id):
    deadline = time.monotonic() + 240
    while time.monotonic() < deadline:
        run = http(api, "/v1/agent-runs/" + run_id)
        if run["state"] not in ("QUEUED", "RUNNING", "STOPPING"):
            if run["state"] != "COMPLETED":
                raise RuntimeError(f"Run {run_id}: {run['state']}: {run['error']}")
            return run
        time.sleep(1)
    raise TimeoutError(f"run {run_id} did not finish")


def artifact(api, run_id):
    with urllib.request.urlopen(
        api + "/v1/agent-runs/" + run_id + "/artifact", timeout=20
    ) as response:
        data = response.read(16 * 1024 * 1024 + 1)
        assert hashlib.sha256(data).hexdigest() == response.headers["X-Content-SHA256"]
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        return {
            item.name: archive.extractfile(item).read().decode()
            for item in archive.getmembers()
            if item.isfile() and item.name.startswith("workspace/")
        }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api", default="http://127.0.0.1:18789")
    parser.add_argument(
        "--environment",
        type=Path,
        help="private dev env.json; enables real provider loss and rebuild test",
    )
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    settings = http(args.api, "/v1/me/llm-config")
    if not settings.get("configured"):
        raise RuntimeError("Configure your tenant LLM in Connections first")
    registered = http(
        args.api,
        "/v1/agents",
        {"display_name": "Managed acceptance " + str(uuid.uuid4())[:8]},
    )["agent"]
    agent = registered["agent_id"]
    http(
        args.api,
        "/v1/managed-agents",
        {
            "agent_id": agent,
            "instructions": "Work only in /workspace. Actually execute the requested code and verify its output. Do not fabricate files or test results.",
        },
    )
    session = http(args.api, "/v1/agent-sessions", {"agent_id": agent})["session_id"]
    results = {
        "agent_id": agent,
        "session_id": session,
        "model": settings["model"],
        "runs": [],
    }

    def submit(prompt, expected_file, expected_text):
        key = str(uuid.uuid4())
        request_path = "/v1/agent-sessions/" + session + "/runs"
        body = {"input": prompt}
        queued = http(args.api, request_path, body, headers={"Idempotency-Key": key})
        duplicate = http(args.api, request_path, body, headers={"Idempotency-Key": key})
        assert queued["run_id"] == duplicate["run_id"], (
            "duplicate submission created a second run"
        )
        print("waiting", queued["run_id"], flush=True)
        run = wait_run(args.api, queued["run_id"])
        files = artifact(args.api, run["run_id"])
        assert files["workspace/" + expected_file].strip() == expected_text, files
        assert run["model_requests"] > 0 and run["input_tokens"] > 0, (
            "no real model accounting evidence"
        )
        detail = http(args.api, "/v1/agent-sessions/" + session)["session"]
        results["runs"].append(
            {
                k: run[k]
                for k in (
                    "run_id",
                    "state",
                    "attempt",
                    "trace_id",
                    "model_requests",
                    "input_tokens",
                    "output_tokens",
                    "result_ref",
                    "created_at",
                    "finished_at",
                )
            }
        )
        results["runs"][-1]["sandbox_id"] = detail["sandbox_binding"]
        results["runs"][-1]["workspace_hashes"] = {
            name: hashlib.sha256(text.encode()).hexdigest()
            for name, text in files.items()
        }
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(results, indent=2))
        return detail["sandbox_binding"]

    first = submit(
        "Create hello.py containing print('hello catena'). Execute python3 hello.py > run-output.txt, then read run-output.txt.",
        "run-output.txt",
        "hello catena",
    )
    second = submit(
        "Keep hello.py and append print('second run') if absent. Execute python3 hello.py > second-output.txt, then read second-output.txt.",
        "second-output.txt",
        "hello catena\nsecond run",
    )
    assert first == second, "healthy session did not reuse sandbox"
    http(args.api, "/v1/agent-sessions/" + session + "/pause", {})
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if (
            http(args.api, "/v1/agent-sessions/" + session)["session"]["state"]
            == "PAUSED"
        ):
            break
        time.sleep(0.5)
    else:
        raise TimeoutError("sandbox pause did not converge")
    http(args.api, "/v1/agent-sessions/" + session + "/resume", {})
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        boxes = http(args.api, "/v1/sandboxes")["sandboxes"]
        if any(box["sandbox_id"] == second and box["state"] == "IDLE" for box in boxes):
            break
        time.sleep(0.5)
    else:
        raise TimeoutError("sandbox resume did not converge")
    results["pause_resume_verified"] = True
    if args.environment:
        env = json.loads(args.environment.read_text())
        boxes = http(args.api, "/v1/sandboxes")["sandboxes"]
        box = next(item for item in boxes if item["sandbox_id"] == second)
        http(
            env["CATENA_MANAGED_RUNNER_URL"],
            "/v1/terminate",
            {"provider_id": box["provider_sandbox_id"]},
            headers={"Authorization": "Bearer " + env["CATENA_MANAGED_RUNNER_TOKEN"]},
        )
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            box = next(
                item
                for item in http(args.api, "/v1/sandboxes")["sandboxes"]
                if item["sandbox_id"] == second
            )
            if box["state"] == "TERMINATED":
                break
            time.sleep(1)
        else:
            raise TimeoutError("lost sandbox did not converge")
        rebuilt = submit(
            "Run the existing hello.py from the restored workspace using python3 hello.py > recovered-output.txt. Read the output.",
            "recovered-output.txt",
            "hello catena\nsecond run",
        )
        assert rebuilt != first, "sandbox was not rebuilt"
        results["rebuild_verified"] = True
    http(args.api, "/v1/agent-sessions/" + session, method="DELETE")
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        if (
            http(args.api, "/v1/agent-sessions/" + session)["session"]["state"]
            == "DELETED"
        ):
            results["cleanup_verified"] = True
            break
        time.sleep(1)
    assert results.get("cleanup_verified"), "cleanup did not finish"
    args.output.write_text(json.dumps(results, indent=2))
    print(
        "verified real execution, duplicate admission, reuse, checkpoints, cleanup",
        flush=True,
    )


if __name__ == "__main__":
    main()
