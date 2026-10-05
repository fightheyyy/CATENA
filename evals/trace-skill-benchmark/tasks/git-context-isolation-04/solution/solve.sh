#!/bin/bash
set -euxo pipefail
cd /workspace/case
git -C project worktree add -b isolated-fix /workspace/case/isolated
python3 -c 'import pathlib;p=pathlib.Path('"'"'isolated/config.json'"'"');s=p.read_text();assert '"'"'8200'"'"' in s;p.write_text(s.replace('"'"'8200'"'"','"'"'8300'"'"'))'
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "updated_in_isolation": true\n}\n'"'"')'
