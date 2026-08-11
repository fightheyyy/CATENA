import sys
from types import SimpleNamespace

from catena_tap import cli
from catena_tap.cli import upstream_argv


def test_upstream_argv_disables_redundant_viewer():
    assert upstream_argv("codex", ["--", "--full-auto"], tap_ui=False) == [
        "claude-tap",
        "--tap-client",
        "codex",
        "--tap-no-open",
        "--tap-no-live",
        "--",
        "--full-auto",
    ]


def test_upstream_argv_can_keep_debug_viewer():
    assert upstream_argv("hermes", [], tap_ui=True) == ["claude-tap", "--tap-client", "hermes"]


def test_wrapped_runtime_exit_code_is_preserved(monkeypatch):
    class Exporter:
        def __init__(self, *_args, **_kwargs):
            pass

        def close(self):
            return SimpleNamespace(captured_records=0, uploaded_traces=0, failed_uploads=0)

    def runtime_main():
        raise SystemExit(7)

    monkeypatch.setattr(cli, "BackgroundTraceExporter", Exporter)
    monkeypatch.setattr(cli, "_install_trace_hook", lambda _exporter: lambda: None)
    monkeypatch.setitem(sys.modules, "claude_tap.cli", SimpleNamespace(main_entry=runtime_main))

    assert cli.main(["tap", "codex", "--api-key", "catena_agent_test"]) == 7
