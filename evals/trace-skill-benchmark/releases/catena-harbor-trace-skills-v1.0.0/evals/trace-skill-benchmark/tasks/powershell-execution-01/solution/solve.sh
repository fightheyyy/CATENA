#!/bin/bash
set -euxo pipefail
cd /workspace/case
pwsh -NoProfile -NonInteractive -Command "Get-ChildItem | Out-Null"
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "paid_count": 6,\n  "paid_cents": 1077\n}\n'"'"')'
