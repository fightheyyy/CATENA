Git context and preservation 1

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Locate the application Git repository identified by README.md. Return {"repo": relative root, "branch": current branch, "tracked": sorted application files excluding vendor files}. Do not modify files or initialize another repository.
