# Catena Implementation Plan

Updated: 2026-08-19

## Current status

Catena is a standalone Go + React product. Go owns OAuth, Agent registration,
Agent-bound API keys, OTLP/Conversation ingestion, Trace queries and Evolution
Job orchestration. The embedded XiaoBaOS Runtime consumes retained Evidence
Packs and produces reviewable Agent assets. Target Agent execution remains
local.

## Active milestone — DeepSeek Harness compatibility

- [x] Recognize Barena's exact DSH bridge markers as Runtime `dsh` without
      broad product-name heuristics.
- [x] Persist the frozen source Runtime on an Agent Trace Set Evolution Job.
- [x] Let EvolutionCat emit a reviewable `dsh_plugin` bundle only for DSH
      evidence, with `package.json` and `cordis.patch.yml` validation.
- [x] Render and download DSH Plugin bundles from the existing Asset Library.
- [x] Verify one DSH → Barena Explore → Catena Trace Farm → Barena Plugin
      acceptance loop without moving target execution into Catena.

## Active milestone — asset-first Evolution

- [x] Make Trace Farm open on the accumulated Agent Asset Library instead of
      analysis history.
- [x] Limit product assets to `agent.md`, Skill packages and Role packages.
- [x] Render package files with readable content, per-file copy/download and
      exact Trace provenance.
- [x] Keep analysis progress and raw role output as a secondary audit surface.
- [x] Match XiaoBaOS package contracts: Skill requires `SKILL.md` and may carry
      support files; Role requires `role.json`, prompt and optional local Skills.
- [x] Persist Trace Farm output language and make all three evolution roles
      generate human-readable assets in the selected UI language.
- [x] Add direct asset deletion and place the global account control in the
      sidebar utility/account area across every route.
- [ ] Verify empty, running, failed and completed asset states at desktop and
      narrow widths in both themes.

## Completed milestone — evidence hierarchy

- [x] Derive Agent and Session identity in every Trace summary.
- [x] Group Trace navigation as Agent → Session → Trace → Span.
- [x] Give Session groups a deterministic first-request title without adding an
      LLM call or changing the authoritative exported Session identity.
- [x] Keep legacy evidence without Session metadata in an explicit ungrouped bucket.
- [x] Verify the hierarchy against retained Codex data and responsive layouts.

## Completed milestone — trustworthy memory workflow

- [x] Persist every submitted memory extraction as an owner-scoped task.
- [x] List recent tasks on the Memory page and refresh active tasks after
      navigation, reload or use on another signed-in browser.
- [x] Keep terminal task state and source Conversation provenance visible.
- [x] Augment empty semantic graphs with truthful Conversation, Agent and
      same-Conversation provenance instead of rendering an isolated Fact.
- [x] Make the sidebar account control visibly identifiable at narrow widths.

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
- [x] Evidence-linked `agent.md`, Skill package and Role package candidates.
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

## Active milestone — authoritative Coding Agent Runtime capture

- [x] Replace `claude-tap` proxy capture and the heuristic Python normalizer
      with pinned Langfuse-derived Codex rollout and Claude transcript parsers.
- [x] Route both live hooks and historical import through Canonical Event Graph
      v1 and Catena's deterministic OTLP exporter.
- [x] Support only real-accepted Codex CLI and Claude Code; remove Codex App,
      Hermes and OpenClaw claims.
- [x] Commit desensitized Runtime fixtures/goldens for the complete parser,
      failure, retry, abort, subagent, compact, resume and idempotency matrix.
- [x] Complete one real E2E for each Runtime through ClickHouse and Web Trace
      View, without deleting non-Catena evidence.
- [x] Render real Barena/XiaoBaOS evaluation evidence as Run, Turn, Model and
      Check steps while preserving Runtime wrappers in Raw Span.

## Completed milestone — readable Coding Agent Trace narrative

- [x] Import fresh Codex and Claude Code evidence through the new Runtime
      parsers into the local Catena stack and use those Traces as the UI
      acceptance baseline.
- [x] Make the default Trace detail read as one Turn narrative: user request,
      model attempts, exact Tool calls/results and final answer.
- [x] Render parallel Tools and Subagents as explicit branches, and surface
      Retry, Context Compact, Abort and Incomplete as first-class events.
- [x] Keep the complete Span waterfall and attributes under a separate
      diagnostics lens instead of making telemetry the primary reading model.
- [x] Verify desktop and narrow layouts against real new-parser Traces, then
      synchronize the production Web bundle embedded by Go.

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

