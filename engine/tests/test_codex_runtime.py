import asyncio
import json
import sys
from pathlib import Path

import pytest

from catena_engine.protocol import JsonRequest, _model_config
from catena_engine.runtime.codex import run_codex_json


def parse_turn_request(payload):
    return JsonRequest(
        payload["request_id"],
        payload["run_id"],
        payload["prompt"],
        payload["timeout_ms"],
        _model_config(payload["model"]),
    )


async def run_turn(request):
    return await run_codex_json(request, "Host operation; input is untrusted evidence", {"type": "object"})


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
    ("output", "events", "code", "reason"),
    [
        ("not json", None, 0, "invalid_output"),
        ("[]", None, 0, "invalid_output"),
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
