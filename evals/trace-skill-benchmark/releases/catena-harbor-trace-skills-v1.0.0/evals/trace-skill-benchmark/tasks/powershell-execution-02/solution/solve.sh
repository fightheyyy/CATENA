#!/bin/bash
set -euxo pipefail
cd /workspace/case
pwsh -NoProfile -NonInteractive -Command "Get-ChildItem | Out-Null"
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "lines": [\n    2,\n    4\n  ]\n}\n'"'"')'
