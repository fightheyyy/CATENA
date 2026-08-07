# Catena architecture

Catena is the cloud evidence and evolution platform; Barena is the local Agent E2E and release engine.

```mermaid
flowchart LR
    GitHub["GitHub OAuth"] --> Core

    subgraph Edge["User environment / CI"]
        Agent["Target Agent"]
        XiaoBa["XiaoBaOS<br/>anthropomorphic Agent"]
        Barena["Barena"]
        Trace["OTLP Trace · Artifact"]
        Conversation["User-visible Conversation"]
        Barena <--> Agent
        Barena <--> XiaoBa
        Agent --> Trace
        XiaoBa --> Trace
        XiaoBa --> Conversation
        Barena --> Trace
    end

    subgraph Cloud["Catena"]
        Web["React Web"] --> Core["Go control plane"]
        Core --> PG[("PostgreSQL")]
        Core --> CH[("ClickHouse")]
        Core --> Runner["XiaoBaOS Evolution Runtime"]
        Runner --> Assets["agent.md · Skill · Role · Harness"]
        Core -->|"Conversation"| GauzMem["GauzMem"]
        GauzMem --> Memory["semantic · graph · temporal memory"]
    end

    Trace --> Core
    Conversation --> Core
```

## Boundaries

| Component | Owns | Does not own |
| --- | --- | --- |
| React Web | interaction and visualization | auth truth, job state, database access |
| Go control plane | auth, API keys, OTLP, state, lineage and audit | target execution, model reasoning |
| XiaoBaOS Evolution Runtime | evidence analysis and Candidate generation | target Agent execution, release authority |
| Local Barena | Explore, Replay, Compare, Verifier and Release Check | cloud tenancy and long-term Trace history |
| GauzMem | memory compilation and recall | Agent identity and release decisions |

## Evidence loop

1. An external Agent exports OTLP and, for XiaoBaOS, user-visible Conversations.
2. Catena groups sources into a canonical Agent without rewriting raw identity.
3. A user selects an Agent and bounded time window.
4. Catena freezes the matching Trace IDs into an Evidence Pack.
5. Inspector, Evolution and Reviewer produce a provenance-linked Candidate.
6. The user may apply it and use local Barena Replay to verify behavior.

XiaoBaOS is designed as a persistent, anthropomorphic coworker. Its memory path therefore consumes only user-visible messages and successfully delivered replies. Internal prompts, reasoning and Tool activity remain Trace evidence for engineering analysis instead of being written into the user's remembered history.

Evolution output remains a proposal. Catena does not apply it automatically or turn a Reviewer opinion into a release decision.

See [ADR 0003](./adr/0003-evolution-candidates-are-proposals.md).
