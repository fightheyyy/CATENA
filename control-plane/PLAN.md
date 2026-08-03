# Catena Control Plane Plan

Updated 2026-08-01.

## Current Status

The repository already contains a working local vertical slice: Go controls
Run lifecycle and PostgreSQL persistence, a Node worker invokes the TypeScript
Engine, and an embedded React client renders live evaluation evidence.

The Catena repository is the selected Apache-2.0 platform downstream. MVP1
joins its Web/Trace subsystem and Go control plane through an authenticated
server-side proxy. A genuine
OTLP/protobuf XiaoBaOS Explore Trace can become an evidence-backed Issue, one
reviewed immutable Case, a canonical Replay, a persisted Evaluation, and a
Release Gate in the same browser workflow.

The final boundary is locked: the LangWatch-derived Platform uses its existing
Scenario UI and orchestration for registered HTTP Agent Explore and owns the raw
Trace subsystem; its User Simulator and trace-aware Judge call the restricted
XiaoBaOS Runtime as UserCat and ReviewerCat. The TypeScript Engine owns
deterministic Replay and Release Check; Go owns the multi-user evolution
workflow. The target Agent Runtime stays external, and Go does not reimplement
agent reasoning, verifier, or OTLP.

## Current Milestone: Embedded XiaoBaOS evolution Runtime

### Scenario Explore evaluator adoption

- [x] Add a restricted internal Scenario role-turn endpoint to
      `catena-runner`; only `user-cat` and `reviewer-cat` are accepted.
- [x] Preserve Scenario's UI, queue, multi-turn orchestration, registered HTTP
      adapter, Trace collection, and result schema while replacing only its
      evaluator agents.
- [x] Make the embedded evaluator the Catena default without requiring a
      second project Model Provider; retain upstream behavior outside Catena.
- [x] Export evaluator OTel with project/run/scenario correlation and verify one
      live HTTP Explore end to end.

Verification (2026-08-03): the Runner accepted only UserCat/ReviewerCat Scenario
turns, propagated project/run/scenario telemetry context, and drove a real
two-turn registered HTTP Agent Explore. The preserved Scenario result UI showed
PASSED (1/1), the transcript and Reviewer reasoning, and opened a correlated
four-span target Trace. Focused platform tests passed (93 passed, 2 skipped),
Engine protocol tests passed (7/7), both typechecks and the production build
passed, and all six Compose services were healthy.

### Stop metrics

- [x] A dedicated worker probes XiaoBaOS ordinary chat and all four cloud roles.
- [x] The worker can execute an isolated allowlisted role turn and rejects all
      target/functional roles before spawning a process.
- [x] Go exposes one sanitized ready/blocked Runtime manifest and lifecycle
      status without leaking host paths.
- [x] The LangWatch-derived Web displays the cloud Runtime and four roles while
      making the external target boundary explicit.
- [x] TypeScript, Go, fork typecheck/build, and real local XiaoBaOS probe pass.

### Embedded Runtime complexity budget

- Reuse the TypeScript `AgentRuntimeAdapter` and XiaoBaOS ordinary chat loop.
- Add one four-role worker contract; add no managed target Runtime, Runner
  daemon, private-network tunnel, second Trace store, or universal scheduler.
- Keep Go responsible for lifecycle and records, not model reasoning.

## Current Milestone: Platform HTTP Explore to Release

### Stop metrics

- [x] A registered, reachable HTTP Agent can be selected and exercised by the
      existing Scenario runtime from the browser.
- [x] The completed Scenario run exposes retained target Trace IDs and can be
      adopted exactly once as a canonical Barena Explore Run without copying
      credentials or recomputing the Scenario Judge.
- [x] The adopted Run enters the existing Issue -> immutable Case workflow.
- [x] A no-secret standard HTTP Case replays through the TypeScript Engine and
      produces a deterministic artifact-verifier-backed Release Gate;
      unsupported HTTP Agents fail closed with a useful reason.
- [x] Compare renders exact facts for two compatible terminal Explore Runs and
      explicitly creates no release decision.
- [x] A deterministic XiaoBaOS-compatible HTTP fixture passes the browser flow,
      focused tests, type checks, production client build, and diff checks.

### Complexity budget

- Reuse the existing Scenario runner, HTTP Agent adapter, run drawer, Trace
  subsystem, Evolution workbench, Go state machine, and TypeScript verifier.
- Add no managed target Runtime, Runner daemon, scheduler, private-network tunnel,
  second Trace store, second Scenario implementation, or user portal.
- Persist no HTTP authorization value, arbitrary header value, query secret,
  or body template in Barena Run/Case records.
- Keep Platform Compare read-only and factual. Only the Engine Release Check
  may emit `cleared`, `held`, or `rejected`.

### Dispatch ledger

