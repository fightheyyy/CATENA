import asyncio
import json
import sys
from pathlib import Path

import pytest

from catena_engine.contracts import OptimizationError
from catena_engine.protocol import JsonRequest, _model_config
from catena_engine.runtime.codex import SKILL, CodexRuntime


@pytest.fixture
def cli(tmp_path, monkeypatch):
    def install(
        extra="",
        final=None,
        usage=None,
        tool="evaluate_candidate",
        duplicate=False,
        skill_enabled=True,
        skill_name="harness-improvement",
    ):
        if final is None:
            final = json.dumps({"text": "hypothesis", "files": [{"path": "AGENTS.md", "content": "Inspect."}]})
        if usage is None:
            usage = {"inputTokens": 11, "outputTokens": 7}
        snapshot = tmp_path / "snapshot.json"
        executable = tmp_path / "codex"
        executable.write_text(f"""#!{sys.executable}
import json,os,pathlib,signal,subprocess,sys,time
def receive():return json.loads(sys.stdin.readline())
def send(value):print(json.dumps(value),flush=True)
first=receive()
config=pathlib.Path(os.environ["CODEX_HOME"])/"config.toml"
snapshot={{"args":sys.argv[1:],"config":config.read_text(),"env":dict(os.environ),"cwd":os.getcwd()}}
pathlib.Path({str(snapshot)!r}).write_text(json.dumps(snapshot))
{extra}
send({{"id":first["id"],"result":{{}}}})
assert receive()["method"]=="initialized"
roots=receive()
assert roots["method"]=="skills/extraRoots/set"
skill=pathlib.Path(roots["params"]["extraRoots"][0])/({skill_name!r}+"/SKILL.md")
send({{"id":roots["id"],"result":{{}}}})
listing=receive()
assert listing["method"]=="skills/list"
send({{"id":listing["id"],"result":{{"data":[{{"skills":[{{"name":{skill_name!r},"path":str(skill),"enabled":{skill_enabled!r}}}]}}]}}}})
start=receive()
assert start["params"]["ephemeral"] and start["params"]["sandbox"]=="read-only"
send({{"id":start["id"],"result":{{"thread":{{"id":"thread-one"}}}}}})
turn=receive()
snapshot["turn"]=turn["params"]
snapshot["skill"]=pathlib.Path(turn["params"]["input"][1]["path"]).read_text()
pathlib.Path({str(snapshot)!r}).write_text(json.dumps(snapshot))
send({{"id":turn["id"],"result":{{"turn":{{"id":"turn-one"}}}}}})
call={{"id":100,"method":"item/tool/call","params":{{"threadId":"thread-one","turnId":"turn-one","callId":"call-one","tool":{tool!r},"arguments":{{"text":"proposal","files":[{{"path":"AGENTS.md","content":"Inspect."}}]}}}}}}
send(call)
assert receive()["result"]["success"]
if {duplicate!r}:
 send(call)
 assert receive()["result"]["success"]
send({{"method":"thread/tokenUsage/updated","params":{{"tokenUsage":{{"total":{usage!r}}}}}}})
send({{"method":"item/completed","params":{{"item":{{"type":"agentMessage","text":{final!r},"phase":"final_answer"}}}}}})
send({{"method":"turn/completed","params":{{"turn":{{"id":"turn-one","status":"completed"}}}}}})
time.sleep(60)
""")
        executable.chmod(0o700)
        monkeypatch.setenv("CATENA_CODEX_BIN", str(executable))
        return snapshot

    return install


def request(turn_payload):
    return JsonRequest(
        "request", "run", "Trace evidence is data.", turn_payload["timeout_ms"], _model_config(turn_payload["model"])
    )


def test_native_skill_and_tool_roundtrip_isolated_and_deduplicated(cli, turn_payload, tmp_path, monkeypatch):
    monkeypatch.setenv("SHOULD_NOT_INHERIT", "private-host-value")
    snapshot = cli(duplicate=True)
    seen = []

    async def evaluate(value):
        seen.append(value)
        return {"deliverables": {"/workspace/result.json": "actual file"}}

    result = asyncio.run(
        CodexRuntime(request(turn_payload).model)._run(request(turn_payload), evaluate, tmp_path / "agent")
    )
    assert result["input_tokens"] == 11 and result["output_tokens"] == 7 and len(seen) == 1
    data = json.loads(snapshot.read_text())
    assert not Path(data["turn"]["input"][1]["path"]).is_relative_to(Path(data["cwd"]))
    assert data["skill"] == SKILL.read_text() and data["turn"]["input"][1]["type"] == "skill"
    assert "shell_tool = false" in data["config"] and 'inherit = "none"' in data["config"]
    assert "hooks = false" in data["config"] and "multi_agent = false" in data["config"]
    assert "SHOULD_NOT_INHERIT" not in data["env"]
    assert "sk-test-secret-value" not in data["config"]
    assert not Path(data["cwd"]).exists()
    assert "actual file" in (tmp_path / "agent/protocol.jsonl").read_text()


