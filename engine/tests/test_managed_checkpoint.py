import base64
import io
import tarfile

import pytest

from catena_engine.managed.entry import restore, snapshot


def test_checkpoint_roundtrip_includes_workspace_and_conversation(tmp_path):
    workspace, home = tmp_path / "workspace", tmp_path / "codex"
    workspace.mkdir()
    (workspace / "hello.py").write_text('print("hello catena")\n')
    (home / "sessions").mkdir(parents=True)
    (home / "sessions" / "conversation.jsonl").write_text('{"role":"user"}\n')
    (home / "thread-id").write_text("thread-test")
    (home / "config.toml").write_text("secret config is excluded")
    encoded = snapshot(workspace, home)
    new_workspace, new_home = tmp_path / "new-workspace", tmp_path / "new-home"
    restore(encoded, new_workspace, new_home)
    assert (new_workspace / "hello.py").read_text() == 'print("hello catena")\n'
    assert (new_home / "sessions" / "conversation.jsonl").exists()
    assert (new_home / "thread-id").read_text() == "thread-test"
    assert not (new_home / "config.toml").exists()


@pytest.mark.parametrize(
    "name,kind",
    [
        ("workspace/../../escape", tarfile.REGTYPE),
        ("/tmp/escape", tarfile.REGTYPE),
        ("workspace/link", tarfile.SYMTYPE),
        ("codex/device", tarfile.CHRTYPE),
    ],
)
def test_checkpoint_rejects_unsafe_archive(tmp_path, name, kind):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w:gz") as archive:
        item = tarfile.TarInfo(name)
        item.type = kind
        item.linkname = "/tmp/escape"
        archive.addfile(item)
    with pytest.raises(ValueError):
        restore(base64.b64encode(output.getvalue()).decode(), tmp_path / "workspace", tmp_path / "codex")


def test_snapshot_does_not_follow_workspace_symlinks(tmp_path):
    workspace, home = tmp_path / "workspace", tmp_path / "home"
    workspace.mkdir()
    secret = tmp_path / "outside"
    secret.write_text("must not escape")
    (workspace / "link").symlink_to(secret)
    encoded = snapshot(workspace, home)
    with tarfile.open(fileobj=io.BytesIO(base64.b64decode(encoded)), mode="r:gz") as archive:
        assert not archive.getmembers()


def test_dev_stop_does_not_signal_reused_pid(tmp_path, monkeypatch):
    import importlib.util
    from pathlib import Path
    from types import SimpleNamespace

    path = Path(__file__).resolve().parents[2] / "deploy/managed/dev.py"
    spec = importlib.util.spec_from_file_location("managed_dev", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    monkeypatch.setattr(module, "STATE", tmp_path)
    (tmp_path / "core.pid").write_text("1234")
    monkeypatch.setattr(
        module.subprocess, "run", lambda *_args, **_kwargs: SimpleNamespace(stdout="unrelated-process", returncode=0)
    )
    signals = []
    monkeypatch.setattr(module.os, "kill", lambda pid, sig: signals.append((pid, sig)))
    module.stop()
    assert signals == []


def test_codex_tools_keep_a_path_without_inheriting_platform_secrets():
    import tomllib

    from catena_engine.protocol import JsonRequest, ModelConfig
    from catena_engine.runtime.codex import _config, agent_profile

    request = JsonRequest("run", "run", "", 1, ModelConfig("catena", "http://192.0.2.1:18788/internal/managed/model", "test", "token"))
    value = tomllib.loads(_config(request, agent_profile("managed")))
    assert value["shell_environment_policy"]["inherit"] == "none"
    assert "/usr/bin" in value["shell_environment_policy"]["set"]["PATH"].split(":")
    assert value["model_providers"]["catena"]["env_key"] == "CATENA_TURN_API_KEY"
