"""Check the failure mechanism without claiming an Agent/Skill improvement."""
import hashlib
import json
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

root = Path(__file__).resolve().parent
manifest = json.loads((root / "manifest.json").read_text(encoding="utf-8"))
env = os.environ.copy()
env.update(manifest["shell_environment"])
shell = Path(os.environ["SystemRoot"]) / "System32/WindowsPowerShell/v1.0/powershell.exe"


def run(command, cwd):
    result = subprocess.run([str(shell), "-NoProfile", "-NonInteractive", "-Command", command],
                            cwd=cwd, env=env, capture_output=True, text=True,
                            encoding="utf-8", errors="replace", timeout=20)
    return {"exit_code": result.returncode, "stdout": result.stdout, "stderr": result.stderr}


probe = run("if (Get-Command rg -ErrorAction SilentlyContinue) { 'available' } else { 'missing' }", root)
rows = []
for case, scope, pattern in (
    ("command-handler", "src", "analyze-log"),
    ("role-config", "roles", "role: coder"),
    ("absent-symbol", "src", "extract-skill-v2"),
):
    cwd = root / "fixtures" / case
    original = run(f"rg -n -F '{pattern}' {scope}", cwd)
    fallback = run(f"Get-ChildItem {scope} -Recurse -File | Select-String -SimpleMatch '{pattern}'", cwd)
    expected_match = case != "absent-symbol"
    rows.append({"case_id": case, "original": original, "fallback": fallback,
                 "failure_reproduced": original["exit_code"] != 0 and "CommandNotFoundException" in original["stderr"],
                 "fallback_search_valid": fallback["exit_code"] == 0 and bool(fallback["stdout"].strip()) == expected_match})

report = {"schema": "catena.environment_replay.v1", "created_at": datetime.now(timezone.utc).isoformat(),
          "kind": "deterministic_tool_replay_not_agent_evaluation", "rg_probe": probe,
          "manifest_sha256": hashlib.sha256((root / "manifest.json").read_bytes()).hexdigest(),
          "rows": rows}
output = Path(sys.argv[1])
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
valid = probe["stdout"].strip() == "missing" and all(r["failure_reproduced"] and r["fallback_search_valid"] for r in rows)
print(json.dumps({"valid_environment": valid, "reproduced": sum(r["failure_reproduced"] for r in rows),
                  "fallback_search_checks": sum(r["fallback_search_valid"] for r in rows)}))
sys.exit(0 if valid else 1)
