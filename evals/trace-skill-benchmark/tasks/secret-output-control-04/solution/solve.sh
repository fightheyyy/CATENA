#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "missing": [\n    "PORT",\n    "REGION"\n  ]\n}\n'"'"')'
