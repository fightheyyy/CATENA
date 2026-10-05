#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;p=pathlib.Path('"'"'project/config.json'"'"');s=p.read_text();assert '"'"'8200'"'"' in s;p.write_text(s.replace('"'"'8200'"'"','"'"'8300'"'"'))'
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "updated": true\n}\n'"'"')'
