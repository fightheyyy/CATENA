# Catena Implementation Plan

Updated: 2026-08-07

## Current status

Catena is a standalone Go + React product. Go owns OAuth, Agent registration,
Agent-bound API keys, OTLP/Conversation ingestion, Trace queries and Evolution
Job orchestration. The embedded XiaoBaOS Runtime consumes retained Evidence
Packs and produces reviewable Agent assets. Target Agent execution remains
local.

## Active milestone — owner-provided Evolution model

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
