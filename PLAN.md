# Catena Implementation Plan

Updated: 2026-08-06

## Current Status

Catena has a standalone React/Vite product frontend served by the Go server.
Go owns OAuth/session, personal API tokens, product APIs, OTLP/HTTP ingress,
owner-scoped Trace and XiaoBaOS Conversation queries, and persistent Evolution
Jobs. The default Compose runs `catena-runner` in evolution-only mode. Target
execution and Release truth stay in local Barena; Catena runs only its embedded
XiaoBaOS Evolution Runtime to turn retained Evidence Packs into
evidence-linked `agent.md`, Skill, Role, and XiaoBaOS Harness assets.

## Active Milestone — Trace fuel to deployable Agent assets

- [x] Remove the Replay handoff contract and UI. Trace Farm produces only
      portable `agent.md`, Skill, and Role assets, plus Harness optimization
      when the canonical source Agent is XiaoBaOS.
- [x] Move memory generation to the XiaoBaOS Conversation path; current
      Trace-to-GauzMem retention remains compatibility code until that slice is
      removed after migration.

- [x] Add first-party XiaoBaOS Conversation ingestion, owner-scoped storage,
      and an intentionally small Conversation list/detail UI. Conversation is
      Memory fuel; OTLP Trace remains Agent-asset and Harness fuel.

- [x] Replace single-Trace Evolution creation with an Agent + bounded
      time-window Trace Set containing at least two server-frozen Trace IDs,
      while preserving existing completed Jobs as read-only compatibility
      records.
- [x] Make Agent classification and Agent Trace history the primary Evolution
      entry; keep single Trace detail limited to observation and Replay
      evidence.
- [x] Expose OTel-derived Agent classification with `identity_source`; disclose
      `service.name` as the MVP fallback without claiming canonical
      cross-Runtime Agent binding.
- [x] Resolve Codex live and historical OTel source aliases into one canonical
      Agent while preserving original per-Trace source identity and making
      Agent-scoped Trace Set selection span every alias.

- [x] Make each personal API-token row independently copyable and revocable,
      with hash-based authentication and encrypted-at-rest recovery.
- [x] Accept one API key for Barena Run Bundle and correlated OTLP upload.
- [x] Make cloud synchronization local-first: upload failure never changes the
      Barena evaluation result and remains retryable.
- [x] Preserve the legacy retained-Trace/Run Evolution path for existing API
      consumers and historical Jobs; do not expose it as the current product
      creation flow.
- [x] Persist and render InspectorCat → EvolutionCat → ReviewerCat stage state.
- [x] Render evidence-linked legacy candidates for retained Jobs.
- [x] Remove Barena target execution from the default cloud Runner profile;
      keep only the XiaoBaOS Evolution Runtime in the Catena product path.

### Milestone acceptance

1. One observed Agent plus a bounded time window containing at least two owned
   Traces creates one idempotent Evolution Job.
2. The server freezes the exact included Trace IDs before the three XiaoBaOS
   role stages start; every stage and candidate preserves plural provenance.
3. Single-Trace detail offers observation and Replay evidence only; explicit
   memory retention belongs to XiaoBaOS Conversation detail.
4. The Web shows OTel Agent classification, its `identity_source`, Agent asset
   kind, deployable content, and immutable Trace provenance.
5. A Barena-connected run remains locally complete when Catena is unavailable
   and can be synchronized later.
6. Catena never invokes or impersonates the target Agent in this journey.
7. XiaoBaOS records only user-visible messages in its local append-only Journal,
   projects them through API-key HTTPS, and Catena persists and renders them in
   owner-scoped order without admitting hidden prompts, reasoning, or Tool data.

## Milestones

### MVP1 — local evolution loop

- [x] LangWatch-derived Catena Web and Trace subsystem.
- [x] GitHub login, projects, API keys, and project isolation.
- [x] OTLP ingestion and Codex bounded history backfill.
- [x] Trace-derived Agent Registry.
- [x] Classify known Barena target telemetry separately from orchestration and
      evaluator evidence, keeping internal sources in Trace while excluding
      them from the Agent Registry and Trace Farm.
