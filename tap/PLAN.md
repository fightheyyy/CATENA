# Catena Tap Plan

Updated: 2026-08-11

## Current Status

- [x] Select one complete open-source capture engine: `claude-tap`.
- [x] Define the local capture, canonicalization and OTLP boundary.
- [x] Implement the Python package and `catena tap` command.
- [x] Verify canonical Codex and Claude tool traces.
- [x] Run a real local Codex smoke against a mock Catena receiver.

## Milestones

### MVP — one capture path

- [x] Pin the upstream capture dependency.
- [x] Add a fail-open asynchronous uploader.
- [x] Normalize Anthropic Messages, OpenAI Responses and Chat Completions.
- [x] Pair tool outputs by call ID and close a turn on a final assistant response.
- [x] Expose Codex, Codex App, Claude Code, Hermes and OpenClaw through one CLI.
- [x] Document install and connection commands.

## Owners

- Catena Tap: local Runtime capture and canonical OTLP export.
- Catena Go control plane: Agent authentication, ingestion and storage.
- Runtime: model execution, tool execution and user-visible behavior.

## Acceptance Criteria

1. `catena tap codex -- --help` delegates to Codex through claude-tap.
2. A tool-using turn produces one root, at least one model span and a tool span
   containing both input and result.
3. Anthropic and OpenAI tool-call shapes have automated coverage.
4. Upload failure does not change the wrapped Runtime exit code.
5. The package has no dependency on the Catena Web application or database.

## Verification Log

- Ten unit/integration tests pass with the actual pinned dependency installed.
- `catena tap codex -- --help` delegated through claude-tap and retained the
  Runtime exit code.
- A real Codex 0.147.0 run called `pwd`; the mock Catena receiver accepted one
  Turn containing Model → `exec` Tool → Model and the Tool Span retained its result.
- Replaying the captured session ignores `/v1/models` discovery traffic and
  deterministically produces one four-Span Turn.

## Risks / Open Questions

- The wrapper intentionally follows a pinned claude-tap release because its
  Python modules are not yet declared as a stable plugin API.
- Proxy capture observes the model-visible tool exchange. Exact internal tool
  execution timing remains Runtime-native telemetry.

## Status Maintenance Rules

Mark an item complete only with an automated test or a recorded real command.
When the pinned upstream version changes, rerun normalization and delegation
tests before publishing.
