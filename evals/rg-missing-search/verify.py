"""Frozen answers live outside the evaluated workspace."""
import json
import sys
from pathlib import Path

EXPECTED = {
    "command-handler": [{"path": "src/commands/inspect.ts", "value": "runLogInspection"}],
    "role-config": [{"path": "roles/production.yaml", "value": "model-silver"}],
    "absent-symbol": [],
}

answer = json.loads(Path("../agent.final.txt").read_text(encoding="utf-8"))
actual = answer.get("matches")
expected = EXPECTED[sys.argv[1]]
passed = isinstance(actual, list) and actual == expected and set(answer) == {"matches"}
print(json.dumps({"passed": passed, "case": sys.argv[1]}, ensure_ascii=False))
sys.exit(0 if passed else 1)
