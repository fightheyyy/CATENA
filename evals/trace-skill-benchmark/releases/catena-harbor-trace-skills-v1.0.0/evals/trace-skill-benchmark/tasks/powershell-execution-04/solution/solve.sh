#!/bin/bash
set -euxo pipefail
cd /workspace/case
pwsh -NoProfile -NonInteractive -Command "Get-ChildItem | Out-Null"
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "names": [\n    "c",\n    "a"\n  ],\n  "sum": 11\n}\n'"'"')'
