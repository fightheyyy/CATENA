# ADR 0001: Catena is the platform; Barena remains the release engine

- Status: accepted
- Date: 2026-08-02

## Context

The product now combines a LangWatch-derived observability application, a Go
continuous-evolution control plane, Barena's TypeScript evaluation engine, and
a restricted XiaoBaOS evolution runtime. Calling every layer “Barena” obscures
the product boundary, while replacing the Barena name would discard the
existing Agent E2E and release semantics.

## Decision

The multi-user product is named **Catena**. It is the Agent continuous-evolution
station. **Barena** remains the embedded Agent E2E evaluation and release
engine. LangWatch remains an attributed infrastructure substrate inside
`catena-app`; it is not presented as original Catena technology.

## Consequences

- Product navigation and deployment services use the Catena name.
- Evaluation commands, Case/Run Package formats, Replay, Compare, verifier,
  and Release Check retain the Barena name.
- The two repositories remain separate so LangWatch upstream can be merged.
- A future memory subsystem can join Catena without being mislabeled as part
  of Barena's release engine.
