# Catena Implementation Plan

Updated: 2026-08-11

## Current status

Catena is a standalone Go + React product. Go owns OAuth, Agent registration,
Agent-bound API keys, OTLP/Conversation ingestion, Trace queries and Evolution
Job orchestration. The embedded XiaoBaOS Runtime consumes retained Evidence
Packs and produces reviewable Agent assets. Target Agent execution remains
local.

## Completed milestone — trustworthy memory workflow

- [x] Persist every submitted memory extraction as an owner-scoped task.
- [x] List recent tasks on the Memory page and refresh active tasks after
      navigation, reload or use on another signed-in browser.
- [x] Keep terminal task state and source Conversation provenance visible.
- [x] Augment empty semantic graphs with truthful Conversation, Agent and
      same-Conversation provenance instead of rendering an isolated Fact.
- [x] Make the fixed account control visibly identifiable at narrow widths.

## Completed milestone — Catena-owned Trace storage

- [x] Replace the packaged third-party ClickHouse image with the pinned official image.
- [x] Move Trace storage and DSNs from the legacy database namespace to `catena`.
- [x] Remove legacy Trace attribute aliases and Scenario source naming.
- [x] Back up, migrate and row-count-verify the public `catena_spans` table.
- [x] Remove the legacy database only after public read/write smoke passes.
- [x] Prove the current source tree and running Compose stack contain no retired platform dependency.

## Completed milestone — observable memory extraction

- [x] Replace Qdrant Local with the private Qdrant Server mode already
      supported by GauzMem.
- [x] Proxy owner-scoped memory task progress through Go.
- [x] Poll and render extraction steps, terminal failure and retry in Conversation.
- [x] Verify submit → progress → completed memory → recall end to end.

## Completed milestone — owner-provided Evolution model

- [x] Move Provider, Base URL, Model and API Key configuration to API management.
- [x] Encrypt each owner's model API Key and return only key-present status.
- [x] Dispatch the owner model configuration ephemerally to each Evolution Job.
- [x] Remove deployment-managed `XIAOBA_LLM_*` model defaults.
- [x] Keep language and theme controls only in Settings.
- [x] Verify two owners cannot read or execute with each other's model config.

## Completed milestone — Agent-first onboarding

- [x] Replace the generic API-key form with `接入新 Agent` and one Agent-name
      input; do not ask for Runtime.
- [x] Atomically create stable `agent_id`, display name and Agent-bound key.
- [x] Force Conversation and Trace ownership from the Agent key.
- [x] Detect XiaoBaOS, Codex, Claude Code or generic OTel Runtime from accepted
      evidence and expose it as read-only status.
- [x] Preserve existing unbound keys as hidden ingestion compatibility.
- [x] Verify create → upload → automatic attribution → revoke end to end.
- [x] Replace raw-key creation with the guided name → configure → verify flow.
- [x] Separate Agent statistics from API-key management and remove the guided route.
- [x] Present one credential row per Agent; copy without a duplicate plaintext
      reveal panel.
- [x] Hide unregistered telemetry aliases from the primary Agent registry.
- [x] Gate Trace Farm and other evidence actions on real connection state.
- [x] Verify the complete journey in the browser at desktop and mobile widths.
- [x] Add one-click copy actions for every ingest endpoint.
- [x] Make Trace Farm enter an analysis overview instead of auto-opening a stale asset document.
- [x] Exposed the former deployment model readiness without leaking its credential
      (superseded by owner-provided LLM configuration).

## MVP1 — complete

- [x] GitHub OAuth, session and personal API-key lifecycle.
- [x] Project-scoped OTLP ingestion and ClickHouse Trace/Span query.
- [x] Canonical Agent grouping with source identity preserved.
- [x] XiaoBaOS user-visible Conversation ingestion.
- [x] Agent + time-window Trace Set selection.
- [x] Inspector → Evolution → Reviewer stage execution and persistence.
- [x] Evidence-linked `agent.md`, Skill, Role and XiaoBaOS Harness candidates.
- [x] Optional Conversation-derived GauzMem recall and graph view.
- [x] Document GitHub identity and the separate Trace-to-asset / Conversation-to-memory paths.
- [x] React product UI served by Go.
- [x] Local and public single-node Compose deployment.
- [x] Repository reduced to Catena-owned product modules and deployment code.

## Next milestone — reliable execution

- [ ] Add PostgreSQL worker lease, heartbeat and lease expiry.
- [ ] Add bounded retry, cancellation acknowledgement and crash recovery.
- [ ] Add queue depth, job latency and duplicate-execution telemetry.
- [ ] Run failure injection for Runner loss, database interruption and restart.
- [ ] Add backup/restore rehearsal for PostgreSQL and ClickHouse.

## Active milestone — canonical Runtime capture

- [x] Add `catena tap` using the pinned claude-tap capture engine.
- [x] Normalize model and tool exchanges into one Trace per user turn.
- [x] Cover Codex, Codex App, Claude Code, Hermes and OpenClaw from one CLI.
- [x] Verify fail-open upload and a real local Codex tool turn.

## Later

- [ ] Organization/team tenancy and RBAC.
- [ ] Quotas and retention policy.
- [ ] Approved asset publication to SkillHub/RoleHub.
- [ ] Multi-node deployment only after measured scale requires it.

