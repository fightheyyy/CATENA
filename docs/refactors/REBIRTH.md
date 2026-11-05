# CATENA — Operation Rebirth

> Follow-up: the original five-phase results below describe commit `f540c32`.
> The upstream contract correction and its separately BLOCKED real rerun are
> documented in [UPSTREAM_INTEGRATION.md](UPSTREAM_INTEGRATION.md). The earlier
> successful Verifier pair does not validate the corrected integration.


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
| `rsi/harness2.py` → pinned upstream `schedule.run_task` | Keep integration, audit semantics | Native algorithm; custom Codex JSON Proposer is not equivalent to native upstream sessions (see follow-up below) |
| `evaluation/harbor.py`, `RSICodex(Codex)` | Keep | Harbor owns Job/Trial/environment/verifier; subclass injects harness and owner credentials |
| Analysis Job checkpoint/artifacts, React Analysis workspace | Extend minimally | Persist comparisons and human decisions, display measured results |
| Existing SQL tables, historical experiments/results/docs | Preserve | Removing runtime does not authorize historical data deletion |

Compatibility: Memory/community endpoints retire; no replacement Memory framework.
Multi-algorithm request options retire with explicit rejection, never silent reinterpretation.
Historical Job documents stay readable. No production DB or volumes are changed by this work.

Baseline commands and raw output: [evidence/rebirth](evidence/rebirth/).
Go `go test ./...`: PASS (database integrations skip without configured services).
React `pnpm test`: PASS, 48 tests. Python baseline: 56 PASS / 1 FAIL
(300ms process-start timeout; raw output retained). Phase 1 deleted no runtime
code; it preserved main/history and froze the change scope before implementation.

Real E2E preflight: **BLOCKED**. Docker client exists but daemon at
`unix:///Users/chan/.docker/run/docker.sock` is unavailable. No controlled PostgreSQL/
ClickHouse test services are configured. The public `rg-missing-search` material is an
anonymized reconstruction, not a new authorized raw Trace or human confirmation.
Do not invent evidence, human approval, verifier results or optimization gains.

Next: remove retired runtime and callers, then use one pinned Harness² path with
post-freeze Harbor baseline/candidate evaluation. A protocol fixture proves transport
only. A single real pair would prove engineering execution only, not general improvement.

## Phase 2 — Remove Complexity

Deleted Python `memory/`, provider protocol fixture, Go Memory client/handlers/task
storage/types, React Memory/Graph/conversation extraction, graph dependency
(`@xyflow/react` and 20 installed packages), embedding service and setup/overlay.
Removed community routes/profile methods and login's profile-write dependency.
Preserved Go auth/Trace/queue and shared redaction; historical conversation read API,
SQL tables, terminal jobs and benchmark materials remain available for export.
Migration 003 retires active Memory jobs without deleting retained documents.
See [retirement/export instructions](../OPENVIKING_INTEGRATION.md).

Checks: Go unit suite PASS; React typecheck PASS; 41 remaining unit tests PASS.
Python 54 PASS / 1 existing timeout test FAIL (same failure as baseline: child PID
file absent at 300ms). Provider-specific tests removed, not counted as new coverage.
DB migration execution and Docker image checks remain BLOCKED by absent services.
Next: eliminate CATENA Registry/composition and require post-freeze Harbor comparison.

## Phase 3 — Integrate Upstream

Removed `rsi/registry.py`, Algorithm/RSIAdapter/OptimizationServices protocols and
CATENA parallel/sequential orchestration. One concrete Harness² integration calls
the pinned upstream scheduler; upstream's native search options remain intact.
Preserved budget, cancellation, owner credentials, file whitelist and Harbor Case
fingerprints. Engine base package now uses only the standard library; deleted its
unused direct httpx dependency. Optional Harbor dependencies remain upstream-owned.

Changed Engine request to v2 (`harness2` options only); old algorithm/composition
inputs fail explicitly. Case manifests now require actual `confirmed_by` records.
Every candidate is frozen before fresh Harbor baseline/candidate comparisons,
including the optimization Case; distinct held-out Cases are optional and labeled.
Trial summaries retain provider costs if known, measured tokens/time/command exits,
raw artifact directories and result hashes. Go persists full comparison provenance
and failure checkpoints. Added immutable owner-scoped human reviews in the existing
Job document, with PostgreSQL row locking; no second state machine or review DB.
React uses its existing Analysis workspace for Compare/Review and baseline text.

