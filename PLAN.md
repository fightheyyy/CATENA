# Catena Implementation Plan

Updated: 2026-08-03

## Current Status

Catena MVP1 is a demoable local vertical slice. The platform can ingest OTLP,
discover Agents from Trace data, run HTTP Agent Explore, retain an Issue,
promote an immutable Regression Case, execute Barena Replay, and persist an
Evaluation and Release Gate. GitHub OAuth, project API keys, English/Chinese
UI, and a restricted four-role XiaoBaOS evaluator/evolution Runtime are
implemented.

## Milestones

### MVP1 — local evolution loop

- [x] LangWatch-derived Catena Web and Trace subsystem.
- [x] GitHub login, projects, API keys, and project isolation.
- [x] OTLP ingestion and Codex bounded history backfill.
- [x] Trace-derived Agent Registry.
- [x] Scenario Explore through a registered HTTP Agent.
- [x] Go Run/Event/Issue/Case/Evaluation/Release state machines.
- [x] InspectorCat → EvolutionCat → ReviewerCat Evolution Jobs.
- [x] Barena Replay and factual Compare integration.
- [x] Six-container Compose topology and smoke test.
- [x] Self-contained repository build with pinned Barena and XiaoBaOS sources.

### MVP1 release hardening

- [ ] Run and retain one non-fixture XiaoBaOS Explore/Evolution/Replay journey
  against the configured model endpoint.
- [ ] Complete a fresh-clone Compose acceptance on a clean machine.
- [ ] Remove remaining internal `spiral-*` compatibility identifiers after
  protocol consumers migrate.
- [ ] Hide or remove unused upstream routes from the Catena product surface.
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

## Next Steps

1. Verify the public repository from a fresh clone.
2. Run the real XiaoBaOS model-backed acceptance and retain its Trace/Release
   identifiers.
3. Freeze MVP1 UI navigation and demo data.
4. Tag the first Catena release.

## Owners

| Area | Owner |
| --- | --- |
| Web, Trace, auth, Scenario | Catena downstream |
| Workflow, tenancy, durable records | Go control plane |
| Evaluation and release semantics | Barena |
| Evaluation/evolution role execution | XiaoBaOS Runtime |
| Target behavior and native telemetry | External Agent Runtime |

## Acceptance Criteria

- Exactly six Compose services become healthy.
- Missing project keys are rejected at Run and OTLP ingress.
- One Trace can retain an Issue, immutable Case, Replay Evaluation, and Release
  Gate without fabricated facts.
- Evolution outputs remain proposals and cannot directly create a Release.
- Cross-project requests fail closed.
- TypeScript tests, Go race tests, focused Web tests, SDK backfill tests, and
  fresh-clone Compose validation pass.

## Verification Log

- 2026-08-03: Barena TypeScript suite passed 186/186.
- 2026-08-03: Go control-plane `go test -race ./...` passed.
- 2026-08-03: focused Catena product/navigation/settings/ingress/evaluator
  tests passed 29/29.
- 2026-08-03: Codex installer and history-backfill tests passed 55/55.
- 2026-08-03: the fresh Catena Compose build started exactly six healthy
  services and passed auth, protected-ingress, storage, and Runner smoke checks.
- 2026-08-03: running MVP retained 4,894 unique Traces and 6,918 spans across
  Codex, Claude Code, XiaoBaOS, Scenario, and Barena services.

## Risks / Open Questions

- The current Runner is one local service and does not recover an in-flight
  role turn after host loss.
- Active browser Explore requires a reachable HTTP Agent; OTLP alone provides
  observation, not control.
- The LangWatch downstream remains large, so upstream sync and feature-surface
  discipline are required.
- Public cloud operation requires a separate security and backup acceptance.

## Status Maintenance Rules

Mark an item complete only when code and reproducible evidence exist. Update
both this plan and [SPEC.md](./SPEC.md) whenever ownership, trust boundaries,
or persistent contracts change.
