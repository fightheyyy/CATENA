# CATENA — Operation Rebirth

Audit baseline: `origin/main f7a6075`, 2026-11-06 (Asia/Shanghai).
Implementation: `codex/operation-rebirth`, isolated worktree; original dirty worktree untouched.

CATENA is a Trace-driven Agent Engineering Control Plane:
Agent Trace → Evidence → Case → Harness² → Candidate → Harbor → Compare → Human Review.

## Phase 1 — Audit & Freeze

Actual dependencies and change scope:

| Path / callers | Keep / delete / replace | Reason |
|---|---|---|
| `tap/`, Go OTLP → ClickHouse → `evidence.go` | Keep | Real collection, owner isolation, redaction and retained sources |
| Go `analysis_{queue,worker,runtime}.go`, PostgreSQL queue | Keep | Lease, heartbeat, fencing, idempotency, cancellation and retry are correctness mechanisms |
| Python `executions.py`, `runtime/codex.py` | Keep | Deduplicates external paid work, prevents uncertain replay, cleans up cancelled subprocesses |
| Python `memory/`, Go `memory*`, HTTPConfig/server wiring | Delete; retain shared Evidence redaction | OpenViking is outside the core loop |
| React Memory, Graph, task view, conversation extraction, API/types/tests | Delete | No retired feature navigation or background calls |
| OpenViking Compose, embedding image/server, setup/env/CI | Delete | Removes two deployment services and provider configuration |
| Community HTTP, profile creation on login, storage methods | Delete | No role in trace-driven engineering; preserve existing SQL data |
| `rsi/registry.py`, CATENA Algorithm/RSIAdapter protocol, host composition | Delete | Only Harness² exists; upstream owns its native search modes |
| `rsi/harness2.py` → pinned upstream `schedule.run_task` | Keep thin integration | Already calls native algorithm, no copied algorithm loop |
| `evaluation/harbor.py`, `RSICodex(Codex)` | Keep | Harbor owns Job/Trial/environment/verifier; subclass injects harness and owner credentials |
| Analysis Job checkpoint/artifacts, React Analysis workspace | Extend minimally | Persist comparisons and human decisions, display measured results |
| Existing SQL tables, historical experiments/results/docs | Preserve | Removing runtime does not authorize historical data deletion |

Compatibility: Memory/community endpoints retire; no replacement Memory framework.
Multi-algorithm request options retire with explicit rejection, never silent reinterpretation.
Historical Job documents stay readable. No production DB or volumes are changed by this work.

Baseline commands and raw output: [evidence/rebirth](evidence/rebirth/).
Go `go test ./...`: PASS (database integrations skip without configured services).
React `pnpm test`: PASS, 48 tests. Python baseline is recorded separately.

Real E2E preflight: **BLOCKED**. Docker client exists but daemon at
`unix:///Users/chan/.docker/run/docker.sock` is unavailable. No controlled PostgreSQL/
ClickHouse test services are configured. The public `rg-missing-search` material is an
anonymized reconstruction, not a new authorized raw Trace or human confirmation.
Do not invent evidence, human approval, verifier results or optimization gains.

Next: remove retired runtime and callers, then use one pinned Harness² path with
post-freeze Harbor baseline/candidate evaluation. A protocol fixture proves transport
only. A single real pair would prove engineering execution only, not general improvement.
