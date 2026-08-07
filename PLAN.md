# Catena Implementation Plan

Updated: 2026-08-07

## Current status

Catena is a standalone Go + React product. Go owns OAuth, API keys, OTLP/Conversation ingestion, Trace queries, Agent identity and Evolution Job orchestration. The embedded XiaoBaOS Runtime consumes retained Evidence Packs and produces reviewable Agent assets. Target Agent execution remains local.

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
