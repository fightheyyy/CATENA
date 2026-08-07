# ADR 0003: Evolution output remains a proposal

- Status: accepted
- Date: 2026-08-02

## Context

Model-driven roles can analyze evidence and propose changes to `agent.md`, a Skill, a Role or a XiaoBaOS Harness. A review of that proposal does not prove the modified Agent still satisfies known capabilities.

## Decision

Every Candidate is evidence-linked and `draft/unverified`. Catena displays and exports it but does not mutate or publish the target automatically. A release claim requires independent Barena Replay and deterministic verifier evidence.

## Consequences

- The UI labels Candidates as proposals.
- Trace provenance remains attached to every generated asset.
- Applying or publishing an asset is an explicit future/user action.
- Only Barena release records may say `cleared`, `held` or `rejected`.
