Discover active configuration 1

Work only inside /workspace/case. Preserve unrelated files and user edits. Do not access /tests or verifier code. Write the requested JSON to /workspace/case/result.json. Do not fetch packages or contact external services; the model connection is handled by the harness.

Find the enabled production runtime JSON configuration in this project. Ignore archive and example files. Return {"path": relative path, "port": integer, "handler": string}. Discover the actual path; do not modify configuration.
