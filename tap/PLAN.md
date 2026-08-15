# Catena Runtime Capture Plan

Updated: 2026-08-14

## Current Status

- [x] Freeze the replacement architecture and supported Runtime boundary.
- [x] Pin both Langfuse upstream commits and confirm their MIT licenses.
- [x] Vendor the selected Parser/turn-assembly sources with modification notes.
- [x] Implement Canonical Event Graph v1 and Catena OTLP adapters.
- [x] Replace live hooks and historical import with shared parser entry points.
- [x] Remove the `claude-tap` dependency, proxy wrapper, TraceStore monkey
      patch and heuristic Python `TraceNormalizer`.

## Milestones

### Parser replacement

- [x] Codex rollout: turns, model attempts, exact tool IDs, Web/MCP/local
      tools, retry/abort/incomplete, compaction and subagent threads.
- [x] Claude transcript: repeated prompts, message-id model attempts, exact
      `tool_use_id` pairing, async/parallel results, compaction, resume and
      nested subagents.
- [x] Stable Runtime-derived Trace/Span IDs and source-event accounting.
- [x] Atomic state/locking, incremental fail-open upload and duplicate-hook
      recovery.
- [x] Explicitly reject unsupported Runtime values.
- [x] Package the Codex hook through the official plugin marketplace contract,
      resolve code from `PLUGIN_ROOT`, and keep its Agent key in private
      `PLUGIN_DATA` with environment variables taking precedence.

### Fixture and golden acceptance

- [x] Commit real, structurally preserved and content-desensitized Codex and
      Claude fixtures plus canonical/OTLP goldens.
- [x] Cover no-tool, one-tool, serial tools with full history, parallel
      out-of-order results, Web Search, tool failure, HTTP/model retry, abort,
      subagent, context compact, identical consecutive user input and resume.
- [x] Prove live and historical import are byte-for-byte canonical-equivalent.
- [x] Prove a repeated hook creates no additional logical Span.

## Owners

- Catena Runtime capture: local Runtime parsing, Canonical Event Graph and
  canonical OTLP export.
- Catena Go control plane: Agent authentication, ingestion and storage.
- Runtime: model execution, tool execution and user-visible behavior.

## Acceptance Criteria

1. Every fixture Runtime event/block has one auditable Canonical disposition.
2. Session → Turn → Model → Tool/Subagent parentage uses Runtime identities.
3. Unknown non-empty result IDs never bind; failures and aborts are errors.
4. Live hook/import equality and duplicate-hook idempotency are automated.
5. No package or runtime path imports `claude-tap` or Langfuse SDKs.
6. Codex and Claude each pass real Runtime → Hook → OTLP → Go → ClickHouse →
   Web acceptance.

## Verification Log

- 2026-08-14: installed and trusted `tracing@catena-runtime` in Codex CLI
  `0.147.0`, configured the existing `Codex Local` Agent through an owner-only
  plugin-data credential, and ran a real no-tool Turn. Stop Hook → parser →
  OTLP → Go → ClickHouse retained Session
  `01a00007-bb6d-74e3-a128-0995b29f5492` as Trace
  `9a1fe9d219fdea97827a10fda487d2e2` with one successful Turn root and one
  successful Model Call. Replaying the identical Hook left one Trace and two
  spans. TypeScript typecheck, 9 tests and bundle build passed.

- 2026-08-14: pinned Codex upstream
  `7500867afecf963d1cf83bf2b860a659591ace18` and Claude upstream
  `5b3d4323c49f3839545fad36883ed02420ebc0ba`; both contain MIT license grants.
- 2026-08-14: Codex fixture accounts 131/131 rows into 14 traces and 54
  spans; Claude accounts 52/52 rows into 14 traces and 52 spans. Schema,
  golden, exact-ID, live/import and replay tests pass.
- 2026-08-14: authenticated Codex CLI `0.147.0` and Claude Code `2.1.112`
  each completed Runtime → Hook → Parser → OTLP → Go → ClickHouse → Web.
  Each accepted trace has four spans and zero errors; direct historical import
  matched live Trace/Span identity and duplicate hooks made no write. See
  `ACCEPTANCE-2026-08-14.md`.

## Risks / Open Questions

- Runtime transcript schemas are internal and may change; fixtures pin the
  accepted versions and unknown records fail open without inventing links.
- Claude transcripts expose session/user-row correlation but no W3C Trace ID;
  Catena derives the transport Trace ID deterministically from those raw IDs.
- Codex CLI `0.147.0` likewise exposes session/turn correlation but no W3C
  Trace ID; a valid Runtime Trace ID will take precedence when one is present.
- Real E2E requires locally authenticated Codex/Claude CLIs and a disposable
  Catena Agent key; it must never delete unrelated retained evidence.
- Codex hashes each non-managed Hook definition. An updated Hook remains
  installed but will not run until the user reviews and trusts its new hash.

## Status Maintenance Rules

Mark an item complete only with an automated test or a recorded real command.
When either pinned commit changes, update source/modification notices and rerun
all fixture, golden, live/import and real E2E acceptance before publishing.