- [x] Scenario Explore through a registered HTTP Agent.
- [x] Go Run/Event/Issue/Case/Evaluation/Release state machines.
- [x] InspectorCat → EvolutionCat → ReviewerCat Evolution Jobs.
- [x] Barena Replay and factual Compare integration.
- [x] Four-container Compose topology and smoke test.
- [x] Self-contained repository build with pinned Barena and XiaoBaOS sources.

### MVP1 release hardening

- [x] Introduce `catena-web`, a standalone React/Vite product frontend with no
  LangWatch UI/runtime dependency.
- [x] Serve the production React build from `catena-server` and keep all
  browser calls same-origin.
- [x] Make Go the owner of GitHub OAuth, sessions, project/API keys, Trace
  ingress/query, Agent Registry, and evolution orchestration.
- [x] Implement Catena-owned OTLP/HTTP ingestion and ClickHouse Trace schema.
- [ ] Migrate retained LangWatch Trace data with stable trace/span IDs and
  verify tool-call waterfall parity before deleting the downstream.
- [x] Replace the current six-container deployment with four MVP containers:
  `catena-server`, `catena-runner`, PostgreSQL, and ClickHouse.
- [x] Remove Redis from the default product topology; PostgreSQL owns durable
  Job state. Add multi-worker leases before horizontal execution.

- [x] Replace the inherited feature catalog with one Catena journey:
  Connect Agent → receive Trace → choose Agent window → start Evolution. The
  authenticated shell exposes only Agent, Trace, Evolution, and Settings;
  Explore is an Evolution action rather than a separate product destination.
- [x] Replace the explanatory dashboard with a state-driven Guided Home for
  zero-Agent, waiting-for-Trace, and ready-to-evolve states.
- [x] Remove Catena mounts for inherited command bar, announcements, upgrade
  UI, workspace switching, and other LangWatch SaaS chrome.

- [x] Add a public Catena landing page at `/`: unauthenticated visitors see
  the product story and OAuth CTA, while authenticated users retain the
  existing project-home redirect.
- [x] Remove inherited SaaS plan visibility gates from Catena Trace reads;
  keep storage retention operator-controlled and preserve project RBAC and
  data-privacy redaction.
- [x] Reconstruct Codex history Tool Call/Result pairs as deterministic child
  spans and verify an existing tool-heavy rollout through the Trace read model
  used by the Waterfall.
- [x] Add a bounded Agent-semantic Trace lens over raw OTel evidence. Default
  detail exposes Turn, Model, Tool, Artifact, and Error steps, folds Runtime
  internals, and keeps raw spans behind incremental disclosure.
- [x] Run and retain one non-fixture XiaoBaOS Explore → Trace → Evolution
  journey against the configured model endpoint; keep Replay as the explicit
  local handoff because automatic candidate import is the next milestone.
- [ ] Complete a fresh-clone Compose acceptance on a clean machine.
- [ ] Remove remaining internal `spiral-*` compatibility identifiers after
  protocol consumers migrate.
- [x] Run Catena through dedicated server and browser route allowlists plus a
  Catena worker profile, and retain parity evidence for the smaller runtime.
- [x] Serialize first-request project-principal upserts in PostgreSQL so a
  fresh project's batched Evolution queries cannot race on identity indexes.
- [x] Canonicalize GitHub login to the configured callback origin before OAuth
  state/PKCE cookies are written, and give stale callbacks a safe retry path.
- [ ] Remove unreachable inherited source and unused event-sourcing pipelines
  only after the allowlisted runtime remains stable through beta acceptance.
- [ ] Add repository CI for focused Web tests, Go race tests, source-pin checks,
  and Compose configuration validation.

### Private beta

- [ ] Deploy behind HTTPS with rotated secrets and documented backup/restore.
- [ ] Add durable Runner job leases, heartbeat, retry, cancellation, and crash
  recovery before multi-node execution.
- [ ] Store large artifacts outside PostgreSQL and bind them to immutable
  evidence manifests.
- [ ] Add active Runtime adapters only where a real execution transport exists;
  OTLP observation remains framework-neutral.
- [ ] Add control-plane metrics, failure injection, and recovery SLO evidence.

### Personal Agent workbench — Conversation to memory

