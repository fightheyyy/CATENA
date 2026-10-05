Delayed execution lifecycle 4

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Run the local experiment.py job exactly once, wait for full completion, and confirm result.json contains the final result. Do not replace or edit the program. A progress file may exist before completion.
