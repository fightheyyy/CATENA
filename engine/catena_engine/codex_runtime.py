"""Execute Catena's existing role turns in disposable Codex CLI sessions."""

from __future__ import annotations

import asyncio
import json
import os
import shutil
import signal
import tempfile
import time
from pathlib import Path
from typing import Any

from .protocol import TurnRequest
from .roles import ROLES
from .runtime import _elapsed_ms, _failed

# Bound CLI diagnostics as well as the assistant output below the Go HTTP limit.
_MAX_STREAM_BYTES = 4 * 1024 * 1024
_MAX_OUTPUT_BYTES = 128 * 1024


def _output_schema(role: str) -> dict[str, Any]:
    # Transport shapes only. Catena's Go prompts and validators continue to own
    # semantic constraints (including portable and DSH asset contracts).
    string = {"type": "string"}

    def obj(properties):
        return {"type": "object", "properties": properties, "required": list(properties), "additionalProperties": False}

    if role == "inspector":
        return obj(
            {
                "finding": obj(
                    {
                        "title": string,
                        "summary": string,
                        "severity": string,
                        "evidence": {"type": "array", "items": string},
                    }
                )
            }
        )
    if role == "evolution":
        return obj(
            {
                "candidate": obj(
                    {
                        "kind": string,
                        "title": string,
                        "summary": string,
                        "content": obj(
                            {
                                "root": string,
                                "files": {"type": "array", "items": obj({"path": string, "content": string})},
                            }
                        ),
                    }
                )
            }
        )
    return obj({"review": obj({"verdict": string, "summary": string})})


def _config(request: TurnRequest) -> str:
    # JSON strings are TOML basic strings for these validated text values.
    def q(value):
        return json.dumps(value, ensure_ascii=False)

    return f"""model = {q(request.model.model)}
model_provider = "catena"
model_reasoning_effort = {q(os.environ.get("CATENA_CODEX_REASONING_EFFORT", "low"))}
developer_instructions = {q(ROLES[request.role].instructions)}
approval_policy = "never"
sandbox_mode = "read-only"
project_doc_max_bytes = 0
web_search = "disabled"
[skills]
include_instructions = false
[skills.bundled]
enabled = false
[features]
shell_tool = false
enable_request_compression = false
apps = false
multi_agent = false
skip_host_skill_discovery = true
plugins = false
remote_plugin = false
hooks = false
workspace_dependencies = false
image_generation = false
browser_use = false
computer_use = false
[shell_environment_policy]
inherit = "none"
[model_providers.catena]
name = "Catena owner model"
base_url = {q(request.model.base_url.rstrip("/"))}
env_key = "CATENA_TURN_API_KEY"
wire_api = "responses"
request_max_retries = 0
stream_max_retries = 0
"""


async def _read_stream(stream: asyncio.StreamReader) -> bytes:
    data = bytearray()
    while chunk := await stream.read(65536):
        data.extend(chunk)
        if len(data) > _MAX_STREAM_BYTES:
            raise ValueError("CLI stream exceeded the size limit")
    return bytes(data)


async def _stop(process: asyncio.subprocess.Process) -> None:
    # Drain pipes after the bounded readers have been cancelled. Otherwise
    # asyncio's process.wait() can hang on a full pipe even after SIGKILL.
    async def discard(stream):
        while await stream.read(65536):
            pass

    waiter = asyncio.gather(process.wait(), discard(process.stdout), discard(process.stderr))
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        await asyncio.wait_for(asyncio.shield(waiter), 1)
    except TimeoutError:
        pass
    finally:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        await waiter


def _events(raw: bytes) -> tuple[dict[str, int], bool]:
    usage = {"requests": 0, "input_tokens": 0, "output_tokens": 0}
    failed = False
    for line in raw.splitlines():
        event = json.loads(line)
        if not isinstance(event, dict):
            raise ValueError("invalid CLI event")
        if event.get("type") == "turn.failed":
            failed = True
        if event.get("type") == "turn.completed":
            usage["requests"] += 1
            for key in ("input_tokens", "output_tokens"):
                value = event["usage"][key]
                if type(value) is not int or value < 0:
                    raise ValueError("invalid CLI usage")
                usage[key] += value
    return usage, failed


