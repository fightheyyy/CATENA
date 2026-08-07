# Catena Control Plane Plan

Updated: 2026-08-07

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

## Active milestone — Agent-bound ingestion

- [x] Add owner-scoped Registered Agent persistence.
- [x] Bind every newly created credential to one stable `agent_id`.
- [x] Override Conversation and OTLP Agent identity from the credential.
- [x] Infer Runtime from accepted evidence without a user-selectable field.
- [x] Merge registered Agents with Trace and Conversation counts.
- [x] Keep legacy unbound credentials compatible but hidden from onboarding.
- [x] Expose cheap Registered Agent connection polling without a ClickHouse scan.

## Active milestone — owner-provided Evolution model

- [x] Persist one encrypted Provider/Base URL/Model/API Key config per owner.
- [x] Expose safe authenticated GET/PUT/DELETE APIs without secret recovery.
- [x] Decrypt only while dispatching the owner's Evolution Job.
- [x] Pass model values per request to the private Runner without global env mutation.
- [x] Remove deployment-managed model defaults and fail clearly when unconfigured.
- [x] Prove cross-owner isolation and secret non-disclosure.

## Verification log

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

## Active milestone — durable worker queue

- [ ] Add atomic `queued → leased → running → terminal` transitions.
- [ ] Persist lease owner, expiry and heartbeat.
- [ ] Recover expired jobs with bounded retry and attempt history.
- [ ] Make cancellation observable and idempotent.
- [ ] Prevent duplicate terminal events and stale-event overwrite.
- [ ] Export queue depth, lease age, job duration and retry count.
- [ ] Verify with concurrent workers and process-kill fault injection.

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
