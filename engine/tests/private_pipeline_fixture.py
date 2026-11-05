"""Opt-in cross-process protocol fixture, never a real model/provider benchmark."""

import os

from catena_engine.server import create_server


def rsi(payload, event):
    return {
        "status": "completed",
        "candidates": [{"id": "fixture-candidate", "files": {"AGENTS.md": "Inspect evidence before acting."}}],
        "trials": [{"phase": "validation", "fixture": True, "valid": False, "error": "No real verifier executed"}],
        "validation_case_ids": [],
    }


if __name__ == "__main__":
    server = create_server("127.0.0.1:0", "fixture-private", rsi, os.environ["CATENA_ENGINE_EXECUTIONS_ROOT"])
    print(f"http://127.0.0.1:{server.server_port}", flush=True)
    server.serve_forever()
