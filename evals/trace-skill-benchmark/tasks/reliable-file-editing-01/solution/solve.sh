#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;p=pathlib.Path('"'"'managed/policy.py'"'"');s=p.read_text();assert '"'"'return 2'"'"' in s;p.write_text(s.replace('"'"'return 2'"'"','"'"'return 5'"'"'))'
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "updated": true\n}\n'"'"')'
