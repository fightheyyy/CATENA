#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;p=pathlib.Path('"'"'service.json'"'"');s=p.read_text();assert '"'"'"retries": 2'"'"' in s;p.write_text(s.replace('"'"'"retries": 2'"'"','"'"'"retries": 5'"'"'))'
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "updated": true\n}\n'"'"')'
