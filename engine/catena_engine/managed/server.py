"""Private OpenSandbox SDK adapter. No user code is executed in this service."""

from __future__ import annotations

import asyncio
import hmac
import ipaddress
import json
import os
import re
import shlex
import urllib.request
from datetime import timedelta
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlsplit

from opensandbox.config import ConnectionConfig
from opensandbox.exceptions import SandboxApiException
from opensandbox.manager import SandboxManager
from opensandbox.models.execd import ExecutionHandlers, RunCommandOpts
from opensandbox.models.filesystem import WriteEntry
from opensandbox.models.sandboxes import NetworkPolicy, NetworkRule, SandboxFilter
from opensandbox.sandbox import Sandbox
from opensandbox.transport import RetryPolicy

TOKEN = os.environ.get("CATENA_MANAGED_RUNNER_TOKEN", "")
CONTROL = os.environ.get("CATENA_MANAGED_AUTH_URL", "http://127.0.0.1:8787/internal/managed/authorize")


def connection():
    return ConnectionConfig(
        domain=os.environ.get("OPEN_SANDBOX_DOMAIN", "127.0.0.1:8080"),
        api_key=os.environ["OPEN_SANDBOX_API_KEY"],
        use_server_proxy=True,
        request_timeout=timedelta(seconds=60),
        retry_policy=RetryPolicy.disabled(),
        disable_metrics=True,
    )


def authorize(data):
    payload = {k: data[k] for k in ("run_id", "sandbox_id", "generation", "token", "fencing_token")}
    request = urllib.request.Request(
        CONTROL,
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {TOKEN}"},
    )
    with urllib.request.urlopen(request, timeout=5) as response:
        if response.status != 200:
            raise PermissionError("execution lease revoked")


async def lifecycle(action, data):
    config = connection()
    if action == "create":
        authorize(data)
        sandbox = await Sandbox.create(
            os.environ.get("CATENA_MANAGED_IMAGE", "catena/managed-codex:0.160.0"),
            connection_config=config,
            timeout=timedelta(hours=1),
            ready_timeout=timedelta(seconds=90),
            resource={"cpu": data["cpu"], "memory": data["memory"]},
            network_policy=NetworkPolicy(
                default_action="deny",
                egress=[
                    NetworkRule(
                        action="allow",
                        target=str(ipaddress.ip_address(urlsplit(os.environ["CATENA_MANAGED_CONTROL_URL"]).hostname)),
                        ports=[urlsplit(os.environ["CATENA_MANAGED_CONTROL_URL"]).port or 80],
                    )
                ],
            ),
            metadata={
                "catena.namespace": data["namespace"],
                "catena.sandbox": data["sandbox_id"],
                "catena.session": data["session_id"],
                "catena.tenant": data["tenant_id"],
            },
        )
        try:
            return {"provider_id": sandbox.id}
        finally:
            await sandbox.close()
    manager = await SandboxManager.create(connection_config=config)
    try:
        if action == "list":
            result, page = [], 1
            while True:
                found = await manager.list_sandbox_infos(
                    SandboxFilter(metadata={"catena.namespace": data["namespace"]}, page=page, page_size=100)
                )
                for item in found.sandbox_infos:
                    result.append(
                        {
                            "provider_id": item.id,
                            "sandbox_id": (item.metadata or {}).get("catena.sandbox", ""),
                            "state": item.status.state,
                            "created_at": item.created_at.isoformat(),
                        }
                    )
                if len(found.sandbox_infos) < 100:
                    return result
                page += 1
                if page > 100:
                    raise RuntimeError("provider listing exceeds controller bound")
        if action == "inspect":
            return (await manager.get_sandbox_info(data["provider_id"])).model_dump(mode="json")
        if action == "pause":
            await manager.pause_sandbox(data["provider_id"])
        elif action == "resume":
            await manager.resume_sandbox(data["provider_id"])
        elif action == "terminate":
            try:
                await manager.kill_sandbox(data["provider_id"])
                await manager.get_sandbox_info(data["provider_id"])
                raise RuntimeError("termination not yet confirmed")
            except SandboxApiException as error:
                if error.status_code != 404:
                    raise
        else:
            raise ValueError("unsupported lifecycle operation")
        return {"ok": True}
    finally:
        await manager.close()


