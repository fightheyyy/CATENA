"""Start the opt-in local worker without displaying or logging credentials."""
from __future__ import annotations

import os
import secrets
import subprocess
import sys
from pathlib import Path


def main() -> None:
    repo = Path(__file__).resolve().parents[2]
    root = repo / ".local/harbor-study"
    root.mkdir(parents=True, exist_ok=True)
    token_path = root / "worker-token.txt"
    token = token_path.read_text().strip() if token_path.exists() else secrets.token_hex(32)
    (root / "worker-token.txt").write_text(token, encoding="utf-8")
    env = os.environ.copy()
    env.update(PYTHONUTF8="1", PYTHONIOENCODING="utf-8", CATENA_HARBOR_WORKER_TOKEN=token, CATENA_EVAL_BASE_URL="http://127.0.0.1:8317/v1",
               CATENA_EVAL_KEY_FILE=str(Path(os.environ["LOCALAPPDATA"]) / "DuMemEval/cliproxyapi-7.3.2/client-key.txt"))
    env.setdefault("CATENA_E2B_KEY_FILE", str(root / "e2b-api-key.txt"))
    env.setdefault("CATENA_EVAL_HOST_ALIAS", "127.0.0.1")
    with (root / "worker.stdout.log").open("w") as stdout, (root / "worker.stderr.log").open("w") as stderr:
        process = subprocess.Popen(
            [sys.executable, "-m", "catena_engine.harbor_worker", "--suite", str(repo / "evals/rg-missing-search"),
             "--output", str(root / "runs"), "--image",
             "mcr.microsoft.com/powershell@sha256:62300a213a9293916333df2b014cd3a8f22fb0b0b65f2bb446aaf436bcf8c868"],
            cwd=repo / "engine", env=env, stdout=stdout, stderr=stderr,
            creationflags=subprocess.CREATE_NO_WINDOW)
    (root / "worker-pid.txt").write_text(str(process.pid))
    (root / "compose.override.yml").write_text(
        "services:\n  catena-engine:\n    environment:\n"
        "      CATENA_HARBOR_WORKER_URL: http://host.docker.internal:8792\n"
        f"      CATENA_HARBOR_WORKER_TOKEN: {token}\n", encoding="utf-8")
    print(f"Local Harbor worker started: PID {process.pid}")


if __name__ == "__main__":
    main()