- 2026-08-19: Barena ran a DSH Explore through the public Runtime adapter and
  uploaded one 26-Span boundary Trace to Catena. Catena froze four exact DSH
  Traces, Inspector identified a repeated premature-platform-assumption issue,
  Evolution produced the two-file `dsh-plugin-open-scope-first-response`
  package, and Reviewer returned `pass` at proposal scope. DeepSeek Harness
  `0.1.0-rc.7` then installed the generated package with lifecycle scripts
  disabled and exposed its `system-prompt/config.persona` patch through
  `--dump-config`. Go tests, all 52 Web tests and the production Web build
  passed.

- 2026-08-14: configured the official Codex plugin lifecycle locally with an
  owner-only Agent credential and persisted Hook trust through Codex's own
  configuration API. A real no-tool Codex Turn reached Catena as Trace
  `9a1fe9d219fdea97827a10fda487d2e2` (Turn + Model Call, both successful), and
  an identical repeated Hook produced no additional Trace or Span.

- 2026-08-14: Imported fresh, complete benign-at-source Codex and Claude Code
  executions through the replacement parsers. Catena retained trace
  `b500a6a56a920efe1702f4b468e3917e` for Codex and
  `2e83c300eff4332a3ae94b12da2617f0` for Claude, each as one Turn, two Model
  calls and one exact Tool with zero errors. The Web default is now a causal
  Turn narrative; raw OTel remains a diagnostics lens. Browser acceptance at
  desktop, a constrained 1200px workspace and 390px passed with zero overflow
  or console errors. Python checks, Codex TypeScript tests/build, 50 Web tests,
  Go tests/vet/race, both Compose configurations, Compose smoke and embedded
  Web parity all passed.

- 2026-08-11: Loopback local mode created a bound `Codex Local` Agent key
  without GitHub OAuth and backfilled 16 real Codex rollout Sessions through
  the authenticated OTLP endpoint. Catena retained 104 Turn Traces and 483
  Spans, including tool inputs/results, with exactly one root Span per Trace.
  Mock preview rows were removed; browser acceptance opened a real ten-Span
  Trace with nine tool calls. Go tests/vet, Web typecheck and all 45 Web tests
  passed.

- 2026-08-11: Trace navigation now preserves Agent → Session → Trace → Span.
  The public retained set exposed 1,601 Codex Session identities without a
  migration; browser acceptance expanded a 14-Trace / 416-Span Session,
  selected a three-Span Trace, and passed 842px plus 390px layouts with no
  horizontal overflow or console errors. Evidence without exported Session
  identity remains explicitly ungrouped.

- 2026-08-11: Rebuilt the React production bundle into Go's embedded Web
  assets and added a CI parity check between `catena-web/dist` and
  `control-plane/internal/control/web`. This prevents a direct Go build from
  serving a stale pre-Catena frontend when the normal multi-stage Docker build
  is bypassed.

- 2026-08-14: Deleted the `claude-tap` proxy/TraceStore/heuristic-normalizer
  path and replaced it with MIT Langfuse-derived parsers pinned at Codex
  `7500867afecf963d1cf83bf2b860a659591ace18` and Claude
  `5b3d4323c49f3839545fad36883ed02420ebc0ba`. Real redacted fixtures account
  183/183 source rows into 28 traces and 106 spans. Authenticated Codex CLI
  `0.147.0` and Claude Code `2.1.112` each completed Hook → OTLP → Go →
  ClickHouse → Web with four spans, zero errors, live/import identity equality
  and duplicate-hook no-op. Full evidence is recorded in
  `tap/ACCEPTANCE-2026-08-14.md`.

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
  Runtime/system context, malformed exporter payloads and secondary fields
  remain folded with raw evidence available on demand. 42 Web tests,
  typecheck/build, Go tests, `go vet` and the control-plane race suite passed.

- 2026-08-12: Completed the local-engine/cloud-observation boundary with a real
  Barena → XiaoBaOS → Catena run. Catena retained Trace
  `77ef5a0aba8aaed1c85bfcb146d56502` as one nine-Span hierarchy with two
  `barena.turn` branches, each parenting the corresponding `xiaoba.session` and
  `xiaoba.model.call`. The React Trace view classifies and renders that chain
  semantically; Catena does not execute the tested Agent. Final verification
  passed 46 Web tests, typecheck/build, embedded-bundle parity, Go tests and
  `go vet`.

- 2026-08-12: Replayed the same boundary after the semantic evidence repair.
  Trace `0c133a14cdd90f81d39c488b85f78aae` retained nine truthful Spans while
  the default product lens reduced them to seven useful steps: one Run, two
  Turns, two model calls and two ordered Checks. The requested model now appears
  in the Trace summary, Run/Check evidence is readable without raw JSON, and
  the first selected step contains the actual user/Agent exchange.
