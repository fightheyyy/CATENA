# Catena Web Specification

Status: implemented MVP1 contract
Updated: 2026-08-07

## Responsibilities

The React client presents five primary journeys: Agent, Conversation, Memory, Trace and Trace Farm. It calls same-origin Go APIs only and contains no authentication or workflow source of truth.

## Current Architecture

```mermaid
flowchart LR
    Routes["React routes"] --> API["Typed fetch client"]
    API --> Go["Catena Go API"]
    Routes --> Views["Agent · Conversation · Memory · Trace · Farm"]
```

## Target Architecture

```mermaid
flowchart LR
    AgentPage["Agent page"] --> Form["Agent name"]
    Form --> API["POST /v1/agents"]
    API --> Credential["agent_id + recoverable Agent key"]
    Credential --> Status["Runtime · connection · Conversation · Trace"]
```

The Agent page is the only connection entry. Settings no longer creates generic
API keys. Runtime is never a form field; the UI only displays the server's
inferred result after evidence arrives.

## UX invariants

- The primary navigation is Agent → Conversation → Memory → Trace → Trace Farm.
- First-run onboarding asks only for an Agent name and labels the action
  `接入新 Agent` / `Connect new Agent`.
- A generated key is presented as that Agent's credential, never as an
  independent settings object.
- Empty states explain the next action rather than exposing internal engine names.
- Trace detail prioritizes Span waterfall, tool calls, input/output and errors.
- Trace Farm starts from an Agent and bounded time window, never one isolated Trace.
- Candidate content is copyable and clearly labeled as a proposal.
- Chinese and English share the same information architecture.
