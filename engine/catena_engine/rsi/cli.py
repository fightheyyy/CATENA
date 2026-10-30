"""Explicit local backend invocation; output may contain private evidence."""

import argparse
import json
import os
from pathlib import Path

from .api import run_payload
from .contracts import OptimizationError
from .registry import default_registry


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--list", action="store_true", help="list registered algorithms without model calls")
    parser.add_argument("--request", type=Path, help="private JSON request; prefer RSI_MODEL_KEY over a key in the file")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if args.list:
        print(json.dumps(default_registry().describe(), indent=2))
        return
    if not args.request or not args.output:
        parser.error("--request and --output are required")
    payload = json.loads(args.request.read_text())
    if os.environ.get("RSI_MODEL_KEY"):
        payload.setdefault("model", {})["api_key"] = os.environ["RSI_MODEL_KEY"]
    try:
        result = run_payload(payload)
    except OptimizationError as error:
        parser.exit(1, str(error) + "\n")
    except Exception:
        parser.exit(1, "RSI operation failed; check the private worker environment\n")
    # Exclusive creation avoids following symlinks or truncating another report.
    with os.fdopen(os.open(args.output, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as output:
        json.dump(result, output, ensure_ascii=False, indent=2)
    if result["status"] != "completed":
        parser.exit(1, "RSI failed; the private report includes completed-operation usage\n")
    print("RSI report saved; candidates require independent review.")


if __name__ == "__main__":
    main()
