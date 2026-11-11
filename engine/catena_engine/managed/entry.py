"""Runs inside the user sandbox as uid 1000; never on the control-plane host."""

from __future__ import annotations

import base64
import io
import json
import os
import signal
import subprocess
import sys
import tarfile
import threading
from pathlib import Path

MAX_SNAPSHOT = 16 * 1024 * 1024
MAX_EXPANDED = 64 * 1024 * 1024


def emit(kind, **data):
    print(json.dumps({"kind": kind, **data}, ensure_ascii=False), flush=True)


def restore(encoded, workspace, home):
    if not encoded:
        return
    data = base64.b64decode(encoded, validate=True)
    if len(data) > MAX_SNAPSHOT:
        raise ValueError("checkpoint exceeds 16 MiB")
    with tarfile.open(fileobj=io.BytesIO(data), mode="r|gz") as archive:
        total, count = 0, 0
        for member in archive:
            count += 1
            if count > 4096:
                raise ValueError("checkpoint contains too many files")
            parts = Path(member.name).parts
            if not parts or parts[0] not in ("workspace", "codex") or ".." in parts or Path(member.name).is_absolute():
                raise ValueError("invalid checkpoint path")
            if not member.isfile():
                raise ValueError("checkpoint only permits regular files")
            root = workspace if parts[0] == "workspace" else home
            target = root.joinpath(*parts[1:])
            if not target.resolve().is_relative_to(root.resolve()):
                raise ValueError("checkpoint path escapes root")
            total += member.size
            if total > MAX_EXPANDED:
                raise ValueError("expanded checkpoint exceeds 64 MiB")
            target.parent.mkdir(parents=True, exist_ok=True)
            with archive.extractfile(member) as source, target.open("wb") as output:
                while chunk := source.read(65536):
                    output.write(chunk)
            target.chmod(member.mode & 0o700)


def snapshot(workspace, home):
    output = io.BytesIO()
    total = 0
    count = 0
    with tarfile.open(fileobj=output, mode="w:gz") as archive:
        for root, prefix in ((workspace, "workspace"), (home / "sessions", "codex/sessions")):
            if not root.exists():
                continue
            for path in sorted(root.rglob("*")):
                if path.is_symlink() or not path.is_file():
                    continue
                total += path.stat().st_size
                count += 1
                if total > MAX_EXPANDED or count > 4095:
                    raise ValueError("workspace exceeds checkpoint limits")
                archive.add(path, arcname=f"{prefix}/{path.relative_to(root)}", recursive=False)
        thread = home / "thread-id"
        if thread.is_file():
            archive.add(thread, arcname="codex/thread-id", recursive=False)
    data = output.getvalue()
    if len(data) > MAX_SNAPSHOT:
        raise ValueError("compressed checkpoint exceeds 16 MiB")
    return base64.b64encode(data).decode()


def config(request):
    q = json.dumps
    return f"""model = {q(request["model"])}
model_provider = "catena"
developer_instructions = {q(request["instructions"])}
approval_policy = "never"
sandbox_mode = "danger-full-access"
web_search = "disabled"
[features]
apps = false
multi_agent = false
plugins = false
remote_plugin = false
hooks = false
workspace_dependencies = false
browser_use = false
computer_use = false
enable_request_compression = false
[shell_environment_policy]
inherit = "none"
set = {{ PATH = "/usr/local/bin:/usr/bin:/bin", HOME = "/home/agent" }}
[model_providers.catena]
name = "CATENA scoped gateway"
base_url = {q(request["gateway"])}
env_key = "CATENA_RUN_TOKEN"
wire_api = "responses"
request_max_retries = 0
stream_max_retries = 0
supports_websockets = false
"""


def main():
    request_path = Path(sys.argv[1])
    request = json.loads(request_path.read_text())
    request_path.unlink()
    workspace, home = Path("/workspace"), Path("/home/agent/.codex")
    workspace.mkdir(exist_ok=True)
    home.mkdir(parents=True, exist_ok=True)
    restore(request.get("checkpoint"), workspace, home)
    # The outer OpenSandbox container and non-root uid own the tool boundary.
    (home / "config.toml").write_text(config(request))
    version = subprocess.check_output(["codex", "--version"], text=True).strip()
    if version != "codex-cli 0.160.0":
        raise RuntimeError("unsupported Codex version")
    thread = home / "thread-id"
    command = ["codex", "-C", str(workspace), "exec"]
    if thread.exists():
        command += ["resume", thread.read_text().strip()]
    command += ["--json", "--skip-git-repo-check", "-"]
    env = {
        "PATH": os.environ["PATH"],
        "HOME": "/home/agent",
        "CODEX_HOME": str(home),
        "CATENA_RUN_TOKEN": request["token"],
    }
    emit(
        "runtime",
        data={
            "type": "runtime.started",
            "run_id": request["run_id"],
            "generation": request["generation"],
            "version": version,
        },
    )
    process = subprocess.Popen(
        command,
        cwd=workspace,
        env=env,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )

    def stop():
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass

    timer = threading.Timer(request["timeout"], stop)
    timer.start()
    result, completed, size, error = "", False, 0, ""
    try:
        process.stdin.write(request["input"].encode())
        process.stdin.close()
        while line := process.stdout.readline(65537):
            size += len(line)
            if len(line) > 65536 or size > 4 * 1024 * 1024:
                raise ValueError("Codex event output exceeds limit")
            try:
                event = json.loads(line)
            except ValueError:
                emit("runtime", data={"type": "runtime.diagnostic", "text": line.decode(errors="replace")[:4096]})
                continue
            if event.get("type") == "thread.started":
                thread.write_text(event["thread_id"])
            if event.get("type") == "turn.failed":
                error = str(event.get("error", {}).get("message", "Codex turn failed"))[:1000]
            if event.get("type") == "turn.completed":
                completed = True
            item = event.get("item", {})
            if item.get("type") == "agent_message":
                result = item.get("text", "")[:131072]
            emit("runtime", data=event)
        code = process.wait()
        emit(
            "result",
            success=code == 0 and completed,
            result=result,
            error=error if code == 0 and completed else error or "Codex turn did not complete",
            snapshot=snapshot(workspace, home),
        )
    finally:
        timer.cancel()
        stop()
        process.wait()


if __name__ == "__main__":
    main()
