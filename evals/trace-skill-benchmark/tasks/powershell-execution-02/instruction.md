PowerShell processing variant 2

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Using PowerShell 7, find lines whose entire text equals "$rate[0] = 'quoted' + \"double\"". Read logs/notes.txt. Return {"lines": [one-based line numbers]}, ascending. Treat the pattern as literal text.
