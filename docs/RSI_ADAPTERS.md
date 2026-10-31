# RSI Adapter backend

Catena owns the optimization contract, host services and experiment records. An
algorithm is replaceable; it does not select arbitrary executors, paths or model
credentials. This release provides a real adapter around upstream Harness²
`1cfe254e8d25b15f37048c3730134686abe60335`, Apache-2.0, plus an explicit registry
and sequential/parallel composition. HarnessPro is **not implemented**.

## Current scope

The single formal entry is authenticated Go `POST /v1/analysis-jobs` with pipeline=rsi. Go owns durable scheduling, leases, heartbeat, fencing, retries and artifacts. Engine privately serves `/v2/rsi`, `/v2/rsi/adapters`, `/v2/cases` and execution receipts. No synchronous public optimization entry, RSI CLI, fixed Role protocol or SDK rollback remains.

Install Python 3.12 `pip install -e '.[dev,rsi]'`. The pinned upstream Harness² and reusable Harbor adapter share one environment; no OpenAI Agents SDK extra is needed. The Engine image includes this extra and Codex CLI 0.160.0. Docker access and Case mounts are explicit operator configuration, never automatically granted.

The owner supplies the model through Connections. Default reasoning effort is low. No model/provider availability is inferred from a preflight. The registry remains explicit; HarnessPro is not implemented. The public request options are the existing case_id/baseline/algorithms/composition/budget/validation_case_ids. Go creates evidence and attaches credentials internally; clients cannot supply paths, import classes or owner headers to the private Engine.

## Register existing Cases

Reuse checked-in Harbor tasks, for example
`evals/trace-skill-benchmark/tasks/python-environment-selection-01`. Create a
private operator manifest mapping a stable Case ID to an absolute task directory
and `task_digest(path)` from `catena_engine.evaluation.harbor`. It fingerprints all task
files, including environment and verifier files; symlinks are rejected. Example
shape (replace the path and digest; this is not a runnable fingerprint):

```json
{
  "python-env-01": {
    "path": "/absolute/CATENA/evals/trace-skill-benchmark/tasks/python-environment-selection-01",
    "sha256": "<task_digest of this directory>"
  }
}
```

```sh
export CATENA_RSI_CASES_MANIFEST=/private/cases.json
export CATENA_RSI_RUNS_ROOT=/private/rsi-runs
```

HTTP callers submit only registered IDs, not filesystem paths, import paths or
shell commands. Operators are responsible for approving the task image, Docker
configuration and verifier. A raw Trace without a replayable Case is insufficient.
The task directory is hashed again before and after each trial.

## Request

Private Engine payload `catena.rsi_request.v1` (Go constructs this, not a public request):

```json
{
  "schema": "catena.rsi_request.v1",
  "request_id": "rsi-example-1",
  "case_id": "python-env-01",
  "baseline": {"AGENTS.md": "Complete the supplied task faithfully."},
  "evidence": "Synthetic observation: the first interpreter lacked a required package.",
  "evidence_refs": ["trace:<id>/span:<id>"],
  "composition": "sequential",
  "algorithms": [{"name": "harness2", "options": {"k": 1, "mode": "parallel"}}],
  "budget": {"model_calls": 6, "trials": 6, "seconds": 900},
  "validation_case_ids": [],
  "model": {
    "provider": "openai",
    "base_url": "https://your-responses-provider.example/v1",
    "model": "gpt-5.6-luna",
    "api_key": "<supplied privately>"
  }
}
```

For CLI use, omit `api_key` from the file and provide `RSI_MODEL_KEY` through an
existing protected environment. Calling the following **does spend model
budget**; install/list/test commands above do not:

```sh
/tmp/catena-rsi/bin/catena-rsi --request /private/request.json --output /private/result.json
```

For HTTP use, run the worker with `CATENA_ENGINE_TOKEN` and send the private
bearer token. Do not publish the Engine or use its empty-token local default on a
shared network. No new public Go endpoint is added.

## Algorithm, Runtime, Evaluator boundaries

`RSIAdapter.optimize(Context, OptimizationServices, options)` returns exactly one
`Candidate` plus diagnostics. Register implementations with `Registry.register`;
unknown names and all options are validated before the first host operation.
Each adapter declares its minimum `Requirements`; the host budgets the whole
composition and reserved validation without branching on algorithm names.
There is no dynamic Python import from a request. Runtime and Evaluator are
separate host protocols; registry/host composition has no Harness² imports.

Sequential composition feeds each candidate to the next adapter and retains
intermediate candidates. Parallel composition gives each adapter the same input
baseline; it does not automatically rank or adopt the outputs. A failed branch
cancels siblings and waits for their host-operation cleanup. The complete
composition shares a single operation slot, wall deadline and budget ledger.
Algorithm computations may overlap; model calls and Harbor trials are serialized.

