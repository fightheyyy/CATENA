# Runtime capture acceptance — 2026-08-14

This record contains no API keys or unredacted transcript content. Existing
Barena, XiaoBaOS and unrelated ClickHouse rows were left untouched.

## Fixture and golden audit

| Runtime | Real redacted source rows | Canonical traces | Canonical/OTLP spans | Final trace states |
| --- | ---: | ---: | ---: | --- |
| Codex CLI 0.147.0 | 131 | 14 | 54 | 11 ok, 2 error, 1 aborted |
| Claude Code 2.1.112 | 52 | 14 | 52 | 11 ok, 2 error, 1 aborted |

Every source row has one Canonical accounting disposition. Automated tests
cover exact unknown-ID non-binding, failure/abort status, serial and parallel
tools, retry, Web Search, a real Codex MCP lifecycle, compaction, subagent
parentage, repeated input, resume, live/import convergence and duplicate-hook
replay. The retained histories contained no real Computer invocation; its
upstream item shape is parser-conformance tested but is not claimed as a real
tool acceptance run.

## Real Codex E2E

- Agent: `agent-1786697627944-c941658697bac8d8`
- Runtime session: `019fff80-2d06-7ca1-ac4b-2123fd61fab2`
- Runtime turn: `019fff80-2d61-7da0-8a5e-606e1a3d3d96`
- Canonical/OTLP trace: `58c2adf92ea64151e1c16d99f54532d4`
- Result: 4 spans, 0 errors; Turn → first Model → exact `exec` Tool, plus
  final Model. The Web view showed the command argument, terminal result,
  user input and final answer.
- A second invocation of the exact Stop-hook payload left both Span count and
  `last_ingested_at` unchanged. The local ledger records the trace as complete.

The first diagnostic run proved Codex persists `task_complete` after its
synchronous Stop hook. The accepted implementation uploads the provisional
incomplete snapshot, then a bounded detached settle pass invokes the same
parser and replaces those stable IDs with the completed graph.

## Real Claude Code E2E

- Agent: `agent-1786697627946-49f4f4f0400d1959`
- Runtime session: `10d0ecdc-7648-476f-86b6-7aafcf940f83`
- Runtime turn/user row: `afd0e498-2989-4c20-851b-4fb459b5dd4c`
- Canonical/OTLP trace: `04e5b2b2c551b347a84d54a8d69e07d1`
- Result: 4 spans, 0 errors; Turn → first Model → exact `Bash` Tool, plus
  final Model. The Web view showed the command argument, result, user input
  and final answer.
- A repeated SessionEnd hook left both Span count and `last_ingested_at`
  unchanged.

## Storage and presentation reconciliation

Direct `catena.catena_spans FINAL` queries returned exactly four rows and zero
errors for each accepted trace. Re-running each real source through historical
import produced the same Trace IDs, Span IDs, parent Span IDs, names and status
codes as the live records. The Go Trace API and Web Trace View both rendered
each as one successful Turn, two Model calls and one Tool with zero errors.
The receiver recognizes native coding-agent evidence only from Catena's exact
service/runtime attribute pairs; product-name substrings and retired aliases
remain generic OTel.

## Fresh narrative-view baseline

Two additional real Runtime runs used benign marker commands at execution
time, so their retained Catena Traces are complete and were not redacted or
rewritten after capture:

| Runtime | Runtime session | Catena trace | Retained flow |
| --- | --- | --- | --- |
| Codex CLI `0.147.0` | `019fffe1-e019-7461-8db7-cf08b76a534b` | `b500a6a56a920efe1702f4b468e3917e` | Turn → Model → exact `exec` → Model |
| Claude Code `2.1.112` | `1efff7fb-04ce-402c-859b-4a57a8ca18cf` | `2e83c300eff4332a3ae94b12da2617f0` | Turn → Model → exact `Bash` → Model |

Each trace contains four spans, zero errors, the complete prompt and final
answer, full tool arguments/result, exact call ID, model Token usage and
Runtime timestamps. The new-parser historical importer uploaded both into the
local Catena stack. Browser acceptance verified the narrative and diagnostic
lenses at desktop, constrained 1200px and 390px widths with no horizontal
overflow and zero console errors or warnings.

## Verification commands

```text
tap/.venv/bin/ruff check tap/catena_tap tap/tests tap/scripts/real_codex_smoke.py
tap/.venv/bin/pytest -q
tap/.venv/bin/python -m compileall -q tap/catena_tap tap/tests tap/scripts
cd tap/codex && pnpm typecheck && pnpm test && pnpm build
cd control-plane && go test ./... && go vet ./... && go test -race ./...
cd catena-web && pnpm test && pnpm typecheck && pnpm build
claude plugin validate tap/claude-plugin
./deploy/catena-mvp1/demo.sh smoke
```
