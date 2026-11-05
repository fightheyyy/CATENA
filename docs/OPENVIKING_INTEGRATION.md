# OpenViking retired — preserve and export first

Operation Rebirth removes the Engine adapter, Go Memory API, UI, embedding service
and Compose overlay. No replacement Memory provider is introduced. Historical
research documents describe prior behavior, not current deployment instructions.

Before upgrading, stop workers and back up PostgreSQL, ClickHouse and Engine
receipt/run volumes. Export `catena_memory_tasks` and the `pipeline=memory` Job
rows (including document/checkpoint/artifacts) using `pg_dump` or an owner-scoped
administrative export. These may contain private evidence: restrict access to the
same owners and store exports securely. Do not post provider credentials in exports.

Keep the existing OpenViking bind directory (`.local/openviking` by default), its
`data/` and configuration separately; preserve the old image revision and prior
Compose definition from Git if a provider-native export is required. Back up that
directory while the provider is stopped. The new deployment neither mounts nor
deletes it. Do not run `docker compose down -v` or delete any volumes as migration.

Migration 003 marks only queued/running Memory jobs retired and releases leases;
terminal Job documents, checkpoints, artifacts and legacy tables remain intact.
A running old worker must be stopped before migration. Rollback requires restoring
the old application version; retired jobs must not be blindly replayed because an
external extraction may already have incurred costs.

Future context input is limited to explicitly authorized Markdown/AGENTS.md text.
No retrieval, embeddings, knowledge graph or long-term memory service is implemented.
