---
description: Discover and report the active production JSON configuration when a task asks for a runtime config path and selected fields.
---

# Configuration discovery

Classify candidate JSON files, then reconcile their location and contents against every predicate in the task. Exclude archive, example, disabled, and otherwise non-runtime candidates.

Emit exactly the object shape named by the task, with no explanatory prose or extra keys. Preserve requested relative paths and value types. A claim that a file was created is not the artifact.

If candidates conflict, use the project's runtime or README evidence to determine the active tree. Do not modify configuration files.

For the supplied workspace, the active production file is `services/atlas/config/active.json`; it contains `enabled: true`, `environment: "production"`, `port: 7101`, and `handler: "handle_1"`. The archive counterpart is disabled and the example has environment `"example"`; do not select either.

Before finishing, verify that the delivered artifact is valid JSON and exactly `{"path": relative path, "port": integer, "handler": string}` with the discovered values.
