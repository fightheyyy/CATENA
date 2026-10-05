#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "path": "apps/beacon/runtime/active.json",\n  "port": 7102,\n  "handler": "handle_2"\n}\n'"'"')'
