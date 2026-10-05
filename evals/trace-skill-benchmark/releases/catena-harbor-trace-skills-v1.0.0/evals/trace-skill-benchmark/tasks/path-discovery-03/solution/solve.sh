#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "path": "packages/courier/settings/active.json",\n  "port": 7103,\n  "handler": "handle_3"\n}\n'"'"')'
