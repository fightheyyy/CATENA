# Codex parser provenance

- Repository: <https://github.com/langfuse/codex-observability-plugin>
- Commit: `7500867afecf963d1cf83bf2b860a659591ace18`
- Retrieved: 2026-08-14
- License: MIT (`LICENSE` in this directory)

Derived files:

- `plugins/tracing/src/parse.ts` → `codex/src/langfuse-derived/parse.ts`
- `plugins/tracing/src/types.ts` → `codex/src/langfuse-derived/types.ts`
- `plugins/tracing/src/utils.ts` → `codex/src/langfuse-derived/utils.ts`
- `plugins/tracing/src/sidecar.ts` informed `codex/src/state.ts`

Catena modifications:

- retain Runtime session/turn correlation, optional `task_started.trace_id`,
  response item IDs and source-event indexes; Codex CLI `0.147.0` has no
  `trace_id`, so Catena deterministically encodes `session_id:turn_id` for
  OTLP transport without inventing a turn boundary;
- expose compaction, retry and unmatched-result evidence;
- retain upstream built-in tool items and MCP lifecycle enrichment, including
  exact call IDs, server/tool identity, actions, results and errors;
- reject any order-based fallback for a non-empty unknown `call_id`;
- replace all Langfuse observation/SDK code with Canonical Event Graph and
  Catena OTLP rendering;
- replace the `.langfuse` sidecar with an atomic Catena upload ledger written
  only after successful OTLP upload;
- add a bounded post-Stop settle pass because Codex persists `task_complete`
  only after the synchronous hook returns.