| Child goal | Write owner | Expected benefit | Lifecycle | Parent disposition |
| --- | --- | --- | --- | --- |
| `platform_product_map` | read-only product inventory | Freezes the developer journey and minimal information architecture | complete | accepted; Explore / Evidence / Evolution boundary adopted |
| `scenario_runtime_map` | read-only Scenario execution inventory | Identifies the real HTTP execution, live state, Judge, and Trace seams | complete | accepted; existing Scenario runner reused |
| `barena_integration_map` | read-only Go/Engine contract inventory | Finds the smallest honest adoption and Replay bridge | complete | accepted; adopt-without-reexecution and fail-closed HTTP Replay chosen |

The parent goal owns all production edits, cross-layer contracts, browser
acceptance, and final verification.

## Completed Milestone: MVP1 Case to Replay and Release

### MVP1 stop metrics

- [x] From the LangWatch-derived Web, create an Issue from retained Run/Trace
      evidence and promote it to exactly one immutable Case.
- [x] Start that Case as a local Replay Run through the existing TypeScript
      Engine Worker; do not introduce a second evaluator or Runner.
- [x] Persist one owner-scoped Harness Version, Evaluation, and Release record
      from the hash-verified Run Package and Engine terminal decision.
- [x] Show Case, Replay progress, Evaluation evidence, Release decision, and
      source Trace lineage in one project-scoped Web workflow.
- [x] Pass deterministic browser E2E, PostgreSQL integration, Go race/vet,
      TypeScript build/tests, and LangWatch frontend typecheck/build.

### Complexity budget

- Production write scopes are limited to the existing TypeScript Engine
  protocol/replay seam, `platform/internal/control`, and one isolated
  LangWatch-derived Workbench plus its server-side Go proxy.
- Add no package dependency, cloud scheduler, queue, Go Runner, Trace store,
  evaluator, Runtime abstraction, or public browser-to-Go authentication path.
- Add at most three Go persistence tables, seven Go endpoints, three focused
  Go production files, one focused TypeScript compiler file, and one Web
  workflow route with its local components/proxy.
- Keep one canonical Case-to-Replay request contract:
  `input.platform_case` plus an absolute `case_base_dir`.

### Dispatch ledger

| Child goal | Write owner | Expected benefit | Lifecycle | Parent disposition |
| --- | --- | --- | --- | --- |
| `mvp1_ts_case_replay` | TypeScript Engine replay/compiler seam | Removes the Case schema-to-Replay protocol break | complete | accepted and integrated |
| `mvp1_go_release` | `platform/internal/control` | Adds persistent Case/Run/Evaluation/Release state machine | complete | accepted and integrated |
| `mvp1_langwatch_map` | read-only LangWatch fork inventory | Removes high-risk navigation/auth/proxy uncertainty before Web edits | complete | informed fork implementation |

The parent goal owns contract alignment, all cross-layer glue, the LangWatch
write set, final verification, and MVP1 acceptance.

- [x] Freeze the endpoint/cloud boundary and the first Web product loop.
- [x] Freeze source-of-truth, repository, authentication, and data ownership
      across the TypeScript Engine, Platform fork, and Go service.
- [x] Add revocable, owner-scoped personal API tokens for Barena Runner.
- [x] Add edge Run creation, ordered Event ingestion, and explicit completion.
- [x] Preserve the existing local worker path as compatibility mode.
- [x] Refocus primary navigation on Explore, Traces, History, and Settings.
- [x] Make endpoint setup discoverable from Settings.
- [x] Verify local and edge Runs through the same evidence UI.
- [x] Audit LangWatch licensing and select the Apache-2.0 community boundary.
- [x] Create the `fightheyyy/CATENA` downstream fork with an explicit
      upstream-sync and enterprise carve-out policy.
- [x] Start the full local platform stack and verify project creation, API-key
      auth, OTLP ingestion, Trace search, span metadata/events, and waterfall.
- [x] Apply the first downstream shell patch: Barena title, logo, Explore
      naming, and onboarding copy.
- [x] Reposition the product around the continuous-evolution flywheel rather
      than a generic Trace/Eval dashboard.
- [x] Add evidence-backed Issue and immutable Case models to the Go domain,
      memory store, PostgreSQL migration, and owner-scoped API.
- [x] Require a promoted Trace ID to exist in the source Run's retained Engine
      Events and make promotion idempotent.
- [x] Prove one retained Run can become an Issue and exactly one Replay Case
      without copying or fabricating Trace content.
- [x] Compile a reviewed `barena.case.v1` into the canonical Engine Replay
      contract without losing Runtime, input, verifier, or provenance fields.
- [x] Execute the promoted Case at the endpoint and persist the resulting
      Evaluation and Release decision with links back to Case and Trace.
- [x] Add a fork-side authenticated workflow gateway with timestamped HMAC
      signatures that bind method, URI, project, actor, and body.
