#!/bin/bash
set -euxo pipefail
cd /workspace/case
rm legacy.py
python3 -c 'import pathlib;p=pathlib.Path('"'"'app.py'"'"');s=p.read_text();assert '"'"'from legacy import title\n\ndef render():\n    return title()\n'"'"' in s;p.write_text(s.replace('"'"'from legacy import title\n\ndef render():\n    return title()\n'"'"','"'"'def render():\n    return "current"\n'"'"'))'
python3 -c 'import pathlib;pathlib.Path("result.json").write_text('"'"'{\n  "updated": true\n}\n'"'"')'