async def run_codex_turn(request: TurnRequest) -> dict[str, Any]:
    started = time.monotonic()
    binary = shutil.which(os.environ.get("CATENA_CODEX_BIN", "codex"))
    if binary is None:
        return _failed("runtime_unavailable", "Codex CLI is not installed", started)
    process = None
    tasks: list[asyncio.Task] = []
    try:
        with tempfile.TemporaryDirectory(prefix="catena-turn-") as directory:
            root = Path(directory)
            home, codex_home, workspace = (root / name for name in ("home", "codex", "workspace"))
            for path in (home, codex_home, workspace):
                path.mkdir(mode=0o700)
            (codex_home / "config.toml").write_text(_config(request), encoding="utf-8")
            schema, output = root / "schema.json", root / "answer.json"
            schema.write_text(json.dumps(_output_schema(request.role)), encoding="utf-8")
            # Do not inherit personal auth, config, proxies, plugins, or MCP.
            # The key is passed only to the selected provider, never argv/config.
            env = {
                "PATH": os.environ.get("PATH", "/usr/local/bin:/usr/bin:/bin"),
                "HOME": str(home),
                "CODEX_HOME": str(codex_home),
                "XDG_CONFIG_HOME": str(home / ".config"),
                "TMPDIR": str(root),
                "CATENA_TURN_API_KEY": request.model.api_key,
            }
            try:
                async with asyncio.timeout(request.timeout_ms / 1000):
                    process = await asyncio.create_subprocess_exec(
                        binary,
                        "exec",
                        "--json",
                        "--strict-config",
                        "--ephemeral",
                        "--skip-git-repo-check",
                        "--sandbox",
                        "read-only",
                        "--color",
                        "never",
                        "--cd",
                        str(workspace),
                        "--output-schema",
                        str(schema),
                        "--output-last-message",
                        str(output),
                        "-",
                        cwd=workspace,
                        env=env,
                        stdin=asyncio.subprocess.PIPE,
                        stdout=asyncio.subprocess.PIPE,
                        stderr=asyncio.subprocess.PIPE,
                        start_new_session=True,
                    )

                    async def feed() -> None:
                        try:
                            process.stdin.write(request.prompt.encode())
                            await process.stdin.drain()
                        except (BrokenPipeError, ConnectionResetError):
                            pass
                        finally:
                            process.stdin.close()

                    tasks = [
                        asyncio.create_task(_read_stream(process.stdout)),
                        asyncio.create_task(_read_stream(process.stderr)),
                        asyncio.create_task(feed()),
                    ]
                    stdout, stderr, _ = await asyncio.gather(*tasks)
                    code = await process.wait()
                usage, failed = _events(stdout)
                if code or failed:
                    # The actual Responses request is the compatibility check;
                    # never assume /chat/completions support implies /responses.
                    diagnostic = (stdout + stderr).lower()
                    if any(
                        marker in diagnostic
                        for marker in (
                            b"404",
                            b"405",
                            b"not supported",
                            b"unsupported",
                            b"not found",
                        )
                    ):
                        return _failed(
                            "responses_incompatible",
                            "the configured model endpoint rejected Codex Responses API; check /responses, streaming and structured output support, or select the SDK backend",
                            started,
                        )
                    return _failed(
                        "model_error",
                        "Codex Responses API turn failed; check the configured model, endpoint and credentials",
                        started,
                    )
                if not usage["requests"]:
                    return _failed("invalid_output", "Codex exited without a completed turn", started)
                if not output.exists() or not output.stat().st_size:
                    return _failed("empty_output", "Codex returned no final output", started)
                if output.stat().st_size > _MAX_OUTPUT_BYTES:
                    return _failed("invalid_output", "Codex final output exceeded the size limit", started)
                content = output.read_text(encoding="utf-8")
                parsed = json.loads(content)
                key = {"inspector": "finding", "evolution": "candidate", "reviewer": "review"}[request.role]
                if not isinstance(parsed, dict) or not isinstance(parsed.get(key), dict):
                    raise ValueError("invalid role output envelope")
                return {
                    "status": "completed",
                    "assistant": {"role": "assistant", "content": content},
                    "usage": usage,
                    "duration_ms": _elapsed_ms(started),
                }
            finally:
                for task in tasks:
                    task.cancel()
                await asyncio.gather(*tasks, return_exceptions=True)
                if process is not None:
                    await _stop(process)
    except TimeoutError:
        return _failed("timeout", "Codex did not answer before the turn timeout", started)
    except (ValueError, KeyError, TypeError, UnicodeError):
        return _failed("invalid_output", "Codex returned invalid JSON events, usage or role output", started)
    except OSError:
        return _failed("runtime_error", "Codex process could not be started or its output could not be read", started)
