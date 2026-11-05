# Harness² → Harbor integration

CATENA calls upstream `harness2.core.schedule.run_task` at commit
`1cfe254e8d25b15f37048c3730134686abe60335`. Harbor is pinned to `0.23.0`,
Codex CLI to `0.160.0`. No algorithm source is copied into CATENA. The integration
adapts upstream Bench/Proposer file edits to owner-configured Codex model calls
and Harbor probes. CATENA has no algorithm registry or composition layer.
Harness² native `k` and parallel/sequential refinement remain upstream options.

The Go PostgreSQL queue owns scheduling, leases, heartbeat, fencing, retry,
timeout and cancellation. Python receipts deduplicate external operations; an
uncertain receipt after restart is never automatically replayed. Budget counts
failed paid operations too; validation trials are reserved before optimization.
These controls are not a provider-enforced dollar/token cap.

## Register a Case

An operator must inspect the task, environment and independent verifier and
confirm its relationship to real retained Trace evidence. Raw traces alone do
not reconstruct a workspace. Create a private operator-owned `cases.json`:

```json
{
  "approved-case": {
    "path": "/absolute/harbor-task",
    "sha256": "<task_digest of the complete task directory>",
    "confirmed_by": "<actual human reviewer / review record>"
  }
}
```

Use `catena_engine.evaluation.harbor.task_digest(Path(...))`; all task files,
including verifier/environment, participate. Symlinks and changed fingerprints
are rejected. Do not fill `confirmed_by` with an invented approval. The registered
ID is exposed to users; the path and verifier are not sent to the optimizer.
See the existing reviewed [rg missing Case](../evals/rg-missing-search/CASE.md)
and [frozen Harbor tasks](../evals/trace-skill-benchmark/README.md).

Configure `CATENA_RSI_CASES_MANIFEST` and `CATENA_RSI_RUNS_ROOT`; Compose uses the
explicit [RSI overlay](../deploy/catena-mvp1/compose.rsi.yml). All Harbor bind paths
must be identical in Engine and Docker host. Docker control is privileged: only
trusted operator-reviewed Cases may be registered.

## Public job creation

Authenticate as the owner and configure the model through Connections. Submit
`POST /v1/analysis-jobs` with an `Idempotency-Key` and either `trace_id`, or
`agent_id` + `window_start`/`window_end` (at least two retained Traces):

```json
{
  "pipeline": "rsi",
  "trace_id": "<owned real Trace ID>",
  "objective": "Avoid repeated searches with unavailable tools",
  "options": {
    "case_id": "approved-case",
    "baseline": {"AGENTS.md": "<actual baseline instructions>"},
    "harness2": {"k": 1, "mode": "parallel"},
    "budget": {"model_calls": 12, "trials": 8, "seconds": 900},
    "validation_case_ids": []
  }
}
```

Go assembles redacted Evidence and owner credentials privately. The Engine request
schema is now `catena.rsi_request.v2`; old `algorithms` and `composition` options
are explicitly rejected. Old receipts and terminal Job documents remain readable.
No SDK, dynamic plugin discovery, arbitrary path or shell command is accepted.
Candidate edits allow only nonempty `AGENTS.md` and `.agents/skills/*/SKILL.md`,
with byte/file bounds and lineage/content fingerprints.

After Harness² freezes one candidate, Harbor runs a fresh baseline/candidate pair
on the optimization Case and on every optional distinct validation Case. The
minimum `k=1` budget is four upstream probes plus two comparison Trials (six
Trials); one held-out Case adds two. Grade/reward fields do not enter Harness²
proposer context. Every execution uses Harbor's native Job, Trial, Docker
Environment, installed Codex and Verifier. `RSICodex` only injects harness files
and private owner model credentials.

## Compare, artifacts and review

Poll `GET /v1/analysis-jobs/{id}`. Go persists the full bounded result as checkpoint
and candidate provenance: comparisons, verifier rewards, token counts, elapsed
seconds, nonzero command exits, upstream revision, Case fingerprints/confirmations,
model identity, and raw Harbor artifact directories/result hashes. Missing tokens
or USD are unknown, not zero. Nonzero shell exit counts are not task failure rates.
Failed executions retain their checkpoint rather than publishing a candidate.

Raw `result.json`, verifier files and native Codex trajectory remain under the
private Engine run volume. The UI shows their relative directories; it does not
publish an unauthenticated filesystem download endpoint. Preserve that volume
along with PostgreSQL/ClickHouse and execution receipts.

A candidate is `review_required` only when Go checks a valid post-freeze pair
citing its candidate and baseline identities. Probes/LLM selection alone leave it
`unverified`. Record the human decision in the UI or POST
`/v1/analysis-jobs/{id}/reviews`:

```json
{"artifact_id":"<candidate artifact>","decision":"adopt","reason":"<human assessment and limits>"}
```

The authenticated reviewer and server timestamp are retained in the Job. Decisions
are immutable, identical retries are idempotent, and reviewed Jobs cannot be
deleted through the ordinary API. Adoption records intent; files are never
installed automatically. One pair proves at most that the engineering path ran;
it does not establish general, causal or statistically significant improvement.
