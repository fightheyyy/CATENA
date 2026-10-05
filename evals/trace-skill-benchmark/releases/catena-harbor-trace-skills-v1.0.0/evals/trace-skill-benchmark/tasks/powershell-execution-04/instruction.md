PowerShell processing variant 4

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Using PowerShell 7 pipeline operations, select ready rows with score >= 4 from events.csv. Sort descending by numeric score. Return {"names": [names], "sum": integer}.
