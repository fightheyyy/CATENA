# Codex analysis → Harbor evaluation

Codex is the internal analysis/improvement Agent. CATENA owns evidence,
deterministic orchestration and engineering records. Harbor owns target execution
and verification. There is one concrete strategy, no algorithm/plugin registry.

## Analysis and the first human decision

Create `POST /v1/analysis-jobs` with `{"pipeline":"analyze","trace_id":"<owned Trace>"}`.
Analysis needs neither a Case nor Harbor. It loads trace-analysis in the same Codex
Runtime, advertises no evaluation tools and persists summary/problems, evidence
references, hypotheses and uncertainty. Reports remain unverified hypotheses.

The report page lets the user keep the analysis or explicitly start improvement.
Only an owned completed analysis can be used. A new improvement Job freezes its
report and original Evidence; `analysis_job_id`, report hash and full snapshot are
retained in provenance. There is no automatic transition or new approval queue.
The explicit authenticated create request records the first human decision;
candidate adoption still has its separate Human Review after evaluation.

## Improvement workflow after the human decision

1. Go freezes owner-scoped Trace Evidence and resolves the operator-confirmed Case.
2. Harbor runs a baseline probe.
3. One isolated Codex App Server turn explicitly loads the packaged improvement
   Skill, Evidence and score-free baseline observations. The pinned binary first
   registers the private Skill root with `skills/extraRoots/set` and confirms the
   enabled Skill through `skills/list`; a path-only input is insufficient at this pin.
4. Codex calls `evaluate_candidate` once with proposed file replacements.
   The host validates them and calls Harbor on the fixed training Case. Actual
   collected files and ATIF observations return; rewards/verifiers are withheld.
5. Codex composes the final harness. Omitted files inherit the successfully probed
   proposal; an explicit empty files list retains it. Proposal edits inherit the
   supplied baseline. Deletions are unsupported. The Workflow validates and freezes
   only after a successful native turn and candidate probe, then hashes the result.
6. Harbor runs fresh baseline and candidate trials on the training Case, then on
   every optional held-out Case. Held-out material never enters the analysis turn.
7. Go persists Compare, artifacts and provenance; a human records adoption/rejection.

The Skill borrows Harness²'s propose/compose idea. Its recursive algorithm,
selection, Library, dependency and custom Proposer were removed. This strategy
inherits no upstream experimental results. Internal analysis Runtime and target
Agent harness are separate: the analysis Skill is stored outside the target
workspace and rejected from generated harness files. The target has no Harbor
analysis tool. Arbitrary semantic contamination still requires human review.

## Register and deploy

Private operator manifest:

```json
{
  "approved-case": {
    "path": "/srv/catena-evaluation/tasks/case",
    "sha256": "<task_digest of all task/environment/verifier files>",
    "confirmed_by": "<real review record>",
    "artifacts": ["/workspace/case/result.json"]
  }
}
```

Native `task.toml` artifacts can declare the files instead. Manifest additions use
Harbor's native collection without modifying a frozen task. Effective declarations
are retained in results. Current target integration accepts single-step Harbor
Codex tasks and 1–32 explicit UTF-8 output files from the main service, totaling at
most 256,000 bytes. Missing declarations fail before paid work. Directories,
binaries, sidecars and multi-step tasks are unsupported. Final replies cannot
substitute for files. Symlinks and task fingerprint changes are rejected.

Set `CATENA_CASES_MANIFEST`, `CATENA_RUNS_ROOT` and persistent
`CATENA_ENGINE_EXECUTIONS_ROOT`. Use [compose.evaluation.yml](../deploy/catena-mvp1/compose.evaluation.yml)
with identical absolute task/run paths in Engine and Docker host. The Docker socket
is a privileged operator opt-in for trusted registered Cases.

## Public request

Configure the owner's model in Connections and submit with an Idempotency-Key:

```json
{
  "pipeline": "improve",
  "analysis_job_id": "<owned completed analysis Job>",
  "objective": "Investigate observed path-discovery failures",
  "options": {
    "case_id": "approved-case",
    "baseline": {"AGENTS.md": "Complete the supplied task faithfully."},
    "budget": {"agent_turns": 1, "trials": 4, "seconds": 900},
    "validation_case_ids": []
  }
}
```

Agent/window Evidence selection remains supported for analysis. Minimum budget is one
analysis turn and four Harbor trials: two probes plus an independent pair. Each
held-out Case adds two trials. Codex can make multiple provider requests within its
turn: `agent_turns` is not an inference-request, token or money cap. Native total
input/output usage is recorded when available; missing usage/cost is unknown.
Timeout and tool allowance bound the loop. There is no custom transport retry loop.

Analysis uses private schema `catena.analysis_request.v1` and `/v3/analyze`. Improvement uses private schema: `catena.improvement_request.v1`; endpoint: `/v3/improve`.
New artifacts use `improvement_candidate`; internal executor is `codex-skill`.
Old rsi/Harness² creation/options are explicitly rejected. Existing Jobs must never
be relabeled or replayed as this new strategy.

## Permissions and records

Codex has a private temporary HOME/CODEX_HOME, empty workspace and explicit Skill
input. Shell, MCP, plugins, hooks, subagents and host Skill discovery are disabled;
the sandbox is read-only. Dynamic requests are restricted to the active thread/turn
and single tool. Arguments cannot select a Case, owner, executor or host path.
Candidate files are limited to AGENTS.md and `.agents/skills/<name>/SKILL.md` with
byte/file bounds; the internal analysis Skill path/content is reserved. Unknown
requests fail closed. Identical repeated tool-call IDs reuse results; conflicting
inputs or extra probes abort. This does not establish malicious multi-tenant
Docker isolation.

Go remains the only durable scheduler: leases, heartbeat, fencing, retries, timeout
and cancellation remain. Engine receipts deduplicate external operations; uncertain
running receipts after restart require inspection. Cancellation awaits Harbor
cleanup before releasing the operation. One Engine owns each receipt root.

Raw Harbor TrialResult, ATIF, collected files, `candidate.json`, bounded key-redacted
native protocol and `result.json` stay under the private run volume. Go retains
Compare, hashes, Evidence references, Skill hash, candidate lineage and immutable
Review. Historical rsi candidates remain readable/reviewable; no completed records
or experiment directories are deleted. Stop old Workers and back up volumes before
deployment. Incomplete old pipeline Jobs fail without running the new strategy;
operator inspection and a fresh request are required.

Engineering memory means these auditable versioned records. Retrieval, automatic
reinjection, embeddings and a separate Memory engine remain unimplemented. One
comparison pair proves at most an engineering path, never general improvement.

The runtime uses the pinned binary's generated protocol schemas. Official guidance
describes [App Server dynamic tools and explicit Skill inputs](https://learn.chatgpt.com/docs/app-server);
the repository tests the exact `0.160.0` protocol rather than assuming newer APIs.