- [x] Add GauzMem as one internal Python service with a pinned source revision.
- [x] Retain GauzMem's implemented Neo4j graph path and local Qdrant index;
  exclude its duplicate Web/auth, Redis task store, and MinIO file path from
  the Catena integration.
- [x] Keep GauzMem MySQL isolated for the first working integration. Do not
  claim PostgreSQL migration until source/chunk/fact CRUD, temporal expansion,
  and recall-quality parity pass against the existing test suite.
- [x] Add owner-scoped Go APIs for explicit Conversation ingestion and three-path
  memory search. Browsers must never call GauzMem directly.
- [x] Add a minimal Memory surface and a Conversation “distill” action to React.
- [x] Preserve Conversation ID, Agent/Runtime, surface, and time metadata in
  every Conversation-derived memory source.
- [x] Verify opaque owner isolation, bounded/redacted Conversation
  conversion, ingestion receipt, and semantic/graph/temporal request contracts.
- [x] Keep Qdrant Local for MVP1 while patching the pinned GauzMem Step 8 path
  to reuse its existing client instead of opening a conflicting second client;
  remove the compatibility patch after the upstream pin contains the fix.
- [x] Restore the pinned GauzMem bundle-search graph expansion contract by
  adding the missing bounded async adapter to its infrastructure Neo4j store;
  remove the compatibility patch after the upstream pin contains the fix.
- [x] Run one credentialed live Conversation compilation and three-path recall
  with the LexVoice DashScope embedding credential and the local OpenAI-compatible
  model endpoint.
- [x] Make Memory graph-first: proxy one owner-scoped GauzMem Fact neighborhood
  through Go and render real selectable Fact/Entity/Relation evidence in React.

## Next Steps

1. Add PostgreSQL job leases, heartbeat, retry, cancellation, and recovery.
2. Migrate retained spans and verify tool-call waterfall parity.
3. Retire Trace-to-memory writes after Conversation-memory parity is proven.

## Owners

| Area | Owner |
| --- | --- |
| React product UI | `catena-web` |
| Auth, tenancy, APIs, OTLP, Trace, durable jobs | Go Catena server |
| Evaluation and release semantics | Barena |
| Evaluation/evolution role execution | XiaoBaOS Runtime |
| Conversation-derived memory compilation and recall | GauzMem behind the Go gateway |
| Target behavior and native telemetry | External Agent Runtime |

## Acceptance Criteria

- Unauthenticated `/` renders the public Catena page without protected API
  calls; its primary CTA starts the selected OAuth provider and preserves `/`
  as the callback. Authenticated `/` still resolves to the user's workspace.
- GitHub login succeeds when the landing page was opened through either
  `localhost` or `127.0.0.1`; missing or mismatched callback state remains a
  rejected flow and is presented with a restart action rather than raw JSON.
- Exactly four MVP Compose services become healthy.
- Missing project keys are rejected at Run and OTLP ingress.
- One Trace can retain an Issue, immutable Case, Replay Evaluation, and Release
  Gate without fabricated facts.
- Evolution outputs remain proposals and cannot directly create a Release.
- Cross-project requests fail closed.
- TypeScript tests, Go race tests, focused Web tests, SDK backfill tests, and
  fresh-clone Compose validation pass.
- The React bundle contains no LangWatch runtime dependency, and the Go binary
  serves its static assets plus SPA fallbacks.
- Existing retained trace/span IDs and tool-call relationships remain visible
  after migration.
- Normal OTLP and Conversation ingestion creates no memory implicitly; an
  authenticated user can explicitly retain one owned XiaoBaOS Conversation
  and retrieve it through a recall bundle whose metadata links back to that
  Conversation.

## Verification Log

- 2026-08-06: the standalone Catena Trace detail passed authenticated browser
  acceptance on a live 42,537-Span Codex Trace. It folded 41,763 internal spans,
  exposed 340 model and 433 Tool/Artifact steps, bounded both semantic and raw
  views to 200 mounted rows, and produced zero console errors. All 29 React
  tests, typecheck, production build, rebuilt Compose, and MVP smoke passed.

