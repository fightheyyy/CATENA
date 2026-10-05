Delayed execution lifecycle 3

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Start this local service once, wait until /health responds, save the parsed response to result.json, and leave the service running. Inspect README.md for its startup command and endpoint.