## Acceptance

1. An authenticated Agent can upload OTLP and appear in the Agent Registry.
2. A bounded window with at least two Traces creates one idempotent Evolution Job.
3. Every stage and Candidate retains exact Trace provenance.
4. Catena never invokes the target Agent or fabricates a Release decision.
5. Web tests/build, Go tests/vet/race, and both Compose configurations pass.

## Verification log

- 2026-08-11: Rebuilt the React production bundle into Go's embedded Web
  assets and added a CI parity check between `catena-web/dist` and
  `control-plane/internal/control/web`. This prevents a direct Go build from
  serving a stale pre-Catena frontend when the normal multi-stage Docker build
  is bypassed.

- 2026-08-11: Catena Tap pinned `claude-tap==0.1.142`, passed ten Python
  tests for OpenAI Responses, Anthropic Messages, Chat Completions, tool-result
  pairing, authenticated OTLP upload and CLI delegation. A real Codex 0.147.0
  turn executed `pwd` through the wrapper and produced one canonical Trace with
  four Spans: Turn, Model, `exec` Tool and final Model. Go OTLP JSON decoding,
  Runtime inference and control-plane tests passed. The Tap suite is included
  in the repository CI path filter and Python 3.12 job.

- 2026-08-11: MVP1 release-candidate verification passed: Go unit tests,
  `go vet`, Go race tests, 39 Web tests, TypeScript typecheck, production Web
  build, local/public Compose rendering, whitespace checks and retired-platform
  source scans. The public deployment returned ready before repository sealing.

- 2026-08-10: Replaced the packaged Trace image with the pinned official
  ClickHouse 25.10.2.65 image and moved storage to `catena.catena_spans`.
  Disposable-volume compatibility, Go integration tests, production Native
  backup, row-count plus content fingerprint, public read/write smoke and
  post-cleanup scans passed. All 35,865 production Span rows were preserved;
  the retired database and runtime image were removed.

- 2026-08-10: Memory extraction receipts and status are now owner-scoped,
  PostgreSQL-backed task records. A real failed extraction remained visible
  after route navigation, page reload and Core restart with its source
  Conversation, Agent, step and display progress intact. Fact graphs add
  explicitly typed Conversation, Agent and same-Conversation provenance while
  keeping semantic edges distinct. The narrow shell exposes a fixed labeled
  account control. Go tests/vet/race, 39 Web tests, typecheck, production build and
  local Compose/browser acceptance passed.

- 2026-08-09: Restored production GitHub OAuth connectivity by routing the
  public Core container to a tested, configurable GitHub edge while retaining
  hostname TLS verification and the existing OAuth security boundary.

- 2026-08-09: The Web shell now exposes the authenticated GitHub identity and
  direct account switching in a persistent top-right menu. Desktop and 390px
  browser acceptance passed without navigation overlap or horizontal overflow.

- 2026-08-09: Production cold-load inspection found the memory graph library in
  the shared entry bundle. Route workspaces and the graph canvas now load on
  demand, reducing the initial JavaScript payload from 157 KB to 74 KB gzip;
  Web tests, typecheck and production build pass with the split chunks.

- 2026-08-09: Final release interaction audit passed all seven product
  destinations plus Home on desktop and 390px. Release blockers in mobile
  navigation and Conversation detail reflow were fixed; first-time Agent setup
  now has a direct entry and Run states no longer expose internal English enums.

- 2026-08-09: Terminal Evolution Jobs gained owner-scoped deletion while queued
  and running Jobs remain protected. Unit, PostgreSQL integration and browser
  acceptance verified two-step confirmation, immediate list removal and source
  Trace preservation.

- 2026-08-09: Conversation and Trace inspection were rebuilt as responsive
  master/detail workspaces. At the production-reported 684px viewport, browser
  acceptance verified index → detail → back navigation, distinct transcript
  cards, compact Trace summary, selected Span evidence, and no simultaneous
  list/detail stacking. Chinese Conversation previews now preserve valid UTF-8.
- 2026-08-09: 34 Web tests, TypeScript typecheck/build, Go tests and `go vet`
  passed for the inspection-workspace release.

- 2026-08-07: Owner-provided LLM configuration passed encrypted persistence,
  blank-key preservation, unsafe Base URL rejection, secret non-disclosure and
  cross-owner read/execution isolation tests.
- 2026-08-07: Browser acceptance verified API Management save/reload behavior,
  an empty secret field after persistence, and durable language/theme controls
  in Settings. The public deployment returned healthy with no shared
  `XIAOBA_LLM_*` model environment.

- 2026-08-07: Deployed the model-visibility release to the public single-node
  environment with a rollback image; Core returned healthy and the public home
  and Settings routes returned HTTP 200.
- 2026-08-07: Authenticated browser acceptance confirmed the online deployment
  renders the existing DashScope-compatible Provider, Base URL and Model while
  exposing only API-key readiness.

- 2026-08-11: Trace evidence now renders model conversation, tool arguments and
  terminal results as semantic UI instead of exposing request JSON by default.
  Runtime/system context and secondary fields remain folded with raw evidence
  available on demand. 41 Web tests, typecheck/build, Go tests, `go vet` and the
  control-plane race suite passed.
