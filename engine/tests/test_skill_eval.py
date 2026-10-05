import os
import shutil
from pathlib import Path

import pytest

from catena_engine.skill_eval import execute

ROOT = Path(__file__).resolve().parents[2] / "evals" / "rg-missing-search"
TOOL_PATH = r"C:\Windows\System32;C:\Windows;C:\Windows\System32\WindowsPowerShell\v1.0;C:\Program Files\PowerShell\7"
pytestmark = pytest.mark.skipif(os.name != "nt", reason="Windows PowerShell Case")


def test_exact_tool_reproduces_missing_rg_and_reads_fixture(tmp_path):
    workspace = tmp_path / "workspace"
    shutil.copytree(ROOT / "fixtures" / "command-handler", workspace)
    missing = execute("rg -n analyze-log src", workspace, ROOT / "read_only_shell.ps1", TOOL_PATH)
    assert missing["exit_code"] != 0 and "CommandNotFoundException" in missing["stderr"]
    found = execute("Get-ChildItem src -Recurse -File | Select-String -SimpleMatch analyze-log",
                    workspace, ROOT / "read_only_shell.ps1", TOOL_PATH)
    assert found["exit_code"] == 0 and "inspect.ts" in found["stdout"]
    empty = execute("Get-ChildItem src -Recurse -File | Select-String -SimpleMatch missing-symbol",
                    workspace, ROOT / "read_only_shell.ps1", TOOL_PATH)
    assert empty["exit_code"] == 0 and not empty["stdout"].strip()


@pytest.mark.parametrize("command", [
    "Get-Content ../../README.md",
    "Get-Content C:/Windows/win.ini",
    "Get-Content src/commands/inspect.ts; Remove-Item src/commands/inspect.ts",
    "Get-Content $(Get-Content ../../README.md)",
    "Get-Content src/commands/inspect.ts > answer.txt",
    "& 'Get-Content' src/commands/inspect.ts",
])
def test_tool_rejects_escape_writes_and_dynamic_execution(command, tmp_path):
    workspace = tmp_path / "workspace"
    shutil.copytree(ROOT / "fixtures" / "command-handler", workspace)
    result = execute(command, workspace, ROOT / "read_only_shell.ps1", TOOL_PATH)
    assert result["exit_code"] != 0 and "EVAL_POLICY" in result["stderr"]
    assert (workspace / "src/commands/inspect.ts").is_file()
    assert not (workspace / "answer.txt").exists()
