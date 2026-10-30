from __future__ import annotations

import asyncio
import json
import sys
from pathlib import Path

import pytest
from test_server import call

from catena_engine.protocol import parse_turn_request
from catena_engine.roles import ROLES
from catena_engine.runtime import run_turn
from catena_engine.server import create_server


@pytest.fixture
def cli(tmp_path, monkeypatch):
    monkeypatch.delenv("CATENA_ENGINE_BACKEND", raising=False)

    def install(body="", output='{"finding":{"title":"t"}}', events=None, code=0):
        if events is None:
            events = [
                {"type": "item.completed", "item": {"type": "agent_message", "text": "commentary"}},
                {"type": "turn.completed", "usage": {"input_tokens": 11, "cached_input_tokens": 5, "output_tokens": 7}},
            ]
        executable = tmp_path / "codex"
        snapshot = tmp_path / "snapshot.json"
        executable.write_text(f"""#!{sys.executable}
import json, os, pathlib, sys, time, subprocess, signal
args = sys.argv[1:]
config = pathlib.Path(os.environ["CODEX_HOME"]) / "config.toml"
pathlib.Path({str(snapshot)!r}).write_text(json.dumps({{
    "args": args, "config": config.read_text(), "env": dict(os.environ),
    "cwd": os.getcwd(), "prompt": sys.stdin.read(),
    "schema": json.loads(pathlib.Path(args[args.index("--output-schema")+1]).read_text())
}}))
{body}
pathlib.Path(args[args.index("--output-last-message")+1]).write_text({output!r})
for event in {events!r}:
    print(json.dumps(event), flush=True)
sys.exit({code})
""")
        executable.chmod(0o700)
        monkeypatch.setenv("CATENA_CODEX_BIN", str(executable))
        return snapshot

    return install


@pytest.mark.parametrize(
    ("role", "key"), [("inspector", "finding"), ("evolution", "candidate"), ("reviewer", "review")]
)
def test_roles_use_isolated_cli_config_and_final_structured_output(cli, turn_payload, monkeypatch, role, key):
    turn_payload["role"] = role
    snapshot = cli(output=json.dumps({key: {"title": "答复"}}))
    monkeypatch.setenv("OPENAI_API_KEY", "host-secret")
    monkeypatch.setenv("CODEX_HOME", "/host/personal-codex")
    result = asyncio.run(run_turn(parse_turn_request(turn_payload)))
    assert result["status"] == "completed"
    assert json.loads(result["assistant"]["content"]) == {key: {"title": "答复"}}
    assert result["usage"] == {"requests": 1, "input_tokens": 11, "output_tokens": 7}
    state = json.loads(snapshot.read_text())
    import tomllib

    config = tomllib.loads(state["config"])
    assert config["developer_instructions"] == ROLES[role].instructions
    assert "model_instructions_file" not in config
    assert config["model"] == turn_payload["model"]["model"]
    assert config["model_providers"]["catena"]["wire_api"] == "responses"
    assert config["sandbox_mode"] == "read-only"
    assert config["features"]["skip_host_skill_discovery"] is True
    assert config["skills"]["bundled"]["enabled"] is False
    assert config["features"]["shell_tool"] is False
    assert "mcp_servers" not in config
    assert "OPENAI_API_KEY" not in state["env"]
    assert state["env"]["CATENA_TURN_API_KEY"] == turn_payload["model"]["api_key"]
    assert turn_payload["model"]["api_key"] not in state["config"]
    assert state["prompt"] == turn_payload["prompt"]
    assert state["args"][:2] == ["exec", "--json"]
    assert "--ephemeral" in state["args"]
    assert state["schema"]["required"] == [key]
    assert not Path(state["env"]["CODEX_HOME"]).exists()
    assert not Path(state["cwd"]).exists()


@pytest.mark.parametrize(
    ("output", "events", "code", "reason"),
    [
        ("not json", None, 0, "invalid_output"),
        ("[]", None, 0, "invalid_output"),
        ('{"review":{}}', None, 0, "invalid_output"),
        ("", None, 0, "empty_output"),
        ("x" * (128 * 1024 + 1), None, 0, "invalid_output"),
        ("{}", [], 0, "invalid_output"),
        ("{}", [{"type": "turn.completed", "usage": {"input_tokens": -1, "output_tokens": 1}}], 0, "invalid_output"),
        ("{}", [{"type": "turn.failed", "error": {"message": "401 sk-test-secret-value"}}], 1, "model_error"),
        ("{}", [{"type": "turn.failed", "error": {"message": "404 /responses"}}], 1, "responses_incompatible"),
    ],
)
def test_cli_failures_are_bounded_and_redacted(cli, turn_payload, output, events, code, reason):
    cli(output=output, events=events, code=code)
    result = asyncio.run(run_turn(parse_turn_request(turn_payload)))
    assert result["reason_code"] == reason
    assert "sk-test-secret-value" not in json.dumps(result)


