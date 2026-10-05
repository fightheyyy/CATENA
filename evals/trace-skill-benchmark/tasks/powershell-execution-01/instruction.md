PowerShell processing variant 1

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Using PowerShell 7 (pwsh), compute the number and total cents of paid orders in "input data/orders.json". Python inside PowerShell is allowed. Result schema: {"paid_count": integer, "paid_cents": integer}.
