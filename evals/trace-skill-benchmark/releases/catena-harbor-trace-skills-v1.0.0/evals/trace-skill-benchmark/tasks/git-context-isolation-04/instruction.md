Git context and preservation 4

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Create a separate Git worktree and branch inside /workspace/case, change the application port to 8300 there, and leave the original project/config.json unchanged. Do not commit. Return {"updated_in_isolation": true}.
