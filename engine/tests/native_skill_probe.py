"""Native protocol check against an offline provider; never quality evidence."""

import http.server
import json
import tempfile
import threading
from pathlib import Path

from catena_engine.contracts import OptimizationError
from catena_engine.protocol import ModelConfig
from catena_engine.runtime.codex import CodexRuntime


def main():
    captured = []

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_POST(self):
            captured.append(json.loads(self.rfile.read(int(self.headers["Content-Length"]))))
            body = b'{"error":{"message":"offline protocol diagnostic","type":"server_error"}}'
            self.send_response(500)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    runtime = CodexRuntime(
        ModelConfig("openai", f"http://127.0.0.1:{server.server_port}/v1", "fixture", "diagnostic-key")
    )
    runtime.preflight()

    async def evaluate(value):
        raise AssertionError("Offline provider must not request Harbor execution")

    try:
        with tempfile.TemporaryDirectory() as directory:
            try:
                runtime.run(
                    "Offline Skill-loading diagnostic only.", 15, evaluate, Path(directory) / "agent", threading.Event()
                )
            except OptimizationError:
                pass  # The offline provider deliberately rejects inference after capturing input.
            else:
                raise AssertionError("Offline provider unexpectedly completed inference")
            assert captured
            body = json.dumps(captured[0])
            assert "Propose a small change to AGENTS.md" in body, "Native Codex did not inject Skill instructions"
            assert "evaluate_candidate" in body
            assert "not solve the evaluation task yourself" in body
            print(
                json.dumps(
                    {
                        "native_skill_body_injected": True,
                        "harbor_tool_declared": True,
                        "provider": "offline HTTP fixture",
                        "quality_evidence": False,
                    }
                )
            )
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    main()