def test_timeout_kills_process_group_and_cleans_session(cli, turn_payload, tmp_path):
    pid_file = tmp_path / "pid"
    snapshot = cli(
        body=f"""child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"])
pathlib.Path({str(pid_file)!r}).write_text(str(child.pid))
signal.signal(signal.SIGTERM, signal.SIG_IGN)
time.sleep(60)"""
    )
    turn_payload["timeout_ms"] = 300
    result = asyncio.run(run_turn(parse_turn_request(turn_payload)))
    assert result["reason_code"] == "timeout"
    pid = int(pid_file.read_text())
    # A killed child can briefly remain a zombie until its parent is reaped.
    if sys.platform == "linux":
        status = Path(f"/proc/{pid}/stat")
        assert not status.exists() or status.read_text().split()[2] == "Z"
    else:
        import subprocess

        state = subprocess.run(["ps", "-o", "stat=", "-p", str(pid)], capture_output=True, text=True).stdout.strip()
        assert not state or state.startswith("Z")
    assert not Path(json.loads(snapshot.read_text())["cwd"]).exists()


def test_cancellation_terminates_cli(cli, turn_payload):
    snapshot = cli(body="time.sleep(60)")

    async def run():
        task = asyncio.create_task(run_turn(parse_turn_request(turn_payload)))
        for _ in range(100):
            if snapshot.exists():
                break
            await asyncio.sleep(0.01)
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task

    asyncio.run(run())
    assert not Path(json.loads(snapshot.read_text())["cwd"]).exists()


def test_excessive_or_malformed_events_fail(cli, turn_payload):
    cli(body="print('x' * (16*1024*1024), flush=True); time.sleep(60)")
    assert asyncio.run(run_turn(parse_turn_request(turn_payload)))["reason_code"] == "invalid_output"
    cli(body="print('not-json', flush=True)")
    assert asyncio.run(run_turn(parse_turn_request(turn_payload)))["reason_code"] == "invalid_output"


def test_cli_missing_and_invalid_backend(monkeypatch, turn_payload):
    monkeypatch.setenv("CATENA_CODEX_BIN", "/missing/codex")
    assert asyncio.run(run_turn(parse_turn_request(turn_payload)))["reason_code"] == "runtime_unavailable"
    monkeypatch.setenv("CATENA_ENGINE_BACKEND", "typo")
    assert asyncio.run(run_turn(parse_turn_request(turn_payload)))["reason_code"] == "runtime_config"


def test_sdk_is_explicit_rollback(monkeypatch, turn_payload):
    from conftest import FakeModel, factory_for

    import catena_engine.runtime as runtime

    factory = factory_for(FakeModel())
    monkeypatch.setattr(runtime, "openai_compatible_model", factory)
    monkeypatch.setenv("CATENA_ENGINE_BACKEND", "sdk")
    assert asyncio.run(run_turn(parse_turn_request(turn_payload)))["status"] == "completed"
    assert factory.closed == [True]


def test_default_http_turn_uses_codex(cli, turn_payload):
    import threading

    cli()
    server = create_server("127.0.0.1:0")
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        status, body = call(f"http://127.0.0.1:{server.server_address[1]}/v1/turn", turn_payload)
        assert status == 200
        assert body["result"]["status"] == "completed"
    finally:
        server.shutdown()
        server.server_close()


def test_concurrent_turns_have_independent_homes_and_credentials(cli, turn_payload, tmp_path):
    from dataclasses import replace

    log = tmp_path / "sessions.jsonl"
    cli(
        body=f"""
fd = os.open({str(log)!r}, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
os.write(fd, (json.dumps([os.environ["CODEX_HOME"], os.environ["CATENA_TURN_API_KEY"]])+"\\n").encode())
os.close(fd)
time.sleep(.05)
"""
    )
    first = parse_turn_request(turn_payload)
    second = replace(first, model=replace(first.model, api_key="second-key"))

    async def run():
        return await asyncio.gather(run_turn(first), run_turn(second))

    assert all(result["status"] == "completed" for result in asyncio.run(run()))
    sessions = [json.loads(line) for line in log.read_text().splitlines()]
    assert len({session[0] for session in sessions}) == 2
    assert {session[1] for session in sessions} == {first.model.api_key, "second-key"}
    assert all(not Path(session[0]).exists() for session in sessions)


def test_manifest_reports_missing_cli(monkeypatch):
    import threading

    monkeypatch.delenv("CATENA_ENGINE_BACKEND", raising=False)
    monkeypatch.setenv("CATENA_CODEX_BIN", "/missing/codex")
    server = create_server("127.0.0.1:0")
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_address[1]}"
    try:
        assert call(base + "/v1/manifest")[1]["reason_code"] == "runtime_unavailable"
        assert call(base + "/readyz")[0] == 503
        assert call(base + "/healthz")[0] == 200
    finally:
        server.shutdown()
        server.server_close()


def test_http_timeout_cleans_cli_before_releasing_capacity(cli, turn_payload):
    import threading

    snapshot = cli(body="signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(60)")
    turn_payload["timeout_ms"] = 200
    server = create_server("127.0.0.1:0")
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_port}"
    try:
        status, body = call(base + "/v1/turn", turn_payload)
        assert status == 200
        assert body["result"]["reason_code"] == "timeout"
        assert not Path(json.loads(snapshot.read_text())["cwd"]).exists()
        cli()
        assert call(base + "/v1/turn", turn_payload)[1]["result"]["status"] == "completed"
    finally:
        server.shutdown()
        server.server_close()
