# CATENA architecture

Codex = analysis/improvement Agent. Harbor = evaluation engine.
CATENA = attribution, orchestration and engineering records.

```mermaid
flowchart TD
    Trace[Tap / OTLP] --> Ingress[Go: API Key / trusted Owner + Agent]
    Ingress --> Gateway[Collector Gateway]
    Gateway --> Kafka[(Kafka)]
    Kafka --> Consumer[Collector Consumer / official ClickHouse Exporter]
    Consumer --> CH[(ClickHouse otel_traces)]
    CH --> Go[Go: owner-scoped frozen Evidence]
    Go --> Queue[(PostgreSQL: Job / lease / heartbeat / fencing)]
    Queue --> Receipt[Private Engine execution receipt]
    Receipt --> Analyze[Codex + trace-analysis Skill / native investigation loop]
    Analyze --> Query[list_traces / get_trace]
    Query --> Gate[Go: live lease + fixed owner / Agent / time scope]
    Gate --> CH
    Gate -->|redacted pages / tool observations| Analyze
    Analyze --> Report[Problem / Evidence / hypotheses / uncertainty]
    Report --> Decision{Human decides whether to improve}
    Decision -->|keep report| Records
    Decision -->|explicit new Job + frozen analysis snapshot + confirmed Case| Workflow[improvement.py: Experiment.run]
    Workflow --> Base[Harbor baseline probe]
    Base --> Agent[Codex App Server + explicit improvement Skill]
    Agent --> Tool[evaluate_candidate: validated files / fixed Case / one probe]
    Tool --> Probe[Harbor native Job / Trial / Codex / Environment / Verifier]
    Probe --> Observation[Actual deliverables + ATIF; no rewards]
    Observation --> Agent
    Agent --> Freeze[Workflow freezes final Candidate]
    Freeze --> Pair[Harbor fresh baseline then candidate; optional held-out pairs]
    Pair --> Persist[Go: Compare / Artifact / version / provenance]
    Persist --> Review[React Human Review]
    Review --> Records[(PostgreSQL engineering records)]
```

| Source | Responsibility |
|---|---|
| `backend/internal/control/analysis_job.go` | Evidence, Job admission, private invocation, persisted result/artifacts |
| `backend/internal/control/trace_investigation.go` | Task-scoped read-only Trace tools, active-lease authorization, pagination and redaction |
| `analysis_worker.go`, `postgres_analysis_queue.go` | Durable scheduling, retry/cancel/timeout, lease and fencing |
| `review.go`, `artifact.go` | Independent Compare eligibility, immutable review, owner-scoped history |
| `engine/catena_engine/analysis.py`, `skills/trace-analysis/SKILL.md` | Analysis-only report, validated evidence references, no Harbor tools |
| `engine/catena_engine/improvement.py` | Fixed stages, trial reserve, candidate validation/freeze, score-free tool callback |
| `runtime/codex.py`, `skills/harness-improvement/SKILL.md` | Shared native Runtime, controlled task modes, strategy instructions, isolated process and audit |
| `evaluation/harbor.py`, `harbor_agent.py`, `evidence.py` | Native Harbor execution, thin target injection, TrialResult/ATIF/artifact reads |
| `executions.py`, `server.py` | Private authentication, owner receipts, operation deduplication |
| `frontend/src/AgentAnalysisLauncher.tsx`, `AnalysisWorkspace.tsx` | Configuration, state, Compare, candidate files and Human Review |

Harness² supplies method inspiration, not a runtime dependency. No Registry,
custom optimizer Host/Bench/Proposer, universal Plugin SDK or second durable queue
remains. Internal analysis Runtime is separate from the evaluated target; the
current target adapter is Codex, not a universal harness format.

Go+React, Python Engine and PostgreSQL retain their control-plane roles. The Trace
data plane adds Collector Contrib Gateway/Consumer and Kafka before ClickHouse.
Go no longer writes ClickHouse in production. Redis is not introduced. See
[delivery, schema, migration and acceptance](TRACE_DATA_PLANE.md). Codex analysis
is an Engine child process; Harbor creates disposable target containers.

Engineering memory reuses versioned Evidence/Candidate/Compare/Review. OpenViking
and automatic retrieval remain retired. Historical terminal Jobs are unchanged.
See [workflow contracts](IMPROVEMENT_WORKFLOW.md), [spec](../SPEC.md)
· [analysis acceptance](refactors/ANALYSIS_GATE.md) · [improvement acceptance](refactors/UNIFIED_RUNTIME.md) and [historical evidence](refactors/REBIRTH.md).
