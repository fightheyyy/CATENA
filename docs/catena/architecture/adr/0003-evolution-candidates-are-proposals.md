# ADR 0003: Evolution output is a proposal until Barena verifies it

- Status: accepted
- Date: 2026-08-02

## Context

XiaoBaOS roles can analyze a Trace and propose changes to a Role, Skill,
Memory, or Harness. A language-model review is useful evidence, but it cannot
prove that applying the change preserves known capabilities or is safe to
release.

## Decision

Evolution jobs persist a Finding, replay Case proposal, Candidate, and Review.
Every Candidate is `draft/unverified`. MVP1 requires explicit human promotion
of evidence into an immutable Case, followed by Barena Replay and Release
Check. Only the Barena release record may say `cleared`, `held`, or `rejected`.

## Consequences

- The UI never presents role output as an applied optimization.
- No target source tree is changed and no Hub publication occurs in MVP1.
- Generic cleared Role/Skill assets may later be published through RoleHub or
  SkillHub adapters; XiaoBaOS-specific Harness changes return to XiaoBaOS.
- Candidate application and before/after Compare need a separate, explicit
  future contract.