- 2026-08-06: credentialed Conversation-to-memory acceptance passed against the
  live stack. One authenticated XiaoBaOS Conversation completed all 8 GauzMem
  stages in 41.4 seconds, producing 14 Facts, 4 explicit relations, and 26
  extracted entities through DashScope embeddings and the local model proxy.
  The Memory UI rendered all 14 Facts with stable Conversation provenance and
  recalled `release-brief.md`, the original Conversation, and its Topic for the
  Chinese query “今天的 Agent 发布检查简报叫什么？”. After repairing the pinned
  graph-expansion adapter, the final three-path UI request completed in 4.379
  seconds without an expansion error; a direct Neo4j acceptance check expanded
  every one of the 14 seed Facts. All seven active containers were healthy,
  Compose configuration validation and `git diff --check` passed, and no
  credential was written to the repository.

- 2026-08-06: memory ownership moved from single Trace to XiaoBaOS
  Conversation. Go now reads one authenticated owner's visible Conversation,
  builds a bounded/redacted document, preserves stable Conversation/Agent
  provenance, and submits it through the private memory boundary. React removed
  the Trace memory action, added “提炼为记忆” to Conversation detail, and
  rewrote Memory states around Conversation fuel. Full Go tests/vet, 23/23
  React tests, typecheck, production build, rebuilt Compose, and authenticated
  browser acceptance passed. Credentialed live recall was completed in the
  later acceptance run above.

- 2026-08-06: Trace Farm asset-boundary acceptance passed. New Jobs no longer
  ask InspectorCat for Case or EvolutionCat for Memory/Case. The runtime accepts
  `agent_md`, Skill, and Role for every Agent and Harness only for canonical
  XiaoBaOS; `agent.md` uses a stable path/markdown content contract. React
  removed the Replay handoff and legacy Case/Memory cards, rendered the retained
  Skill directly, and copied its asset content successfully. Full Go tests, vet,
  control-plane race tests, 22/22 React tests, typecheck, production build,
  rebuilt four-service stack, and authenticated browser acceptance passed with
  zero console errors.

- 2026-08-06: XiaoBaOS Conversation acceptance passed end to end. The local
  Journal recorded and exported one ordered user/assistant exchange containing
  text, a delivered file, Role, and a shared 32-hex Trace ID through a personal
  API token into PostgreSQL; the authenticated React Conversation workspace
  rendered the same two messages with zero browser errors. XiaoBaOS passed the
  full repository test command, `npm run build`, and 52/52 focused tests. Catena
  passed the full Go suite, vet, control-plane race tests, a real PostgreSQL
  idempotency/isolation round trip, 22/22 React tests, typecheck, production
  build, and the four-service Compose smoke test. The temporary acceptance token
  was removed after the run.

- 2026-08-06: Barena source classification acceptance passed. The central Go
  resolver maps exact `barena-xiaoba-target` telemetry to canonical
  `xiaobaos / XiaoBaOS`, excludes the Barena orchestrator, User Simulator,
  Inspector, and Reviewer from Agent-scoped reads, and keeps unknown OTel
  sources fail-open. Real retained data now renders one XiaoBaOS Agent with 2
  Traces / 12 Spans / 0 Errors and one live source; Trace Farm matches exactly
  those two target Traces. The four internal Traces remain unchanged in
  ClickHouse and the global Trace contract. Go full/race/vet, the real
  ClickHouse round trip, 19 React tests, typecheck, production build, rebuilt
  Compose service, and authenticated browser acceptance passed.

- 2026-08-05: canonical Agent identity acceptance passed. Catena now resolves
  `codex`, `codex-app-server`, and `Codex Desktop` to one `codex / Codex`
  Agent with explicit live/history source records. Agent-scoped Trace queries
  and Evolution Trace Set selection expand all aliases while Trace detail
  retains the original `service.name`. Go full/race/vet, the real ClickHouse
  round trip, 19 React tests, TypeScript typecheck, and production build passed.
  The rebuilt four-service stack stayed healthy; browser acceptance rendered
  exactly one Codex row with 3,977 owned Traces at refresh, two source badges,
  one Trace Farm option, and zero console warnings/errors.

