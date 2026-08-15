# Claude Code parser provenance

- Repository: <https://github.com/langfuse/claude-observability-plugin>
- Commit: `5b3d4323c49f3839545fad36883ed02420ebc0ba`
- Retrieved: 2026-08-14
- License: MIT (`LICENSE` in this directory)

Derived source:

- `hooks/langfuse_hook.py` transcript reader, state model, turn assembly,
  strict `tool_use_id` routing and subagent discovery →
  `catena_tap/claude_parser.py`

Catena modifications:

- remove Langfuse imports, configuration, client, observation types and API
  authentication;
- rename state/log configuration to Catena and commit state only after a 2xx
  Catena OTLP response;
- produce Canonical Event Graph nodes with deterministic Runtime-derived IDs;
- preserve tool failure, model retry, abort/incomplete, context compaction and
  source-event accounting;
- use the same parser functions for Stop/SessionEnd hooks and historical
  import.
