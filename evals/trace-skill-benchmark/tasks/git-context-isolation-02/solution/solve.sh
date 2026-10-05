#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "repo": "apps/beacon",\n  "branch": "main",\n  "tracked": [\n    "config.json",\n    "notes.txt"\n  ]\n}\n'"'"')'
