#!/usr/bin/env python3
"""Local Mac/Docker Desktop stack. Does not modify existing CATENA deployments."""

from __future__ import annotations

import argparse
import ipaddress
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
import venv

ROOT = Path(__file__).resolve().parents[2]
STATE = ROOT / ".local/managed-dev"
DB = "catena-managed-dev-postgres"
API = "http://127.0.0.1:18789"
NODE = "node:22-bookworm-slim@sha256:43ac6c60b8f89723f746e8a92ce91abd5017e627ce1ddfe4238355d3a30b772c"


def run(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, **kwargs)


def request(path, method="GET", data=None):
    req = urllib.request.Request(
        API + path,
        method=method,
        data=json.dumps(data).encode() if data is not None else None,
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=10) as response:
        return json.load(response)


def setup():
    if sys.version_info < (3, 12):
        raise RuntimeError("Python 3.12+ required")
    run(["docker", "info"], stdout=subprocess.DEVNULL)
    STATE.mkdir(parents=True, exist_ok=True, mode=0o700)
    venv.create(STATE / "venv", with_pip=True)
    run(
        [
            str(STATE / "venv/bin/pip"),
            "install",
            "-r",
            str(ROOT / "deploy/managed/requirements.txt"),
        ]
    )
    for image in (
        "opensandbox/execd:release-1.2.0",
        "opensandbox/egress:release-1.2.0",
    ):
        run(["docker", "pull", image])
    run(
        [
            "docker",
            "build",
            "-f",
            "deploy/managed/Dockerfile.codex",
            "-t",
            "catena/managed-codex:0.160.0",
            ".",
        ]
    )
    run(["pnpm", "--dir", "frontend", "install", "--frozen-lockfile"])
    run(["pnpm", "--dir", "frontend", "build"])
    shutil.rmtree(ROOT / "backend/internal/control/web")
    shutil.copytree(ROOT / "frontend/dist", ROOT / "backend/internal/control/web")
    run(
        [
            "go",
            "-C",
            "backend",
            "build",
            "-o",
            str(STATE / "catena-server"),
            "./cmd/catena-server",
        ]
    )
    env_path = STATE / "env.json"
    if not env_path.exists():
        # Resolve through Docker Desktop, rather than hard-coding its host gateway address.
        gateway = subprocess.check_output(
            [
                "docker",
                "run",
                "--rm",
                NODE,
                "getent",
                "ahostsv4",
                "host.docker.internal",
            ],
            text=True,
        ).split()[0]
        ipaddress.IPv4Address(gateway)
        env = {
            "BARENA_SERVER_ADDR": "127.0.0.1:18789",
            "BARENA_DATABASE_URL": "postgres://postgres:local-managed-dev@127.0.0.1:55441/postgres?sslmode=disable",
            "BARENA_API_TOKEN_ENCRYPTION_KEY": secrets.token_urlsafe(32),
            "CATENA_MANAGED_RUNNER_URL": "http://127.0.0.1:18792",
            "CATENA_MANAGED_RUNNER_TOKEN": secrets.token_urlsafe(32),
            "CATENA_MANAGED_CONTROL_URL": f"http://{gateway}:18788",
            "CATENA_MANAGED_AUTH_URL": API + "/internal/managed/authorize",
            "CATENA_MANAGED_RUNNER_ADDR": "127.0.0.1:18792",
            "CATENA_MANAGED_GATEWAY_ADDR": "0.0.0.0:18788",
            "OPEN_SANDBOX_DOMAIN": "127.0.0.1:18080",
            "OPEN_SANDBOX_API_KEY": secrets.token_urlsafe(32),
            "CATENA_MANAGED_CAPACITY": "2",
            "CATENA_MANAGED_WORKERS": "2",
            "CATENA_COLLECTOR_URL": "http://127.0.0.1:14318",
            "CATENA_CLICKHOUSE_DSN": "http://default:managed-local-traces@127.0.0.1:18125/catena?dial_timeout=5s",
            "CATENA_TRACE_BACKEND": "otel",
        }
        env_path.write_text(json.dumps(env))
        env_path.chmod(0o600)
    env = json.loads(env_path.read_text())
    config = f'''[server]
host = "127.0.0.1"
port = 18080
api_key = "{env["OPEN_SANDBOX_API_KEY"]}"
max_sandbox_timeout_seconds = 3600
[proxy]
resolve_internal = false
[runtime]
type = "docker"
execd_image = "opensandbox/execd:release-1.2.0"
[store]
type = "sqlite"
path = "{STATE / "opensandbox.db"}"
[docker]
network_mode = "bridge"
publish_host = "127.0.0.1"
port_range_min = 42000
port_range_max = 42100
drop_capabilities = ["AUDIT_WRITE", "MKNOD", "NET_ADMIN", "NET_RAW", "SYS_ADMIN", "SYS_MODULE", "SYS_PTRACE", "SYS_TIME", "SYS_TTY_CONFIG"]
no_new_privileges = true
pids_limit = 128
[egress]
image = "opensandbox/egress:release-1.2.0"
mode = "dns+nft"
'''
    path = STATE / "opensandbox.toml"
    path.write_text(config)
    path.chmod(0o600)


