# Unified Codex runtime implementation and acceptance

The current architecture runs one internal Codex analysis/improvement Agent,
one concrete Skill and one controlled Harbor Tool inside a deterministic Workflow.
The real Trace → Case → Skill → Tool → freeze → independent Compare → Human Review
path completed. The user rejected the generated candidate; no files were installed.
This is engineering acceptance, not evidence of general Agent improvement.

## Removed and retained

- Removed `engine/catena_engine/rsi/`: native Harness² scheduler integration,
  Bench, HostProposer, temporary/library execution model, JSON model transport and
  their exclusive tests. Harness² was removed from package dependencies and the
  test environment. No upstream algorithm code was copied into CATENA.
- Added one `improvement.py` Workflow and one packaged improvement Skill. The
  existing Codex process isolation/cleanup code now drives native App Server
  requests, explicit Skill loading and one `evaluate_candidate` tool. Codex owns
  the reasoning/tool loop; CATENA owns deterministic limits and stage transitions.
- Preserved Harbor native Job/Trial/environment/verifier, real ATIF and artifact
  collection, the thin Codex target extension, case fingerprints and operator
  confirmation. Output hashes preserve collected UTF-8 bytes, including CRLF.
- Preserved Go PostgreSQL queue, lease/heartbeat/fencing, cancellation/retry,
  owner isolation, Evidence, artifacts and immutable Review. New admission uses
  `pipeline=improve`; obsolete rsi/adapter/algorithm options fail explicitly.
  Historical terminal jobs, candidate types and reviews remain readable.
- Engineering memory reuses Evidence, versions, Compare and Review records.
  No retrieval engine, embedding service, automatic reinjection, plugin framework,
  second strategy abstraction or additional middleware was added.

Engine production Python shrank from **17 files / 1,699 lines** at `02a6907` to
**13 files / 1,392 lines**: **307 fewer lines**. This count excludes tests,
documentation, Skill text and generated Web assets. Four permanent services remain.

## Files and contracts

| Area | Changes |
|---|---|
| Engine | `contracts.py`, `improvement.py`, `runtime/codex.py`, `runtime/cancellation.py`, Skill, private server/receipts, native artifact reads |
| Go | `analysis_job.go` admission/private invocation/current artifact type; artifact/review compatibility; contract and retirement tests |
| React | current request, current/historical artifact normalization, corrected product text, browser Fixture, regenerated embedded bundle |
| Deployment | Harbor-only evaluation extra, packaged Skill data, renamed evaluation overlay/environment, native Skill check in CI |
| Documentation | README, SPEC, architecture, runtime, Workflow, deployment and historical-report notices |

See [the current Workflow](../IMPROVEMENT_WORKFLOW.md) for requests, budget semantics,
permissions, exact stage order and upgrade requirements. Stop old Workers before
deployment; unfinished old jobs are never reinterpreted as the new strategy. No
database schema migration, table deletion or rewrite of completed records occurred.

## Real experiment

Case: operator-confirmed frozen `path-discovery-01`, linked to the retained real
Trace `89b6c10a758a66675fb5af3307972197`. Case fingerprint and confirmation are in
the [real-run evidence](evidence/unified-runtime/real-e2e.json).

Job: `analysis-job-1793925382008-bf733740e4183870`.
Model: the authorized CLIProxyAPI `gpt-5.6-luna`; Codex `0.160.0`, Harbor `0.23.0`.

The packaged Engine matched all 13 source files and Skill hash. The native Skill
was registered and confirmed enabled, then Codex issued exactly one native Harbor
tool call. All four genuine Harbor trials completed with valid Verifier reward 1.
The final target candidate contains AGENTS.md only, with no internal analysis Skill.

| Independent pair | Baseline | Candidate |
|---|---:|---:|
| Verifier pass / trials | 1 / 1 | 1 / 1 |
| Input tokens | 46,295 | 47,545 |
| Output tokens | 570 | 656 |
| Total tokens | 46,865 | 48,201 |
| Native trial seconds | 36.209806 | 35.553457 |
| Harbor-reported USD | 0.00321532 | 0.00340724 |
| Native tool calls | 5 | 5 |

Analysis used one native turn, reporting 226,277 input and 705 output tokens.
Turn count is not provider request count or a money cap. Costs above are Harbor
reports, not invoices. No held-out or repeated statistical test was conducted.

The candidate added overly broad JSON-output and task-text trust rules. The user
explicitly chose **reject** after seeing the comparison and limitations. That
decision was written through the real Review API and read back. Core/Engine
restart preserved the new Compare, candidate and rejection exactly, while the
earlier case-only adoption remained unchanged. Temporary model credentials were
removed from the isolated test Core; the authorized provider configuration itself
was not changed. A private current PostgreSQL backup was saved.

## Failures retained, not counted as acceptance

The [attempt history](evidence/unified-runtime/attempts.json) preserves three prior
runs. The first finished its trials but copied the internal analysis Skill into
the target: excluded. Later runs exposed an empty composition contract and an
illegal result.json edit, which was refused before execution.

The main root cause was verified using the real pinned App Server against an
offline HTTP provider: a path-only Skill input outside discovery roots did **not**
inject its body. Registering the private root did. Production now registers and
confirms the Skill before starting a model turn. An offline native check is in CI;
it is explicitly protocol evidence, not a model-quality test. The final fresh real
run used the corrected loading and edit contracts; older results were not replayed
or edited to make them appear successful.

## Verification and remaining limits

- Engine: **40 passed**; native Skill loading refusal, tool scope, duplicate calls,
  process-group timeout/cancel cleanup, stream bounds, candidate/analysis separation,
  inherited edits, comparison reserve and private receipt contracts.
- Go: full PostgreSQL/ClickHouse integration, race and cross-process tests, plus
  vet, passed. A base-only Engine environment with neither Harbor nor Harness²
  also passed the private cross-process Fixture test.
- React: **38 unit / 12 browser tests**, TypeScript and production build passed.
- Tap: **20 passed**. Frozen suite: **28 tasks / 116 verifier checks passed**.
- Skill validator, Ruff, dependency check, Compose+evaluation overlay, packaged
  source/Skill hashes and offline native Skill loading check passed.

Evidence and source counts are under [evidence/unified-runtime](evidence/unified-runtime/).
Raw model/Trace/Harbor records remain private. Fixture and reference-solution checks
are distinct from the four real model trials. Production capacity, malicious
multi-tenant Docker isolation, other target Agent adapters, retrieval/reinjection
and general optimization effectiveness are not established. The user requested
engineering completion only; no further candidate tuning or paid trials followed.
