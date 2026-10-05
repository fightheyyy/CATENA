import asyncio
import json
from types import SimpleNamespace

import pytest

module = pytest.importorskip("catena_engine.harbor_e2b", exc_type=ImportError)


def environment(tmp_path):
    from harbor.models.task.config import NetworkMode

    value = object.__new__(module.CatenaE2BEnvironment)
    value.trial_paths = SimpleNamespace(trial_dir=tmp_path)
    value.environment_name = "test"
    value.session_id = "trial-test"
    value._template_name = "template-test"
    value._sandbox = None
    value._startup_env = lambda: {}
    value._sandbox_create_network_options = lambda: None
    value._network_policy = SimpleNamespace(network_mode=NetworkMode.PUBLIC)
    return value


def test_create_has_bounded_timeout_and_records_id(tmp_path, monkeypatch):
    value = environment(tmp_path)
    received = {}

    async def create(**kwargs):
        received.update(kwargs)
        return SimpleNamespace(sandbox_id="sandbox-test")

    monkeypatch.setattr(module.AsyncSandbox, "create", create)
    asyncio.run(value._create_sandbox())
    assert received["timeout"] == 1200
    assert received["metadata"]["platform"] == "catena"
    assert json.loads((tmp_path / "catena-e2b-lifecycle.json").read_text())["sandbox_id"] == "sandbox-test"


@pytest.mark.parametrize("fails", [False, True])
def test_cleanup_failure_is_visible(tmp_path, fails):
    value = environment(tmp_path)
    value._sandbox = object()

    async def stop():
        if fails:
            raise RuntimeError("cleanup failed")

    value._stop_sandbox = stop
    if fails:
        with pytest.raises(RuntimeError):
            asyncio.run(value.stop(True))
    else:
        asyncio.run(value.stop(True))
    record = json.loads((tmp_path / "catena-e2b-lifecycle.json").read_text())
    assert record["kill_confirmed"] is (not fails)
    assert value._sandbox is None
