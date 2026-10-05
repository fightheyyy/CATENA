Credential-safe configuration audit 4

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Compare required variable names in .env.example against nonempty values in runtime .env. Return sorted missing names as {"missing": [names]}. Do not print credential values.
