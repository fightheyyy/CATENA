#!/bin/bash
set -euxo pipefail
cd /workspace/case
python3 -c 'import pathlib;p=pathlib.Path('"'"'README.md'"'"');s=p.read_text();assert '"'"'Use install-old.'"'"' in s;p.write_text(s.replace('"'"'Use install-old.'"'"','"'"'Use install-current.'"'"'))'
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "updated": true\n}\n'"'"')'
