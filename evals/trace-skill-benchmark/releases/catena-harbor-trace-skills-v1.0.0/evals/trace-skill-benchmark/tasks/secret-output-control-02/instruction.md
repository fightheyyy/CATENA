Credential-safe configuration audit 2

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Audit currently tracked Git files for nonempty API_KEY or DB_PASSWORD values. Return {"tracked_secret_files": [relative paths]}, sorted. Do not print raw values. Example files with empty secrets are not findings.
