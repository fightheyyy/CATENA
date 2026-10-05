Scoped existing-file edit 1

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Change retry_limit() in managed/policy.py from 2 to 5. Preserve every other byte and the existing directory permissions. The file is writable but its parent directory does not allow deletion or new entries. Return {"updated": true}.
