# Catena Control Plane Plan

Updated 2026-08-06.

## Current Status

The repository contains a standalone Go control plane and React product
surface. Go owns authentication, OTLP ingress, owner-scoped Agent/Trace queries,
owner-scoped XiaoBaOS Conversation ingestion/query, PostgreSQL Job state, and
orchestration of a restricted XiaoBaOS Evolution Runtime. LangWatch remains a
migration/rollback source, not the product request path.

The final boundary is locked: Barena runs Explore, UserCat, target interaction,
Replay, Compare, verifier, and Release Check beside the target Runtime. Catena
owns durable OTLP/Run evidence and the multi-user evolution workflow. Its
restricted XiaoBaOS Evolution Runtime consumes Evidence Packs through
InspectorCat, EvolutionCat, and ReviewerCat and emits portable Agent assets plus
XiaoBaOS-only Harness optimization.
The target Agent Runtime stays external; Go does not reimplement Agent
reasoning or verifier semantics and never recomputes a Barena release verdict.

## Active Milestone: Edge evidence to cloud evolution

- [x] Remove current Case/Memory generation and Replay handoff semantics from
      the Agent Trace Set path. Accept `agent_md`, Skill, and Role for all
      Agents, and Harness only for canonical XiaoBaOS.

- [x] Add `xiaoba.conversation_batch.v1` API-key ingress, idempotent
      PostgreSQL persistence, owner-scoped list/detail reads, and Trace
      correlation without changing OTLP ingestion.

- [x] Unify direct standalone API-key ingress for Barena Run Events, terminal
      Run Bundle facts, and correlated OTLP.
- [x] Keep local Barena completion authoritative when cloud synchronization
      fails; retain a retryable sync state.
- [x] Preserve the versioned single-Trace Evidence Pack and optional Run context
      for legacy API consumers and completed Jobs only.
- [x] Build a versioned Agent Trace Set Evidence Pack from one observed Agent
      plus a bounded time window containing at least two owned Traces; freeze
      the exact included Trace IDs before execution.
- [x] Return OTel-derived Agent classification with `identity_source`, using
      `service.name` only as the disclosed MVP fallback.
- [x] Merge known Codex live/history source aliases into one canonical Agent;
      expand canonical filters for Trace queries and Trace Set snapshots while
      preserving original Trace source identity.
- [x] Classify known Barena target telemetry as the observed Agent while
      keeping its orchestrator, User Simulator, Inspector, and Reviewer as
      Trace-only internal evidence.
- [x] Execute InspectorCat → EvolutionCat → ReviewerCat without invoking the
      target Agent.
- [x] Persist evidence-linked legacy candidate records for compatibility.
- [x] Remove target execution and deterministic Replay from the default cloud
      Runner profile after the edge synchronization path passes acceptance.

### Acceptance

- One observed Agent plus a bounded time window resolves to at least two owned
  Traces; the server freezes their exact IDs into an immutable Evidence Pack.
- The Agent Trace Set completes one three-stage XiaoBaOS Evolution Job and keeps
  plural source provenance on every output.
- The candidate page shows stage evidence, provenance, content, and an explicit
  unverified state.
- Single-Trace detail contains no cloud Evolution action; it remains an
  observation, Memory, and Replay-evidence surface.
- Agent classification discloses `identity_source` and source aliases; known
  Codex sources resolve to one Agent and unknown sources retain the explicit
  `service.name` fallback.
- Catena outage changes only sync status, never the local Explore verdict.
- No Catena service invokes or stores credentials for the target Agent.

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
- [x] Add encrypted token recovery, owner-only no-store reveal, legacy
      hash-only compatibility, and restart-safe PostgreSQL persistence.
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
- [x] Serialize PostgreSQL compatibility-principal upserts per external
      identity and cover the fresh-project fan-out race with an integration
      test.
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
- [x] Normalize standalone GitHub login to the configured callback origin,
      preserve strict state/PKCE validation, and verify localhost/127.0.0.1
      regression coverage.

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

- [x] Add owner-scoped Agent Trace queries and create Evolution from one Agent
      plus a bounded time window containing at least two Traces, freezing the
      exact source Trace IDs.