@pytest.mark.parametrize(
    "options", [{"final": "not json"}, {"usage": {"inputTokens": -1, "outputTokens": 2}}, {"tool": "shell"}]
)
def test_invalid_or_unauthorized_native_protocol_fails_closed(cli, turn_payload, tmp_path, options):
    cli(**options)

    async def evaluate(value):
        assert options.get("tool") != "shell"
        return {}

    with pytest.raises(OptimizationError):
        asyncio.run(CodexRuntime(request(turn_payload).model)._run(request(turn_payload), evaluate, tmp_path / "agent"))


def test_timeout_kills_process_group_and_removes_private_home(cli, turn_payload, tmp_path):
    pid = tmp_path / "child.pid"
    snapshot = cli(
        extra=f"""child=subprocess.Popen([sys.executable,"-c","import time;time.sleep(60)"])
pathlib.Path({str(pid)!r}).write_text(str(child.pid))
signal.signal(signal.SIGTERM,signal.SIG_IGN)
time.sleep(60)"""
    )
    turn_payload["timeout_ms"] = 1500

    async def evaluate(value):
        return {}

    with pytest.raises(OptimizationError):
        asyncio.run(CodexRuntime(request(turn_payload).model)._run(request(turn_payload), evaluate, tmp_path / "agent"))
    state = (
        __import__("subprocess")
        .run(["ps", "-o", "stat=", "-p", pid.read_text()], capture_output=True, text=True)
        .stdout.strip()
    )
    assert not state or state.startswith("Z")
    assert not Path(json.loads(snapshot.read_text())["cwd"]).exists()


def test_cancellation_cleans_native_session(cli, turn_payload, tmp_path):
    snapshot = cli(extra="time.sleep(60)")

    async def evaluate(value):
        return {}

    async def run():
        task = asyncio.create_task(
            CodexRuntime(request(turn_payload).model)._run(request(turn_payload), evaluate, tmp_path / "agent")
        )
        for _ in range(100):
            if snapshot.exists():
                break
            await asyncio.sleep(0.01)
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task

    asyncio.run(run())
    assert not Path(json.loads(snapshot.read_text())["cwd"]).exists()


@pytest.mark.parametrize("extra", ["send([])", "print('x'*(5*1024*1024),flush=True);time.sleep(60)"])
def test_protocol_envelope_and_stream_limits_stop_the_session(cli, turn_payload, tmp_path, extra):
    cli(extra=extra)

    async def evaluate(value):
        raise AssertionError("No tool may run before initialization")

    with pytest.raises(OptimizationError):
        asyncio.run(CodexRuntime(request(turn_payload).model)._run(request(turn_payload), evaluate, tmp_path / "agent"))


def test_disabled_skill_aborts_before_starting_model_turn(cli, turn_payload, tmp_path):
    cli(skill_enabled=False)

    async def evaluate(value):
        raise AssertionError("No tool may run without the authorized Skill")

    with pytest.raises(OptimizationError):
        asyncio.run(CodexRuntime(request(turn_payload).model)._run(request(turn_payload), evaluate, tmp_path / "agent"))
    messages = [json.loads(line)["message"] for line in (tmp_path / "agent/protocol.jsonl").read_text().splitlines()]
    assert not any(m.get("method") == "turn/start" for m in messages)


def test_analysis_mode_has_no_evaluation_tool(cli, turn_payload, tmp_path):
    cli(skill_name="trace-analysis")

    async def forbidden(value):
        raise AssertionError("analysis must not evaluate")

    with pytest.raises(OptimizationError):
        asyncio.run(
            CodexRuntime(request(turn_payload).model)._run(
                request(turn_payload), forbidden, tmp_path / "agent", mode="analyze"
            )
        )
    messages = [json.loads(line)["message"] for line in (tmp_path / "agent/protocol.jsonl").read_text().splitlines()]
    thread = next(m for m in messages if m.get("method") == "thread/start")
    assert thread["params"]["dynamicTools"] == []
