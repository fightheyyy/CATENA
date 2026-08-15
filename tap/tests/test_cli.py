from catena_tap.cli import build_parser, main


def test_only_accepted_runtimes_are_advertised():
    help_text = build_parser().format_help()
    assert "codexapp" not in help_text
    assert "hermes" not in help_text
    assert "openclaw" not in help_text


def test_hook_fail_open_on_empty_input(monkeypatch):
    monkeypatch.setattr("sys.stdin.read", lambda: "")
    # json.load raises at the hook boundary and the CLI deliberately returns 0.
    assert main(["trace", "hook", "claude"]) == 0
