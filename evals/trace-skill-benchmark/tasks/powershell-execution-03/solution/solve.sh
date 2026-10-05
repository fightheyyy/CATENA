#!/bin/bash
set -euxo pipefail
cd /workspace/case
pwsh -NoProfile -NonInteractive -Command "Get-ChildItem | Out-Null"
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "names": [\n    "O\'"'"'Reilly",\n    "alpha",\n    "beta",\n    "delta"\n  ]\n}\n'"'"')'
