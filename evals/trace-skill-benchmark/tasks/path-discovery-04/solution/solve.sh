#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "path": "components/delta/deploy/active.json",\n  "port": 7104,\n  "handler": "handle_4"\n}\n'"'"')'