- [x] Introduce the versioned Agent Trace Set Evidence Pack and plural
      provenance while retaining completed single-Trace Jobs as compatibility
      records only.
- [x] Remove the single-Trace Evolution creation route from the product Web
      flow after Agent Trace Set creation passes API and browser acceptance.

### GauzMem private memory service

- [x] Add an owner-scoped Fact graph read contract through Go, retain the
      private GauzMem project boundary, and verify request scope, safe provider
      errors, and live Neo4j-backed Fact/Entity/Relation output.

- [x] Keep GauzMem behind a replaceable `MemoryBackend`; browsers never call
      the Python service or choose their own tenant namespace.
- [x] Expose owner-scoped recent-memory, recall, readiness, and explicit
      Conversation-retention APIs from Go; keep Trace retention compatibility
      API-only.
- [x] Preserve source Conversation, Agent/Runtime, surface, time, and visible
      content provenance while bounding and redacting it before it leaves
      Catena.
- [x] Verify the pinned GauzMem image, MySQL authority, local Qdrant index,
      Neo4j graph path, Go race suite, and live private-network health.
- [x] Move the current product write path to authenticated XiaoBaOS
      Conversation documents with stable Conversation provenance; retain the
      older Trace method only for API compatibility.
- [x] Patch the pinned GauzMem Step 8 Qdrant Local lock failure by reusing the
      already initialized project-scoped vector client. Remove this patch when
      the upstream pin includes the same correction.
- [x] Patch the pinned GauzMem bundle-search mismatch by adding the missing
      bounded async graph-expansion adapter to its infrastructure Neo4j store.
      Remove this patch when the upstream pin includes the same correction.

- [x] Promote the Go control plane to the only Catena product backend in the
      standalone migration slice.
- [x] Serve the standalone `catena-web` React build with SPA fallback.
- [x] Move project/API-key and Trace ingress/query ownership into Go.
- [ ] Add PostgreSQL job leases, heartbeat, retry, cancellation, and recovery.
- [ ] Preserve TypeScript/Python engines behind versioned worker contracts;
      do not port evaluation logic to Go.
- [x] Remove LangWatch from the public proxy/auth path; retain its legacy port
      only until retained Trace parity.

- [x] Lock Catena as the platform name and Barena as the embedded release
      engine; record the three-service/six-container ADR.
- [x] Retain persistent, tenant-isolated, idempotent single-Trace Evolution Jobs
      for legacy API/history compatibility, with optional terminal Run context
      and no synthetic Run. Do not expose this creation path in the current Web.
- [x] Retain InspectorCat -> EvolutionCat -> ReviewerCat stage state and expose
      Finding, `draft/unverified` Memory/Role/Skill/Harness/Case candidates, and
      proposal-only Review.
- [x] Accept one owner-scoped, immutable `barena.run_bundle.v1` with a
      hash-verified opaque terminal fact; retain the legacy three-step edge
      ingest endpoints only for compatibility.
- [x] Build and retain a bounded/redacted
      `catena.evolution_evidence_pack.v1` from real stored Trace spans/tool
      evidence plus optional Run Bundle facts, and expose the explicit
      no-target-execution/no-Release boundary.
- [x] Expose the workflow through the signed project gateway and bilingual
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

- 2026-08-06: the owner-scoped Fact graph contract passed unit, full, vet, and
  race verification. `GET /v1/memories/facts/{fact_id}/graph` derives the
  private GauzMem project from the authenticated owner, forwards the private
  service credential, strips `project_id`, and hides provider failures. The
  live route returned Fact 27 with four Entities and two typed Relations.

- 2026-08-06: live Conversation memory acceptance passed through the Go-only
  public boundary. The private GauzMem pipeline completed 8/8 stages in 41.4
  seconds and persisted 14 Facts plus 4 relations; the authenticated React
  client then recalled the expected `release-brief.md` Fact, original
  Conversation, and Topic. The final three-path request completed in 4.379
  seconds without an expansion error, and direct Neo4j acceptance expanded all
  14 seed Facts. The LexVoice DashScope credential was injected only into the
  running private service and was not stored in the repository.

