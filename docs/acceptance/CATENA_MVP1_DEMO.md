# Catena MVP1 — retained end-to-end acceptance

Date: 2026-08-05

This is a real local product acceptance, not a fixture, benchmark score, or
claim that Catena verified an Agent change.

## Boundary under test

- Local Barena owned User simulation, target execution, inspection, review,
  evidence sealing, and the terminal Explore verdict.
- XiaoBaOS Base was the external target Agent.
- Catena accepted native OTLP plus the immutable Barena Run Bundle.
- Catena's embedded XiaoBaOS Evolution Runtime consumed the retained Evidence
  Pack. It did not execute or impersonate the target Agent.
- Every cloud output remained `draft/unverified`; local Barena remained the
  Release authority.

## Run

| Item | Retained value |
| --- | --- |
| Model endpoint | local OpenAI-compatible proxy, `gpt-5.5` |
| Behavior | ask for missing planning constraints before acting |
| Barena Run | `explore-20260805080046-28ad32` |
| Result | `fail`, task success `0.2`, evidence quality `0.9` |
| Native telemetry | 9 OTLP envelopes, 18 spans, complete |
| Root Trace | `d4644c38a3cb1d44e7f5ed482bef6f7c` |
| Primary target Trace | `331a58eea589a8329393fc0a043c2064` |
| Run Bundle | `run-bundle-88c896d2510ddc2ada300d7faaaaad09` |

The target recognized that calendar/task constraints were missing, but still
returned a generic morning/afternoon schedule. Barena retained the actual
conversation, XiaoBaOS model/tool spans, artifact facts, one Inspector issue,
and one Replay Case candidate.

## Evolution

Evolution Job: `evolution-job-1785917679761-e19874f1e3f5b362`

1. InspectorCat grounded the failure in the retained conversation, tool spans,
   and Barena terminal fact.
2. EvolutionCat proposed a Role constraint: when tasks, fixed appointments,
   priority, or available working time are missing, clarify before producing a
   schedule.
3. ReviewerCat returned `pass` only for proposal grounding and coherence. It
   explicitly stated that the artifact-only verifier was not sufficient for
   the desired first-turn semantic check.

Retained outputs:

- Case: “缺少日程约束时应先澄清而非生成泛化计划” — `draft/unverified`.
- Role: “日程规划前置澄清约束” — `draft/unverified`.
- Replay handoff: `catena.barena_replay_handoff.v1`, bound to the source Run,
  Trace, and Evidence Pack, with `creates_release=false` and
  `release_authority=local_barena`.

## Faults found by the real journey

The first attempt exposed two PostgreSQL-only faults that memory-store unit
tests did not reveal:

1. A NUL-separated advisory-lock key was invalid PostgreSQL text. The lock now
   uses the deterministic Run Bundle ID.
2. Evidence Pack SHA-256 changed after JSONB reordered nested object keys. The
   digest now uses a number-preserving canonical JSON representation and is
   checked again after durable storage.

The failed Evolution Job remains visible in history; Catena does not erase
failed attempts.

## Verification

- Barena: TypeScript build and 191 tests passed.
- Catena Go: full race suite and `go vet ./...` passed.
- PostgreSQL integration tests passed against an isolated local test database.
- React: 5 focused tests and production build passed.
- Compose: exactly four default services became healthy.
- Runner: `mode=evolution`; target Engine and Scenario routes returned 404.
- Browser: real Trace waterfall, three live role stages, candidates, Reviewer
  result, and Replay handoff passed with zero console errors or warnings.

## Honest MVP limitation

The handoff package is copyable and provenance-complete, but Barena does not
yet import it with one command. The next slice is direct handoff import followed
by a real local Replay whose verifier-backed terminal fact is synchronized to
Catena. Until that happens, no generated candidate is presented as verified or
released.
