#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "tracked_secret_files": [\n    ".env"\n  ]\n}\n'"'"')'
