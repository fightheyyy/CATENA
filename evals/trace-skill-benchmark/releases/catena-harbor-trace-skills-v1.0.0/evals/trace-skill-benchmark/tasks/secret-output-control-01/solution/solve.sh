#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "configured": [\n    "API_KEY",\n    "DB_PASSWORD",\n    "PORT"\n  ]\n}\n'"'"')'
