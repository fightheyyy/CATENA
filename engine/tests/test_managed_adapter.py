import asyncio
import json
from types import SimpleNamespace

import pytest

from catena_engine.managed import server


@pytest.mark.parametrize("crash", [False, True])
def test_adapter_does_not_publish_result_before_command_exit(monkeypatch, crash):
    async def scenario():
        result_written, permit_exit = asyncio.Event(), asyncio.Event()
        written = []
        data = {
            "run_id": "run-test",
            "sandbox_id": "box",
            "provider_id": "provider",
            "session_id": "session",
            "tenant_id": "tenant",
            "namespace": "deployment",
            "timeout": 10,
        }

        async def run(_command, *, opts, handlers):
            await handlers.on_stdout(SimpleNamespace(text=json.dumps({"kind": "result", "success": True}) + "\n"))
            result_written.set()
            await permit_exit.wait()
            if crash:
                raise ConnectionError("adapter lost command stream")
            return SimpleNamespace(error=None, complete=object(), exit_code=0)

        async def info():
            return SimpleNamespace(
                metadata={
                    "catena.sandbox": "box",
                    "catena.session": "session",
                    "catena.tenant": "tenant",
                    "catena.namespace": "deployment",
                }
            )

        async def nothing(*_args, **_kwargs):
            return None

        sandbox = SimpleNamespace(
            commands=SimpleNamespace(run=run), get_info=info, files=SimpleNamespace(write_files=nothing), close=nothing
        )

        async def connect(*_args, **_kwargs):
            return sandbox

        monkeypatch.setattr(server.Sandbox, "connect", connect)
        monkeypatch.setattr(server, "authorize", lambda _: None)
        monkeypatch.setattr(server, "connection", lambda: None)
        task = asyncio.create_task(server.execute(data, written.append))
        await asyncio.wait_for(result_written.wait(), 1)
        assert written == [], "early stdout result escaped before SDK command exit"
        permit_exit.set()
        if crash:
            with pytest.raises(ConnectionError):
                await task
            assert written == []
        else:
            await task
            assert written == [{"kind": "result", "success": True, "command_exited": True, "exit_code": 0}]

    asyncio.run(scenario())
