Scoped existing-file edit 2

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Set retries to 5 in service.json. Preserve all other values and user notes. Return {"updated": true}.
