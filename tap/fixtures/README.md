# Runtime fixtures

These fixtures are format-preserving, redacted extracts of real local runtime
history captured on 2026-08-14:

- Codex CLI `0.147.0` rollout JSONL
- Claude Code `2.1.112` transcript JSONL

The source histories remain in their Runtime-owned directories and were not
modified or deleted. Sanitization replaced prompt/result text, absolute paths,
repository names, session/turn/message/call IDs, and timestamps while retaining
the real row schemas, lifecycle ordering, split-message behavior, retry/error
markers, tool correlation fields, and subagent directory layout. No secret,
credential, proprietary source text, Barena trace, or XiaoBaOS trace is present.
In particular, the Codex fixture intentionally has no synthetic
`task_started.trace_id`: CLI `0.147.0` does not emit that field.

Together the two fixtures cover:

- ordinary no-tool and single-tool turns;
- two serial tools followed by a model request carrying complete prior tool
  history;
- parallel tool calls with out-of-order results;
- Function, Custom, Local Shell, Web Search and File Search tools, plus tool
  failure and HTTP/model retry;
- exact MCP server/tool/call correlation from a real Codex MCP lifecycle;
- turn abort, subagent parentage and context compaction;
- consecutive identical user input, session resume and an unknown non-empty
  result ID;
- live/historical convergence and duplicate-hook replay.

`golden/*.canonical.json` is generated from these checked-in JSONL inputs. The
test suite validates each golden against
`contracts/canonical-event-graph.schema.json`, checks one accounting entry per
input row, and proves the finalized live-hook graph/OTLP identity is exactly the
historical-import result. Provisional Stop snapshots remain explicitly
incomplete until the Runtime persists its completion evidence.

The checked-in histories did not contain a real Computer tool invocation.
Codex's upstream `computer_call` shape therefore has a separate parser
conformance test for exact `call_id`, action and result retention; it is not
reported as a real Computer-tool acceptance run.
