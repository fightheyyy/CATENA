Scoped existing-file edit 4

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Remove legacy.py and replace app.py with a dependency-free render() function that returns "current". Preserve notes/user.txt. Keep app.py exactly as: def render(): followed by four-space-indented return "current", with a trailing newline. Return {"updated": true}.