Candidate IDs fingerprint content, parent version IDs and algorithm identity.
`content_sha256` separately identifies identical file contents, including no-op
edits. Only `AGENTS.md` and `.agents/skills/<name>/SKILL.md` are editable: no
scripts, hooks, plugins, arbitrary paths, deletions or new Roles. Harbor installs
these validated files into the target workspace before executing its native Codex
adapter. Both baseline and candidate use fresh Docker trial environments.

Harness² runs its upstream Propose–Probe–Compose, selection and reflection code.
Its original file-edit proposer is adapted into a structured JSON edit envelope
through the **existing** read-only, ephemeral Codex transport. Evidence is supplied
as named file contents in the prompt; context above the input cap is rejected,
not silently truncated. This differs from upstream's tool-enabled file browsing
and unrestricted editable surface; do not transfer upstream performance claims.
Historical Trace evidence is supplemental: fresh probes are still performed.
The first evaluator presents the final assistant answer and native tool trajectory,
not rendered office documents or arbitrary generated files. Use answer-oriented
Cases when assessing optimizer behavior. Its per-run upstream library is temporary;
cross-request improvement-memory persistence is not enabled.

## Feedback, budgets and results

The optimizer receives task instructions, historical evidence, executed
trajectories and agent answers. It does not receive verifier files or rewards.
Reflection experiences remain unverified experiment diagnostics; nothing is
written to GauzMem/OpenViking or automatically installed in a user's Agent.

`k=1` schedules four Harbor probes and three improvement stages; reflection adds
an improver call, and `k>1` adds a selector call. Invalid edits may require extra
calls. Preflight checks the minimum budget; all attempts, including failures,
consume the shared counters. `model_calls` bounds **optimizer invocations**, not
the target Agent's individual model requests. Target execution is bounded by
trial count, task limits and remaining wall time. Tokens are measured separately
for optimizer and target trials; unknown trial usage remains null. Completeness
flags identify missing usage from failed operations; optimizer totals sum only
completed-call usage. Dollar cost is
unknown, never reported as zero. This is not a provider-enforced token/money cap.

Optional validation Case IDs must differ from the optimization Case. Their
trials are reserved before optimization and run after candidates are frozen:
one original baseline and each terminal candidate per validation Case. These
results never go back to the optimizer. Distinct IDs alone do not prove a strict
held-out split; operators must ensure fixtures and answers do not overlap.

Responses use `catena.rsi_result.v1`, report candidate lineage, Case fingerprints,
model/runtime settings, trial summaries and usage. Probe-only results say
`evidence_insufficient`; validated results say `review_required`. Neither means
an improvement was established. Failed optimization publishes no candidates but
retains completed-operation usage. Full native trial artifacts remain private
under the configured runs root; HTTP reports omit raw transcripts/answers.
`result.json` is written with mode 0600 inside a per-experiment 0700 directory.
Harbor trial profile is 1 CPU / 1024 MiB / concurrency 1 for both arms, recorded
in the report. These are configured limits, not measured performance guarantees.

## Lifecycle and deployment limits

Go cancellation, lease loss and timeout issue private DELETE to the owner-scoped Engine receipt. Codex and Harbor await cancellation/cleanup. HTTP disconnect alone is not cancellation: a retried Go lease can reconnect to the same active execution. Completed receipts reuse results; interrupted running receipts after Engine restart reject automatic replay. Receipt directories must be persisted and used by one Engine instance. This is external-operation deduplication, not a second queue or distributed transaction. See [Engine lifecycle](CODEX_ENGINE_RUNTIME.md).

Harbor's native Codex executes tools inside Docker and does not inherit the
analysis CLI's read-only/tool-disabled constraints. Use trusted task images and
host-level egress restrictions. Public benchmark answers must be blocked from
target executors. This adapter does not create an egress firewall or claim a new
sandbox guarantee. Do not give untrusted callers Docker access. Same-user host
processes remain within the model-key trust boundary.

## Validation

CI installs `.[dev,rsi]` into one clean Python environment and runs the full suite, including pinned upstream Harness², Harbor adapter, bounded Codex transport, private receipt idempotency/cancellation and Memory owner isolation contracts. Go separately tests the existing durable queue against PostgreSQL and a cross-process Engine protocol fixture. No deterministic fixture measures real model quality or improvement.

These tests make **no paid model calls**. Harbor adapter tests use simulated Job
execution with real Harbor config classes; they are not a real Docker benchmark.
No claim is made yet that Harness² improves Catena's real-model success rate,
Token usage or cost.

## Public queued request

```json
{"pipeline":"rsi","trace_id":"<retained-32-hex-trace>","adapter":"harness2","options":{"case_id":"python-env-01","budget":{"model_calls":6,"trials":6,"seconds":900}}}
```

Send with a stable `Idempotency-Key` to `/v1/analysis-jobs`. Alternatively supply agent_id/window_start/window_end. Result Artifacts retain complete candidate/trial provenance in the original Job JSON. No successful Harbor rate or real model validation is claimed by protocol fixtures; see V2 acceptance.
