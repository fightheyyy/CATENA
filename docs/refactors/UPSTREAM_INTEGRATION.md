# Upstream integration correction

> Current execution has moved to the [unified Codex Workflow](../IMPROVEMENT_WORKFLOW.md). This report describes the historical implementation and its evidence, not the current strategy.

The earlier `f540c32` integration used the real upstream Harness² algorithm but
misrepresented delivery: a final assistant reply became a fabricated `answer.txt`.
It also discarded upstream run directories and independently parsed Codex JSONL
despite Harbor already providing ATIF. The older successful Verifier pair remains
historical engineering evidence; it does not validate these corrected semantics.

## Changes and retained boundaries

- Deleted `engine/catena_engine/codex_artifacts.py` and its exclusive tests.
  Harbor's `Job.run().trial_results`, `TrialResult.compute_token_cost_totals()`,
  native timestamps and validated ATIF now supply evidence and metrics. Unknown
  shell exit counts remain null. `evaluation/evidence.py` only reads native ATIF
  and the Harbor artifact manifest; it executes no Agent or Verifier.
- `evaluation/harbor.py` accepts declared output file paths from native task
  artifacts or operator manifest additions. Harbor collects them before verifying.
  Frozen task files and fingerprints are unchanged. Effective declarations are
  retained as `case_artifacts`; missing declarations fail before paid work.
- `rsi/harness2.py` implements actual delivered-file semantics and renders ATIF
  observations with the upstream grammar and observation bound. It retains the
  upstream tree, selection, Library, Proposer audit and score-free Harbor linkage
  under the private experiment directory. Identical run IDs reuse their evidence;
  an incomplete execution is refused rather than paid for again.
- The custom `HostProposer` remains necessary for the current Codex channel.
  Upstream's pinned native Proposer only offers Claude CLI and OpenCode. The
  integration now labels its actual channel `catena-codex-json`, preserves system
  instructions/deadlines, records tokens and wall time, and reports one transport
  attempt. It does not claim native agentic session/cache/retry equivalence.
  Read-only/no-shell isolation and the validated harness edit surface remain.
- `rsi/contracts.py`, `rsi/host.py`, `rsi/api.py` carry actual deliverables privately,
  persist output hashes without publishing file contents, and expose the retained
  upstream directory. Library scope remains one experiment. No cross-task Library
  service, registry, new algorithm or general Adapter SDK was added.
- Go durable scheduling, owner isolation, receipts, review immutability and the
  React workflow remain intact. No schema migration, history deletion or automatic
  candidate installation occurs.

Documentation changed: root README, architecture README, integration guide,
Rebirth report and reproduction commands. Tests changed: RSI host/upstream/Harbor
tests plus `test_harbor_evidence.py` replacing the retired raw-parser tests.

## Verification

- Engine: **61 passed**, including the pinned upstream scheduler, actual-file
  selection, no-file non-delivery, run ID reuse/uncertain replay refusal, real
  channel audit, system/deadline forwarding and confined native artifact reads.
- Tap: **20 passed**. Frozen Case checks: **28 tasks / 116 verifier checks passed**.
- Go: `go test ./...` and `go vet ./...` passed, with the cross-process Engine test
  configured. Database integration/race and React suites were not rerun for this
  Python-only change; their previous phase results are historical.
- Ruff, pip dependency consistency and `git diff --check` passed.
- Final Engine image build and packaged source smoke results are retained in the
  [evidence directory](evidence/upstream-integration/).

Unit/protocol fixtures are not quality evidence. No new dependency was added.

## Real rerun: BLOCKED

The new Go job `analysis-job-1793915043484-46971a714c093570` used the same real Trace,
frozen `path-discovery-01`, pinned upstream Harness²/Harbor and `gpt-5.6-luna`.
The operator manifest added only `/workspace/case/result.json` to native Harbor
collection. The first rollout passed its real Verifier with reward 1; the actual
JSON file and ATIF were successfully read, linked and retained. This validates
that part of the integration, not optimization effectiveness.

Two optimizer calls completed. The second rollout hit upstream
`usage_limit_reached`, HTTP 429, before completing; Harbor recorded
`ApiRateLimitError`. CLIProxyAPI reported a reset at **2026-11-06 06:27:09 +08:00**.
No new candidate or Compare was published and no new human adoption was recorded.
The failed checkpoint and the earlier immutable case-only adoption were read back
from CATENA. Temporary model credentials were removed from the isolated test Core.
Raw native runs and optimizer evidence remain private; a redacted
[real-run summary](evidence/upstream-integration/real-run.json) records the hashes,
metrics, case fingerprint and incomplete work.

**Outstanding:** after provider quota recovers, restore the authorized connection
and submit a fresh idempotent Job using [the normal API client](../../evals/rebirth.py).
The current single-step text-file contract is documented in
[RSI_ADAPTERS.md](../RSI_ADAPTERS.md). The full six-Trial comparison is not verified
for this revision. Held-out/repeated quality improvement and native Proposer
equivalence remain unproven. Do not reuse the previous candidate's human decision
for a newly generated artifact.