- [x] Replace `BARENA_PLATFORM_TOKEN` with one fork-issued project API key for
      both Run gateway and OTLP setup; do not expose a second Go credential.
- [x] Scope fork-originated Go tenancy to signed fork-project context while
      preserving existing Run IDs and compatibility principals.
- [x] Persist and checksum-validate immutable Run Packages, scorecard facts, and
      Engine-produced decision records without recomputing Release Check.
- [x] Add Harness Version lineage and durable source/replay Trace correlation
      to Evaluation and Release records.
      after the first Trace-to-Case slice is accepted.
- [x] Add the Release Workbench page to the fork and bind it to Go Run, Case,
      and Decision APIs.
- [x] Export a deterministic XiaoBaOS Explore Run to the fork and retain
      browser evidence for both Trace and release state.
- [ ] Retire the custom React shell, custom target OAuth path, and per-Run Trace
      viewer after Workbench parity.
- [ ] Retire Go-owned sessions/personal tokens and the Go-managed local Node
      worker after project-gateway, endpoint-push, cancellation, and Workbench
      parity tests pass.

## Active Milestone: Unified Platform Authentication

- [x] Enable GitHub OAuth at `catena-app` through the six-container deployment
      without duplicating OAuth state in Go.
- [x] Accept one fork-issued project API key for both OTLP and edge Run/Event
      ingress.
- [x] Resolve the key only in `catena-app`, enforce project permissions, and
      forward an HMAC-signed project principal to Go.
- [x] Update the TypeScript endpoint client to use the public Catena origin and
      `BARENA_PLATFORM_API_KEY`.
- [x] Verify invalid-key denial, project isolation, complete edge lifecycle,
      GitHub provider configuration, and Compose wiring.

## Deferred

- Project/team tenancy beyond the existing user owner boundary.
- Cloud-to-local orchestration or a persistent endpoint tunnel.
- A Go Runner, cloud scheduler, job leases, and heartbeats. Add them only after
  a cloud-triggered execution requirement is accepted.
- Community data flywheel and public profiles.
- Compare as a primary Web workflow.
- Enterprise-only LangWatch modules under `langwatch/ee/`.
- Automatic forwarding of compatibility-mode per-Run OTLP capture to the fork.
  Production XiaoBaOS endpoints should export to the fork directly; the local
  MVP acceptance fixture imports the retained protobuf envelopes explicitly.

## Active Milestone: Catena Trace-to-Evolution MVP1

- [x] Lock Catena as the platform name and Barena as the embedded release
      engine; record the three-service/six-container ADR.
- [ ] Add persistent, tenant-isolated, idempotent Evolution Jobs sourced from a
      terminal Run and retained Trace.
- [ ] Retain InspectorCat -> EvolutionCat -> ReviewerCat stage state and expose
      Finding, Case proposal, draft Candidate, and proposal-only Review.
- [ ] Expose the workflow through the signed project gateway and bilingual
      Evolution page.
- [ ] Run Barena engine and XiaoBa evolution workers behind a functional
      `catena-runner` in Compose.
- [ ] Verify Go race/vet, TypeScript tests/build, fork tests/typecheck/build,
      Compose smoke, API isolation/idempotency, and complete browser E2E.

## Ownership

- Barena Platform fork: authentication, projects, API keys, OTLP ingest, Trace
  storage/search, Scenario/Explore presentation, and the Web shell.
- Go continuous-evolution control plane: Run/Event ingest, state transitions,
  SSE, Issues, Case promotion, Harness Version lineage, immutable Run Package
  validation, persisted scorecard/release records, audit, and Trace
  correlation. It does not compute evaluations or release decisions.
- TypeScript Engine: User Simulator, Agent Connector, Inspector, Reviewer,
  verifier, Explore, Replay, Compare, scorecard, and Release Check semantics.
- Endpoint Runner: Agent execution, artifact capture, Run Event upload, and
  OTLP export.
- Release Workbench: joins Trace evidence from the fork with canonical
  evaluation and release state from Go.

## Verification

- Target integration tests prove fork project-key validation, signed internal
  context, cross-project isolation, edge Event idempotency/order, Run Package
  integrity, completion, and no cloud worker launch.
- Existing Go integration tests continue to pass.
- TypeScript Engine tests and build continue to pass.
- Fork Web typecheck/build succeeds; the embedded Go Web remains a compatibility
  check only until it is removed.
- Browser QA covers Explore entry, retained target evidence, adoption, Case
  promotion, Replay, Release Gate, and Compare without a product-blocking
  request failure.
- Fork integration tests prove that project authentication is checked before a
  Run Event reaches Go and that cross-project Run access fails closed.
- Go domain tests prove source ownership, Trace correlation, Issue validation,
  idempotent promotion, and immutable Case revision creation.

## Verification Log