async def execute(data, write):
    authorize(data)
    if not re.fullmatch(r"run-[a-zA-Z0-9_-]+", data["run_id"]):
        raise ValueError("invalid run identity")
    sandbox = await Sandbox.connect(data["provider_id"], connection_config=connection())
    try:
        info = await sandbox.get_info()
        metadata = info.metadata or {}
        if any(
            metadata.get(key) != data[value]
            for key, value in (
                ("catena.sandbox", "sandbox_id"),
                ("catena.session", "session_id"),
                ("catena.tenant", "tenant_id"),
                ("catena.namespace", "namespace"),
            )
        ):
            raise PermissionError("sandbox binding mismatch")
        request_path = f"/tmp/{data['run_id']}.json"

        # SDK 1.2.0 transmits permission digits as text (600, not Python 0o600).
        await sandbox.files.write_files(
            [WriteEntry(path=request_path, data=json.dumps(data), mode=600, owner="agent", group="agent")]
        )
        authorize(data)
        pending = ""
        result = None

        def receive(line):
            nonlocal result
            event = json.loads(line)
            if event.get("kind") == "result":
                if result is not None:
                    raise ValueError("duplicate runtime result")
                result = event
            else:
                write(event)

        async def stdout(message):
            nonlocal pending
            pending += message.text
            while "\n" in pending:
                line, pending = pending.split("\n", 1)
                if line:
                    receive(line)
            if len(pending) > 24 * 1024 * 1024:
                raise ValueError("runtime frame exceeds limit")

        async def stderr(message):
            write({"kind": "runtime", "data": {"type": "runtime.stderr", "text": message.text[:4096]}})

        execution = await sandbox.commands.run(
            f"mkdir -p /run/catena && mkdir /run/catena/{shlex.quote(data['run_id'])} && exec runuser -u agent -- python3 /opt/catena/entry.py {shlex.quote(request_path)}",
            opts=RunCommandOpts(
                timeout=timedelta(seconds=data["timeout"] + 10), uid=0, gid=0, working_directory="/workspace"
            ),
            handlers=ExecutionHandlers(on_stdout=stdout, on_stderr=stderr, skip_accumulation=True),
        )
        if pending.strip():
            receive(pending)
        if execution.error or execution.complete is None or execution.exit_code != 0:
            raise RuntimeError(
                f"sandbox command exit not confirmed: complete={execution.complete is not None}, exit_code={execution.exit_code}, provider_error={execution.error}"
            )
        if result is None:
            raise RuntimeError("sandbox exited without a runtime result")
        result["command_exited"] = True
        result["exit_code"] = execution.exit_code
        write(result)
    finally:
        await sandbox.close()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass  # Never log capability-bearing request bodies.

    def do_GET(self):
        if self.path != "/healthz":
            self.send_error(404)
            return
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}')

    def do_POST(self):
        if not hmac.compare_digest(self.headers.get("Authorization", ""), f"Bearer {TOKEN}"):
            self.send_error(401)
            return
        streaming = False
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if not 0 < length <= 24 * 1024 * 1024:
                raise ValueError("invalid body length")
            data = json.loads(self.rfile.read(length))
            action = self.path.removeprefix("/v1/")
            if action == "execute":
                authorize(data)
                self.send_response(200)
                self.send_header("Content-Type", "application/x-ndjson")
                self.end_headers()
                streaming = True

                def write(event):
                    self.wfile.write(json.dumps(event).encode() + b"\n")
                    self.wfile.flush()

                asyncio.run(execute(data, write))
            else:
                result = asyncio.run(lifecycle(action, data))
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps(result).encode())
        except Exception as error:
            if not streaming:
                self.send_error(502, type(error).__name__)
            else:
                detail = str(error)
                for secret in (TOKEN, os.environ.get("OPEN_SANDBOX_API_KEY", ""), data.get("token", "")):
                    if secret:
                        detail = detail.replace(secret, "[REDACTED]")
                try:
                    self.wfile.write(
                        json.dumps(
                            {"kind": "adapter.error", "data": {"type": type(error).__name__, "error": detail[:1000]}}
                        ).encode()
                        + b"\n"
                    )
                    self.wfile.flush()
                except OSError:
                    pass
            # No stream retry: execution may already have produced side effects.


def main():
    if len(TOKEN) < 32:
        raise ValueError("CATENA_MANAGED_RUNNER_TOKEN must have at least 32 characters")
    host, port = os.environ.get("CATENA_MANAGED_RUNNER_ADDR", "127.0.0.1:8792").rsplit(":", 1)
    ThreadingHTTPServer((host, int(port)), Handler).serve_forever()


if __name__ == "__main__":
    main()
