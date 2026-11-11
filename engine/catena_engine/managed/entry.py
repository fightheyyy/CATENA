"""Runs inside the user sandbox as uid 1000; never on the control-plane host."""

from __future__ import annotations

import asyncio
import base64
import io
import json
import sys
import tarfile
import tempfile
from pathlib import Path

from ..contracts import OptimizationError
from ..harness_transport import private_post
from ..protocol import JsonRequest, ModelConfig
from ..runtime.codex import CodexRuntime, agent_profile

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



def main():
    request_path = Path(sys.argv[1])
    request = json.loads(request_path.read_text())
    request_path.unlink()
    workspace, home = Path("/workspace"), Path("/home/agent/.codex")
    workspace.mkdir(exist_ok=True)
    home.mkdir(parents=True, exist_ok=True)
    profile = agent_profile(request["profile"], request["profile_version"], request["instructions"],
                            request["tool_policy"], request["skills"] or (), request["knowledge_refs"] or ())
    if profile.persistent:
        restore(request.get("checkpoint"), workspace, home)
    runtime = CodexRuntime(ModelConfig("catena", request["gateway"], request["model"], request["token"]))
    runtime.preflight()
    emit("runtime", data={"type": "runtime.started", "run_id": request["run_id"], "generation": request["generation"],
                          "protocol": "codex-app-server", "profile": profile.kind, "profile_version": profile.version})
    turn = JsonRequest(request["run_id"], request["run_id"], request["input"], request["timeout"] * 1000, runtime.model)

    async def tool(name, arguments):
        return await asyncio.to_thread(private_post, request["tool_gateway"], request["token"],
                                       {"tool": name, "arguments": arguments}, request["timeout"], 128 * 1024)

    async def evaluate(arguments):
        return await tool("evaluate_candidate", arguments)

    thread = home / "thread-id"
    try:
        with tempfile.TemporaryDirectory(prefix="catena-turn-") as directory:
            reply = asyncio.run(runtime._run(
                turn, tool if profile.kind == "analyze" else evaluate, Path(directory) / "agent",
                profile=profile, workspace=workspace, codex_home=home if profile.persistent else None,
                thread=thread.read_text().strip() if profile.persistent and thread.exists() else None,
                event=lambda item: emit("runtime", data=item),
            ))
    except OptimizationError as error:
        # A failed protocol does not prove that the Sandbox has stopped.
        emit("runtime", data={"type": "runtime.failed", "error": str(error)})
        raise SystemExit(1) from None
    if profile.persistent:
        thread.write_text(reply["thread_id"])
        result, checkpoint = reply["value"], snapshot(workspace, home)
    else:
        result, checkpoint = json.dumps(reply, ensure_ascii=False), ""
    emit("result", success=True, result=result, snapshot=checkpoint)


if __name__ == "__main__":
    main()