- 2026-08-05: Agent Trace Set acceptance passed. `go test ./...`, `go vet
  ./...`, and `go test -race ./...` passed; focused coverage proves that zero
  or one matching Trace is rejected with `422` and no Job is persisted. The
  React suite passed 17/17 together with typecheck and the production build.
  The rebuilt four-service Compose stack was healthy. Authenticated browser
  acceptance classified `Codex Desktop` through the disclosed `service.name`
  fallback, loaded all 29 of its recent Traces through the Agent-scoped query,
  and completed Job `evolution-job-1785938466699-1ca39c9ec534d530` over 12
  server-frozen Traces selected from 29 matches. InspectorCat, EvolutionCat,
  and ReviewerCat completed with plural provenance and produced only
  `draft/unverified` proposals; the target Agent was not invoked. The Trace
  page retained observation/Memory actions and exposed no Evolution mutation.

- 2026-08-05: Catena connected a real Codex Desktop installation with a
  dedicated personal API key and imported 29 unique completed turns from 25
  current-day rollout files in two accepted OTLP batches. The importer now
  serializes OTLP/JSON trace/span byte fields as Base64, while Go recognizes
  reconstructed `langwatch.input/output` and standard
  `gen_ai.tool.call.arguments/result` evidence. Focused TypeScript tests passed
  19/19, the SDK production build passed, the Go control package passed under
  the race detector, and browser acceptance opened a 21-span Codex Trace with
  20 child Tool Calls whose inputs and outputs were visible.

- 2026-08-05: a non-fixture local Barena Explore drove XiaoBaOS Base through
  the configured `gpt-5.5` endpoint, retained 9 native OTLP envelopes / 18
  spans, and correctly failed the ambiguous-planning behavior with task success
  0.2. Catena atomically retained its Run Bundle, then completed real
  InspectorCat → EvolutionCat → ReviewerCat stages over the primary target
  Trace and produced one `draft/unverified` Case plus one Role candidate. The
  Reviewer accepted proposal grounding while explicitly rejecting it as
  verification or Release evidence. Four-service Compose smoke, Go race/vet,
  PostgreSQL integration, React build/tests, Runner route isolation, Replay
  handoff provenance, and a zero-error browser console passed. Full evidence:
  `docs/acceptance/CATENA_MVP1_DEMO.md`.

- 2026-08-05: standalone GitHub OAuth now normalizes an alternate loopback
  login host to the configured `localhost:5670` callback origin before issuing
  state/PKCE cookies. Strict callback rejection remains intact; stale flows
  return to a bilingual recovery state instead of exposing a JSON problem.
  Go race/vet, React build, live redirect headers, and a Playwright recovery →
  GitHub authorization regression passed.

- 2026-08-05: GauzMem was pinned as a private Compose service with isolated
  MySQL authority, local Qdrant storage, and Neo4j. Go now owns readiness,
  owner-scoped recent-memory, explicit Trace retention, and three-path recall
  APIs; the React Memory workbench passed desktop/mobile browser acceptance
  with zero console errors. `go test -race ./...`, `go vet ./...`, `pnpm build`,
  Compose validation, live service health, and a signed owner-scoped list call
  passed. Live semantic recall remains gated only by an embedding credential.

- 2026-08-03: Barena TypeScript suite passed 186/186.
- 2026-08-03: Go control-plane `go test -race ./...` passed.
- 2026-08-03: focused Catena product/navigation/settings/ingress/evaluator
  tests passed 29/29.
- 2026-08-03: Codex installer and history-backfill tests passed 55/55.
- 2026-08-03: the fresh Catena Compose build started exactly six healthy
  services and passed auth, protected-ingress, storage, and Runner smoke checks.
- 2026-08-03: running MVP retained 4,894 unique Traces and 6,918 spans across
  Codex, Claude Code, XiaoBaOS, Scenario, and Barena services.
- 2026-08-03: Catena runtime slimming reduced the formal client artifact from
  1,984 files / 106 MiB to 778 files / 26.7 MiB, cut the tRPC surface from 83 to 42
  top-level routers, stopped four unrelated standalone worker loops, passed
  focused boundary tests and six-service smoke, and completed an authenticated
  ten-route browser plus real OTLP-ingestion acceptance with zero console
  errors. A PostgreSQL identity-scoped transaction lock also passed a 12-way
  fresh-project concurrency test and the complete Go race suite.