- Browser acceptance started registered HTTP Agent Scenario Run
  `scenariorun_0004MQPAovVIySBtXW4I213vhjPsW`. Trace
  `929e40cd8045ac94e07f01e5febb233d` retained nine spans, including the real
  W3C-parented `SerializedHttpAgentAdapter.call -> xiaoba.role.turn` boundary.
  The target span was exported by the XiaoBaOS-compatible HTTP fixture through
  OTLP; it was not synthesized by Barena.
- The terminal Scenario facts were adopted as Go Run
  `run-platform-702c474ef35f5275e22c803f` without re-execution or Judge
  recomputation. The same source produced Issue
  `issue-1785495217466-73113c9f751e2d62`, immutable Case
  `case-1785495239484-d0e46f2fda52f418`, Replay
  `run-1785495248023-1f7787e7b5b24692`, Evaluation
  `evaluation-run-1785495248023-1f7787e7b5b24692`, and Release
  `release-run-1785495248023-1f7787e7b5b24692`. The verifier-backed Release
  decision was `cleared`; replay Trace
  `437358120d57525800b150afc9affd06` remains linked.
- Three compatible terminal adopted Explore Runs rendered in Compare with
  exact source status, duration, Judge verdict/criteria, completion time, and
  Trace links. The view explicitly states that it is evidence comparison, not
  a release decision.
- Final acceptance passed all 172 TypeScript tests, `npm run build`, focused
  HTTP fixture/Replay tests, `go test -race ./...`, `go vet ./...`, Go server
  build, 10 focused fork tests, full fork typecheck, production bundle build,
  targeted Biome checks, and whitespace checks in both repositories.
- The deterministic acceptance used a local OpenAI-compatible model double for
  Scenario User Simulator/Judge repeatability. Provider/model labels in this
  fixture are evidence metadata, not a claim that a live hosted model ran.
- Unified-auth acceptance on 2026-08-02 passed 178 TypeScript tests and build,
  Go race/vet, fork typecheck and production build, 45 focused API-key tests,
  signed edge-ingress integration, and six-container smoke. Live project
  `spiral-e2e` rejected an invalid key (401), completed Run
  `run-1785613925383-d5cfaf24cccd7cd8`, accepted OTLP (200), and exposed one
  matching stored span in ClickHouse. A real GitHub callback remains an
  operator acceptance step because OAuth App credentials are external secrets.

### Historical compatibility verification

- HTTP integration proves a completed Explore Run with a retained Trace ID can
  create an Issue, reject an unrelated Trace ID, and promote exactly one
  immutable `barena.case.v1` revision.
- PostgreSQL integration proves migration, Trace correlation, atomic
  Issue-to-Case promotion, and idempotent retries against PostgreSQL 17.
- TypeScript build and all 166 repository tests pass.
- Go race tests and `go vet` pass, including OAuth ownership, Token
  hashing/revocation, evidence-free completion rejection, Event ordering,
  idempotency, and edge completion.
- Web typecheck and production build pass; the Go binary serves the generated
  assets.
- A real TypeScript client created an `origin=edge` Run through the running Go
  server, appended a terminal Event with a Trace ID, completed it, and rendered
  the same result/OTel evidence view used by local Runs.
- Desktop and 390px mobile browser QA pass with zero current-page console
  errors. Settings stops the History polling loop while open.
- Local Barena Platform v3.7.0 returned HTTP 200 with zero rejected spans for a
  five-span Explore trace. Trace Explorer showed one trace, five spans,
  `barena-xiaobaos`, `gpt-5.6-sol`, tool/artifact events, all four actor stages,
  and a 7-second waterfall.
- The same instance accepted all eight genuine OTLP/protobuf envelopes retained
  by a prior XiaoBaOS Explore run and rendered four original
  `xiaoba.session` traces with their source durations and span counts.
- Browser evidence:
  `output/playwright/barena-platform-otlp-waterfall.png` and
  `output/playwright/barena-platform-xiaoba-native-traces.png`.

## Risks

- The existing Go schema is user-owned while the target is fork-project-owned.
  Migration must preserve Run IDs and fail closed if a project mapping is
  missing; dual authorization must have an explicit removal point.
- The fork and Go may share a PostgreSQL instance but must use separate logical
  schemas/databases and versioned APIs; cross-service table reads would collapse
  the ownership boundary.
- Edge cancellation is initially cooperative state: the endpoint must poll or
  receive a future command stream. The Platform must not imply it killed a
  process it cannot reach.
- OTLP ingestion and Run Event ingestion are separate channels. Correlation
  depends on propagated `trace_id`, `run_id`, and W3C Trace Context.
- Upstream merges can touch the same navigation/auth surfaces as Barena.
  Downstream patches must remain isolated, and `langwatch/ee/` must never be
  treated as Apache-2.0 community code.