def start():
    env = os.environ | json.loads((STATE / "env.json").read_text())
    env["PYTHONPATH"] = str(ROOT / "engine")
    gateway = subprocess.check_output(
        ["docker", "run", "--rm", NODE, "getent", "ahostsv4", "host.docker.internal"],
        text=True,
    ).split()[0]
    ipaddress.IPv4Address(gateway)
    env["CATENA_MANAGED_CONTROL_URL"] = f"http://{gateway}:18788"
    run(["docker", "compose", "-f", "deploy/managed/compose.telemetry.yml", "up", "-d"])

    existing = subprocess.run(["docker", "inspect", DB], capture_output=True)
    if existing.returncode == 0:
        info = json.loads(existing.stdout)[0]
        if info["Config"]["Labels"].get("catena.managed.dev") != "1":
            raise RuntimeError("refusing to reuse unrelated container")
        run(["docker", "start", DB])
    else:
        run(
            [
                "docker",
                "run",
                "-d",
                "--name",
                DB,
                "--label",
                "catena.managed.dev=1",
                "--memory",
                "512m",
                "--cpus",
                "1",
                "-e",
                "POSTGRES_PASSWORD=local-managed-dev",
                "-p",
                "127.0.0.1:55441:5432",
                "-v",
                "catena-managed-dev-postgres:/var/lib/postgresql/data",
                "postgres:17-alpine",
            ]
        )
    for _ in range(60):
        if (
            subprocess.run(
                ["docker", "exec", DB, "pg_isready", "-U", "postgres"],
                capture_output=True,
            ).returncode
            == 0
        ):
            break
        time.sleep(0.5)
    else:
        raise RuntimeError("PostgreSQL did not become ready")
    processes = [
        (
            "opensandbox",
            [
                str(STATE / "venv/bin/opensandbox-server"),
                "--config",
                str(STATE / "opensandbox.toml"),
            ],
        ),
        (
            "runner",
            [str(STATE / "venv/bin/python"), "-m", "catena_engine.managed.server"],
        ),
        ("core", [str(STATE / "catena-server")]),
    ]
    for name, command in processes:
        pid_path = STATE / f"{name}.pid"
        if pid_path.exists():
            try:
                os.kill(int(pid_path.read_text()), 0)
                raise RuntimeError(f"{name} is already running; use status/down")
            except ProcessLookupError:
                pass
        with (STATE / f"{name}.log").open("a") as log:
            child = subprocess.Popen(
                command,
                cwd=ROOT,
                env=env,
                stdout=log,
                stderr=log,
                start_new_session=True,
            )
        pid_path.write_text(str(child.pid))
    for _ in range(60):
        try:
            request("/readyz")
            print(f"Ready: {API}/agent-sessions. Configure your LLM in Connections.")
            return
        except (OSError, urllib.error.URLError):
            time.sleep(0.5)
    raise RuntimeError(f"Startup failed; inspect {STATE}/*.log")


def stop():
    for name in ("core", "runner", "opensandbox"):
        path = STATE / f"{name}.pid"
        if path.exists():
            pid = int(path.read_text())
            # Verify PID identity to avoid signalling a process that reused an old PID.
            command = subprocess.run(
                ["ps", "-p", str(pid), "-o", "command="], capture_output=True, text=True
            ).stdout
            if str(STATE) in command or (
                name == "runner" and "catena_engine.managed.server" in command
            ):
                os.kill(pid, signal.SIGTERM)
            path.unlink()
    subprocess.run(["docker", "stop", DB], check=False)
    run(["docker", "compose", "-f", "deploy/managed/compose.telemetry.yml", "down"])
    print("Stopped services; database volume and checkpoints preserved.")


def cleanup():
    sessions = request("/v1/agent-sessions")["sessions"]
    for session in sessions:
        if session["state"] != "DELETED":
            detail = request(f"/v1/agent-sessions/{session['session_id']}")
            for item in detail["runs"]:
                if item["state"] in ("QUEUED", "RUNNING", "STOPPING"):
                    request(f"/v1/agent-runs/{item['run_id']}/cancel", "POST", {})
    for _ in range(60):
        pending = False
        for session in sessions:
            if session["state"] == "DELETED":
                continue
            try:
                request(f"/v1/agent-sessions/{session['session_id']}", "DELETE")
            except urllib.error.HTTPError as error:
                if error.code != 409:
                    raise
                pending = True
        if not pending:
            print(
                "Cleanup requested. Verify Sandbox Monitor reaches TERMINATED; unknown creates retain reservations until provider TTL + grace."
            )
            return
        time.sleep(0.5)
    raise RuntimeError(
        "Cleanup still pending; keep services running and inspect Sandbox Monitor"
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("setup", "up", "down", "status", "cleanup"))
    args = parser.parse_args()
    os.chdir(ROOT)
    if args.action == "setup":
        setup()
    elif args.action == "up":
        start()
    elif args.action == "down":
        cleanup()
        for _ in range(60):
            resources = request("/v1/me/resources")
            if not resources["quota"] or resources["quota"]["reserved_sandboxes"] == 0:
                stop()
                break
            time.sleep(0.5)
        else:
            raise RuntimeError(
                "Sandbox cleanup remains unconfirmed; services left running to reconcile. Inspect status/logs before stopping OpenSandbox."
            )
    elif args.action == "cleanup":
        cleanup()
    else:
        print(json.dumps(request("/v1/me/resources"), indent=2))


if __name__ == "__main__":
    main()