- 2026-08-03: the Catena app moved from a source-over-upstream development
  image to a production-only multi-stage build. The image fell from 612.7 MiB
  to 407.3 MiB (33.5%) and now sits 12.4 MiB above the 394.9 MiB upstream
  LangWatch image. The rebuilt image passed six-service smoke and a fresh
  authenticated ten-route browser acceptance; retained OTLP evidence remained
  visible in both Agent Registry and Trace Explorer.
- 2026-08-03: Catena Compose gained complete GitHub/Google OAuth passthrough
  with startup validation for missing provider credentials. GitHub is the
  recommended developer-product login; email remains a local-only fallback.
  Browser acceptance observed a stateful GitHub authorization request with the
  exact Catena callback and no console errors.
- 2026-08-04: Catena GitHub login began requesting GitHub's account picker with
  `prompt=select_account`; the full LangWatch provider keeps its default OAuth
  behavior. Focused tests and the live stateful authorization URL verified the
  provider-owned account-selection handoff.
- 2026-08-04: the public Catena landing page was redesigned as a bilingual,
  responsive black-and-white editorial experience using the Catena mark and a
  real Evolution workspace capture. Four focused tests, Biome, the production
  client build, six-service health, live browser acceptance, locale switching,
  and the GitHub OAuth redirect chain passed.
- 2026-08-04: Catena stopped applying LangWatch SaaS plan visibility windows
  to Trace reads. Thirty-eight focused tests passed, the production image was
  rebuilt, all six services returned healthy, and browser acceptance opened a
  29-day-old Codex Trace with its real Waterfall and no upgrade gate.
- 2026-08-04: Codex rollout recovery began emitting one deterministic child
  span per Tool Call/Result pair instead of hiding tool evidence inside the
  turn input. Parser/export tests, the 323-test governance suite, TypeScript
  typecheck, and focused lint passed. Re-importing real Trace
  `32de8077a7e3bdd5bc99b5a589e745ca` produced 19 unique tool spans under its
  turn root with paired arguments/results and rollout-derived durations.
- 2026-08-04: the standalone React/Vite client built without LangWatch or a UI
  framework and was embedded into the Go server with SPA fallback. Go accepted
  authenticated OTLP protobuf/JSON, stored owner-scoped spans in ClickHouse,
  returned Trace and Agent APIs, and rendered a three-span tool-call waterfall
  with input/output evidence. Unit tests, a real ClickHouse round trip, the
  production image build, desktop browser acceptance, and a zero-error console
  check passed.
- 2026-08-04: the public `5670` entrypoint moved from LangWatch to the
  Go-served React product without deleting database volumes. The existing
  GitHub callback path remains compatible, OAuth reached GitHub with PKCE and
  account selection, all six migration services became healthy, the Go Trace
  store and four-role XiaoBaOS Runtime reported ready, and desktop/mobile
  browser acceptance completed with zero console errors. LangWatch remains on
  `5671` only as the temporary retained-data rollback source.
- 2026-08-04: the cutover OAuth callback retained the former
  `/api/auth/callback/github` route. PKCE state/verifier cookies now use the
  root path and clear both legacy and native callback paths, preventing the
  callback alias from losing OAuth state. The compatibility-path OAuth test,
  full Go suite, vet, rebuilt container, smoke test, and in-app browser landing
  check passed with zero console errors.

## Risks / Open Questions

- The current Runner is one local service and does not recover an in-flight
  role turn after host loss.
- Active browser Explore requires a reachable HTTP Agent; OTLP alone provides
  observation, not control.
- Until Trace migration parity passes, both old and new Trace paths exist. The
  exit condition is an ID-stable data migration plus query/detail acceptance;
  LangWatch must then be removed rather than maintained as a permanent second
  backend.
- Public cloud operation requires a separate security and backup acceptance.

## Status Maintenance Rules

Mark an item complete only when code and reproducible evidence exist. Update
both this plan and [SPEC.md](./SPEC.md) whenever ownership, trust boundaries,
or persistent contracts change.