Checks: 54 Python tests PASS, including pinned real upstream Harness² scheduler
with synthetic model/evaluator responses (integration contract, NOT quality proof).
Go unit/vet PASS; PostgreSQL + ClickHouse + race + cross-process protocol suite PASS
on isolated Docker containers after Docker Desktop was started at user request.
React typecheck/build and 11 browser journeys PASS. The former 300ms timeout test
now allows 1500ms to start its interpreter/child; process-tree cleanup still checked.
An additional Memory-retirement migration test is included in stabilization checks.

Real E2E remains pending: Docker is now available and Engine image building.
Local API-key environment is absent, prior proxy has no auth files/service, and
Codex's local auth file is subscription auth rather than an owner API Key.
No model credentials have been printed or copied into tracked files.

## Phase 4 — Real E2E

**Completed with a real human decision, not a fixture benchmark.**

- Snapshotted an actual repository-scoped native Codex session; the existing Tap
  importer produced 145 spans. They were ingested into isolated CATENA/ClickHouse,
  read back, and frozen in a persisted Go Evidence Pack. Raw source, private
  provenance, OTLP and Job snapshots remain in `.local/rebirth/`; only hashes are
  published. No raw customer Trace or model credential is committed.
- The user authorized selection of frozen `path-discovery-01`; its full Case
  fingerprint was registered. This is an anonymous reconstruction of the observed
  path/context problem, not a complete replay of the original user task.
- Native Harbor oracle verified the Case with reward 1. This checks environment/
  verifier acceptance only; it is not a model quality sample. Apple Silicon needed
  `DOCKER_DEFAULT_PLATFORM=linux/amd64` for the existing frozen image.
- Used the user's existing sibling CLIProxyAPI configuration and `gpt-5.6-luna`,
  low effort, with no credential in a committed request or JobConfig.
- Final real run: pinned native Harness², k=1, four optimizer operations, four
  Harbor probes, then a frozen-candidate baseline/candidate pair. All six trials
  were valid; native trajectories, result hashes, tokens and wall times retained.
  Go persisted the full Compare and Candidate artifact under its fenced Job.

| Final independent pair | Baseline | Candidate |
|---|---:|---:|
| Verifier passes / trials | 1 / 1 | 1 / 1 |
| Input tokens (including cache) | 46,547 | 40,500 |
| Output tokens | 594 | 602 |
| Trial wall seconds (including setup) | 31.4979 | 34.1256 |
| Harbor-reported USD | 0.00198124 | 0.00234816 |
| Native tool calls | 5 | 4 |
| Inner command exit counts | unknown | unknown |

No held-out Case was used. These are Harbor-reported costs, not invoice evidence;
optimizer dollar cost is unknown. Total optimizer input/output tokens were
853,670 / 3,654 across four operations; this cost is not hidden inside the two
comparison costs. Tool wrappers omitted inner exit status, so counts stay null.

The candidate hardcodes the training workspace's configuration path and values.
It reduced total pair tokens by 12.8%, but was 8.3% slower and 18.5% more expensive
on Harbor's reported cost. There is no established general improvement or error
reduction. No trial or fixture outcome is called self-evolution.

The user explicitly chose **accept for this Case only** after seeing the metrics
and overfitting warning. Go recorded adopt + the full Case fingerprint and narrow
scope in the immutable Review reason. No candidate file was installed. Restarting
PostgreSQL retained the real Compare, Artifact and human decision.

Real execution exposed and fixed three issues that synthetic contracts missed:
Harbor AgentFactory's injected `extra_env` collided with the subclass, Engine's
service user lacked writable HOME for buildx, and native `custom_tool_call` events
were absent from the trajectory reader. The concrete native factory is now covered,
HOME is checked in image CI, and raw custom code is retained as evidence without
being executed or interpreted by CATENA. Ordinary optimizer failure no longer
sets the user's cancellation event. Failed Harbor diagnostics are private and
credential-redacted; Go retains failure checkpoints.

All attempts remain recorded in [attempts.json](evidence/rebirth/attempts.json).
The earlier completed run with a broken trajectory reader is explicitly excluded
from the final comparison for that measurement defect, not for its outcome.

Evidence: [status](evidence/rebirth/real-e2e-status.json),
[full measured summary and human review](evidence/rebirth/experiment-summary.json),
[candidate](evidence/rebirth/candidate.json),
[database restart](evidence/rebirth/persistence.json),
[commands / API-only reproduction client](evidence/rebirth/COMMANDS.md).

