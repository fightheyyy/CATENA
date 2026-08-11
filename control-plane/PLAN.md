# Catena Control Plane Plan

Updated: 2026-08-11

## Current state

- [x] Go owns public auth, Registered Agents, bound credentials and project isolation.
- [x] Go ingests OTLP and queries Catena-owned ClickHouse tables.
- [x] Go stores user-visible Conversations and immutable Barena Run Bundles.
- [x] Go resolves canonical Agent identities while preserving source names.
- [x] Evolution starts from an immutable multi-Trace Agent window.
- [x] Inspector, Evolution and Reviewer stages are persisted and streamed.
- [x] Candidate assets retain exact evidence provenance.
- [x] React is built into and served by the Go binary.
- [x] GauzMem is private and tenant scope is derived by Go.

## Completed milestone — observable memory extraction

- [x] Add an owner-scoped GauzMem task-status projection.
- [x] Preserve upstream step progress while hiding private project identity.
- [x] Return expired or missing tasks explicitly instead of implying success.
- [x] Run GauzMem against Qdrant Server to remove local-client lock failures.

## Completed milestone — durable memory workflow

- [x] Persist the receipt and latest status of each memory extraction.
- [x] Add an owner-scoped recent-task list and refresh records during polling.
- [x] Preserve source Conversation and Agent provenance in every task record.
- [x] Add provenance and same-Conversation graph edges when semantic extraction returns
      an isolated Fact, without inventing semantic claims.

## Completed milestone — Agent-bound ingestion

- [x] Add owner-scoped Registered Agent persistence.
- [x] Bind every newly created credential to one stable `agent_id`.
- [x] Override Conversation and OTLP Agent identity from the credential.
- [x] Infer Runtime from accepted evidence without a user-selectable field.
- [x] Merge registered Agents with Trace and Conversation counts.
- [x] Keep legacy unbound credentials compatible but hidden from onboarding.
- [x] Expose cheap Registered Agent connection polling without a ClickHouse scan.

## Completed milestone — owner-provided Evolution model

- [x] Persist one encrypted Provider/Base URL/Model/API Key config per owner.
- [x] Expose safe authenticated GET/PUT/DELETE APIs without secret recovery.
- [x] Decrypt only while dispatching the owner's Evolution Job.
- [x] Pass model values per request to the private Runner without global env mutation.
- [x] Remove deployment-managed model defaults and fail clearly when unconfigured.
- [x] Prove cross-owner isolation and secret non-disclosure.

## Verification log

- 2026-08-11: The embedded React assets are now generated from the current
  Catena Web build, and CI compares them byte-for-byte after every production
  Web build so a standalone Go binary cannot regress to stale product UI.

- 2026-08-11: MVP1 release-candidate Go tests, `go vet` and race tests passed;
  local and public Compose configurations rendered successfully.

- 2026-08-10: Added the PostgreSQL `catena_memory_tasks` ledger, owner-scoped
  task list, poll-to-record refresh and terminal-state fallback. Unit tests
  cover persisted source identity, progress and provenance graph augmentation;
  Go tests, `go vet` and race tests passed. Local acceptance proved the task ledger survives
  a Core restart.

- 2026-08-09: Production OAuth diagnosis proved the cloud resolver-selected
  GitHub edge timed out before TLS while an alternate official edge completed
  both the token and identity endpoints. The public Compose overlay now keeps
  configurable GitHub host mappings without weakening TLS, state or PKCE.

- 2026-08-09: Added terminal Evolution Job deletion to the memory and
  PostgreSQL stores plus the owner-scoped HTTP API. Tests cover cross-owner
  denial, non-terminal conflict, terminal removal and persisted deletion.

- 2026-08-07: LLM configuration lifecycle and two-owner isolation tests passed;
  persisted secrets are AES-GCM envelopes and GET responses expose only
  `api_key_configured`.
- 2026-08-07: Go tests and vet passed. Local PostgreSQL acceptance confirmed
  the API key is not stored as plaintext, and public deployment created the
  owner-scoped configuration table without shared model environment variables.

- 2026-08-07: Former deployment-model visibility and Base URL sanitization
  passed; this design is superseded by owner-provided LLM configuration.
- 2026-08-07: Public Core deployment remained healthy with the existing
  DashScope-compatible model configuration and a tagged rollback image.

## Next milestone — durable worker queue

- [ ] Add atomic `queued → leased → running → terminal` transitions.
- [ ] Persist lease owner, expiry and heartbeat.
- [ ] Recover expired jobs with bounded retry and attempt history.
- [ ] Make cancellation observable and idempotent.
- [ ] Prevent duplicate terminal events and stale-event overwrite.
- [ ] Export queue depth, lease age, job duration and retry count.
- [ ] Verify with concurrent workers and process-kill fault injection.

## Completed milestone — Catena-owned Trace store

- [x] Point the control plane exclusively at `catena.catena_spans`.
- [x] Remove retired platform attribute aliases and source-kind values.
- [x] Verify standard OTLP input/output normalization and ClickHouse round trips.
- [x] Verify the migrated public table has the same row count before deleting the legacy database.

## Acceptance

1. A worker crash releases work after lease expiry without duplicate terminal output.
2. Cancellation races cannot overwrite a completed or failed job.
3. Retry count is bounded and visible.
4. Full tests, `go vet` and race tests pass against PostgreSQL.

## Deferred

- Multi-region scheduling.
- Kubernetes deployment.
- Cross-organization sharing.
- Automatic mutation or publication of Candidate assets.

## Verification log

- 2026-08-11: Synchronized the semantic Trace evidence frontend into the Go
  embedded asset tree. Asset parity, all Go tests, `go vet` and
  `go test -race ./internal/control` passed.
