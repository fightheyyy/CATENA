# Catena MVP1 acceptance

Status: accepted for single-node Beta
Updated: 2026-08-07

## Demonstrated journey

1. GitHub OAuth creates an authenticated Catena session.
2. A personal API Key accepts OTLP/HTTP from Codex and XiaoBaOS.
3. Source aliases are grouped into one canonical Agent while raw `service.name` remains visible.
4. Trace detail renders Span hierarchy, duration, tool input/output and error evidence.
5. XiaoBaOS sends user-visible Conversation records through the separate Conversation API.
6. An Agent plus bounded time window freezes a multi-Trace Evidence Pack.
7. Inspector, Evolution and Reviewer stages complete and retain their output.
8. The resulting `agent.md`, Skill, Role or XiaoBaOS Harness Candidate remains linked to every source Trace.
9. Conversation-derived memory compiles into semantic, graph and temporal recall through the optional private memory service.

## Verified gates

- React tests, typecheck and production build pass.
- Go unit/integration tests, vet and race tests pass.
- Local and public Compose configurations validate.
- Cross-owner API reads fail closed.
- OTLP and Conversation retries are idempotent.
- Evolution does not execute the target Agent or create a release verdict.

## Known limits

- Single-node deployment only.
- In-flight Runner work is not lease-recovered after host loss.
- Backups, quotas and full team RBAC are not yet productized.
