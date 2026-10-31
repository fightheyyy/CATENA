#!/usr/bin/env python3
"""Compare active core source with a Git baseline; tests/research/bundles excluded."""
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BASE = "8ac6708"
PREFIXES = ("control-plane/", "engine/", "catena-web/src/", "tap/catena_tap/")
EXTENSIONS = {".go", ".py", ".ts", ".tsx"}


def core(name):
    return (name.startswith(PREFIXES) and Path(name).suffix in EXTENSIONS
            and not name.endswith("_test.go") and "/tests/" not in name and "/web/" not in name)


def measure(items):
    return {"files": len(items), "lines": sum(len(content.splitlines()) for content in items.values())}


def main():
    names = subprocess.check_output(["git", "ls-tree", "-r", "--name-only", BASE], cwd=ROOT, text=True).splitlines()
    before = {name: subprocess.check_output(["git", "show", BASE + ":" + name], cwd=ROOT) for name in names if core(name)}
    after = {str(p.relative_to(ROOT)): p.read_bytes() for prefix in PREFIXES for p in (ROOT / prefix).rglob("*")
             if p.is_file() and core(str(p.relative_to(ROOT)))}
    result = {"baseline": BASE, "scope": {"prefixes": PREFIXES, "extensions": sorted(EXTENSIONS),
              "excluded": ["*_test.go", "tests", "Go embedded web", "CSS", "research/evals archives"]},
              "before": measure(before), "after": measure(after)}
    result["delta_lines"] = result["after"]["lines"] - result["before"]["lines"]
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