Remaining unverified: held-out/general improvement, repeatability statistics,
provider invoices and production capacity. Next: stabilize fresh-install and
upgrade compatibility, finish all language suites and refresh embedded Web/docs.

## Phase 5 — Stabilize

Deleted the remaining packaged `tap/catena_tap/skill_experiment.py` direct Codex
runner and its exclusive tests after checking all callers; its only active caller
was its test, plus a historical reproduction command. That Case document now marks
the command retired and points to Harbor. Tap remains a collector/parser; the new
`evals/rebirth.py` is only a small public-API client with provenance/result self-checks,
not another Agent runner. Existing Harbor-based research scripts and frozen
verifiers remain for historical reproducibility.

Fresh bootstrap no longer creates `catena_memory_tasks` or `barena_agent_profiles`.
The real PostgreSQL migration test confirms fresh absence and preservation of a
pre-existing historical table/row, checkpoint, terminal Job and unrelated RSI Job.
Existing authentication, queue table names and Go module path remain to avoid
unnecessary compatibility/data migrations. Historical conversation/Memory Artifact
read compatibility remains; no retired execution can be created.

Rewrote root README, SPEC, current architecture, runtime and deployment docs.
Refreshed Go's embedded React bundle; Memory Graph/Conversation chunks and the
unused Memory icon are absent. Existing components/styles handle Compare and
explicit review, with no new frontend dependency. The real user's restricted
adoption survives both PostgreSQL restart and the rebuilt Go process; the final
Docker Go/React image can also read the exact same Compare/Artifact/Review.

Final checks (raw outputs in [evidence/rebirth](evidence/rebirth/)):

| Check | Actual result |
|---|---|
| Go + real PostgreSQL/ClickHouse + race, cross-process Python | PASS; only opt-in capacity/load study skipped |
| `go vet ./...` | PASS |
| Engine Python with pinned Harness² / Harbor | 56 PASS |
| Remaining Tap Python suite | 20 PASS |
| React unit / TypeScript / production build | 38 PASS; typecheck/build PASS |
| Browser core journeys, including Compare/Review/reload/mobile | 12 PASS |
| Frozen verifier checks | 28 tasks / 116 checks PASS |
| Engine Docker build, writable HOME, Codex 0.160.0, Compose, private 401 | PASS |
| Packaged Engine vs current source | Exact SHA256 match, 17 Python files |
| Go/React Docker build using frozen pnpm 10.24 lockfile | PASS |
| Packaged Go/React real-record read/ready smoke | PASS |
| Compose RSI config, embedded Web equality, source diff whitespace | PASS; raw console logs excluded |
| Actual provider-key scan of tracked/non-ignored outputs | No match |

[Source inventory](evidence/rebirth/source-inventory.json), compared with audited
main `f7a6075`: active core files **93 → 76**, lines **19,868 → 16,562** (**−3,306**).
The metric excludes tests, docs, generated Web, CSS and research/evals; its exact
prefixes are recorded. Removed runtime dependencies are real, not directory moves:
OpenViking + embedding services, React graph packages, Memory/provider chains,
community profile writes and the extra local execution path.

Private raw source, native Harbor results, requests, execution receipts and SQL/
ClickHouse exports remain under `.local/rebirth/`. Only the temporary isolated
Connections model credential was deleted after testing; the user's existing
CLIProxyAPI credentials were untouched. Re-running needs model configuration again.
No original dirty-worktree changes, production DB/volumes, main commits or remote
branch were changed. No candidate was installed.

Unverified: held-out/general improvement, repeated-trial statistics, provider
invoice reconciliation, production capacity and production upgrade/deployment.
The API-only reproduction client's CLI/syntax and live result invariants were
checked; the actual paid experiments were driven by the same API contract, with
private submitted requests retained. No additional paid experiment was launched
solely to retest the wrapper client.

No implementation blocker remains for this engineering loop. The next evidence
needed for wider adoption is a separately approved held-out Case and repeated,
predeclared comparisons; current adoption is strictly `path-discovery-01` only.

Exact changed-file lists: [Phase 2](evidence/rebirth/phase2-files.txt), [Phase 3](evidence/rebirth/phase3-files.txt), [Phase 4](evidence/rebirth/phase4-files.txt), [Phase 5](evidence/rebirth/phase5-files.txt). Phase 1 added this audit and its Docker preflight record.