- 2026-08-06: the Agent-asset protocol passed focused and full control-plane
  tests. Inspector emits only a finding; Evolution accepts `agent_md`, Skill,
  and Role for any Agent, rejects Memory/Case, rejects Harness for Codex, and
  accepts Harness for canonical XiaoBaOS. Go full tests, vet, and race tests
  passed; historical Candidate/Case fields remain readable.

- 2026-08-06: Conversation control-plane acceptance passed. Personal API-token
  and signed-gateway ingestion enforce XiaoBaOS-only visible message schemas,
  exact retries are idempotent, mutations and sequence conflicts fail atomically,
  and list/detail reads remain owner-scoped. The full Go suite, vet,
  `go test -race ./internal/control`, and a real PostgreSQL integration run
  passed.

- 2026-08-06 Barena target/internal identity: exact
  `barena-xiaoba-target` now resolves to canonical `xiaobaos / XiaoBaOS` and
  expands only to XiaoBaOS target aliases. Exact Barena engine, User Simulator,
  Inspector, and Reviewer sources return no selectable Agent identity; direct
  internal Agent paths fail validation, while unknown sources still use the
  disclosed `service.name` fallback. Focused handler and Evolution tests prove
  that only target Traces enter a Trace Set and candidate provenance. The real
  ClickHouse integration preserves raw source names and all global Traces while
  returning only the target through `xiaobaos`. Go full/race/vet and the live
  2-Trace / 12-Span read model passed.

- 2026-08-05 canonical Agent identity: the deterministic resolver maps
  `codex`, `codex-app-server`, and `Codex Desktop` to `agent_id=codex`, returns
  auditable live/history `sources`, and preserves unknown `service.name`
  identities. Agent Trace reads and Evolution membership checks expand the
  alias family without rewriting raw Trace summaries. `go test ./...`, `go
  vet ./...`, `go test -race ./...`, and the real ClickHouse integration test
  passed; the live owner read model returned one Codex row across both retained
  sources.

- 2026-08-05 Agent Trace Set acceptance: `go test ./...`, `go vet ./...`, and
  `go test -race ./...` passed. Focused HTTP tests cover plural immutable
  provenance and reject zero/one-Trace windows with `422` without persisting a
  Job. The rebuilt PostgreSQL/ClickHouse/Go/Runner Compose path remained
  healthy and completed live Job `evolution-job-1785938466699-1ca39c9ec534d530`
  for Agent `Codex Desktop`: 29 Traces matched the seven-day window, the server
  froze 12 error-first/newest Trace IDs, and InspectorCat, EvolutionCat, and
  ReviewerCat completed without invoking the target Agent. The stored Finding,
  Case proposal, candidate, Review, and Replay handoff retain the Agent,
  bounded window, Evidence Pack digest, and plural Trace provenance.

- 2026-08-05 legacy compatibility: Trace-only Evolution accepts any
  owner-scoped stored OTLP Trace
  without synthesizing a Run, persists a bounded/redacted Evidence Pack, and
  retains InspectorCat -> EvolutionCat -> ReviewerCat output as provenance-bound
  `draft/unverified` candidates. Canonical `barena.run_bundle.v1` ingest is
  atomic and idempotent, validates exact terminal fact SHA-256 and 12 KiB/1 MiB
  limits, and associates both Event and `run.input.trace_ids` Traces. Focused
  contract tests, the complete Go race suite, vet, and diff checks passed.

- 2026-08-05: OAuth host-normalization tests prove that `127.0.0.1` receives no
  flow cookie and redirects to `localhost` before GitHub authorization. A stale
  callback remains rejected and receives a safe restart location. The complete
  Go race suite and vet passed against the rebuilt service.

- 2026-08-04: the Go CSP now allows only Catena-owned images plus GitHub's
  official avatar origin. The authenticated Web loaded the persisted GitHub
  avatar at 420x420 source resolution and rendered it at 28x28 with no browser
  console errors; focused Go tests, vet, rebuilt service, and smoke passed.
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
