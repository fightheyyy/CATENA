#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "path": "services/atlas/config/active.json",\n  "port": 7101,\n  "handler": "handle_1"\n}\n'"'"')'
