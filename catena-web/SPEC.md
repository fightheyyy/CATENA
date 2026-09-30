# Catena Web Specification

Status: implemented MVP1 contract
Updated: 2026-09-16

## Responsibilities

The React client presents Overview, History, Memory and Outputs, with Agent
inspection, connections and settings as supporting routes. History includes
Trace and Conversation reading. It calls same-origin Go APIs only and contains
no authentication or workflow source of truth.

## Current Architecture

```mermaid
flowchart LR
    Routes["URL route · exact evidence identity"] --> Data["Journey-scoped cache<br/>cancel · refresh · retry"]
    Data --> API["Typed fetch client · bounded reads"]
    API --> Go["Catena Go API"]
    Routes --> Shell["Warm light/dark shell<br/>single heading · sidebar refresh · mobile bottom nav"]
    Shell --> Views["Overview · History · Memory · Outputs"]
    Data --> Overview["Recent history · compact outputs<br/>partial errors · search · exact links"]
    Views --> Overview
    Views --> History["Trace · Conversation tabs"]
    Views --> TraceIndex["Agent → Session → Trace index"]
    TraceIndex --> Narrative["Turn narrative<br/>request · final answer"]
    Narrative --> CausalSpine["causal spine<br/>Model · Tool · state events"]
    CausalSpine --> InlineEvidence["selected-step evidence<br/>input · output · exact call ID"]
    Narrative --> Diagnostics["raw Span waterfall<br/>attributes · timing"]
    Views --> Farm["Readable output library · analysis history"]
    Farm --> JobDetail["selected analysis · bounded polling"]
    JobDetail --> JobState["shared updated Job records"]
    Farm --> Background["active Job refresh in library"]
    Background --> JobState
    JobState --> EmbeddedAsset["fresh asset library · analysis history"]
    Views --> MemoryCards["Memory collection · full-text reader<br/>mixed-result search · reset · retry"]
    MemoryCards --> Graph["Optional relationship graph"]
    MemoryCards --> MemoryTask["Extraction tasks when present or failed<br/>step progress · retry · result"]
    MemoryTask --> API
    Routes --> Keys["Connections"]
    Keys --> API
    Keys --> Guide["POSIX · PowerShell configuration<br/>clipboard-only credential"]
    Guide --> FirstData["first-data check · retry · Agent handoff"]
    FirstData --> API
    Routes --> Settings["Language · Theme · Account"]
    Settings --> API
    Routes --> AccountMenu["Sidebar account area<br/>identity · switch · sign out"]
    AccountMenu --> API
```

## Target Architecture

```mermaid
flowchart LR
    Route["URL route · Agent · exact Trace"] --> RouteData["Journey-scoped loading<br/>cancel · cache · refresh · retry"]
    Route --> Shell["Quiet sidebar · four primary destinations<br/>single page heading · accessible refresh · no persistent slogans"]
    Shell --> Overview["Content-first overview<br/>searchable recent history · compact outputs<br/>no metric tiles or promotional cards"]
    RouteData --> Overview
    Overview --> EvidenceLinks["Exact Trace · Agent · analysis links"]
    Shell --> History["History tabs<br/>Trace · Conversation"]
    Shell --> MemoryCollection["Memory cards first<br/>search · source context · optional graph"]
    Shell --> Outputs["Readable asset library<br/>generation history as secondary view"]
    RouteData --> Views["Independent workspaces"]
    Views --> JobState["Shared updated Job records"]
    JobState --> FreshAssets["Progress → completed asset library"]
    Keys --> Guide["Shell config · clipboard-only key"]
    Guide --> FirstData["Bounded first-data polling<br/>retry · open Agent"]
    Conversation["Conversation index"] --> ConversationDetail["User-visible transcript detail"]
    AgentTrace["Agent selector"] --> Session["Session index"]
    Session --> SessionGroup["Session group header"]
    SessionGroup --> Trace["Inset Trace child list"]
    Trace --> TraceDetail["Turn narrative<br/>request → execution → outcome"]
    TraceDetail --> CausalSpine["causal spine<br/>Model · Tool · state events"]
    CausalSpine --> Branches["parallel Tool · Subagent branches"]
    TraceDetail --> Diagnostics["raw Span waterfall<br/>attributes · timing"]
    ConversationDetail --> Responsive["Desktop split view<br/>Narrow-screen master/detail"]
    ConversationDetail --> Submit["Distill to memory"]
    Submit --> Poll["poll owner-scoped task status"]
    Poll --> TaskCenter["Memory task center<br/>survives navigation · reload"]
    TaskCenter --> Progress["step progress · failure · retry · open result"]
    TraceDetail --> Responsive
    Stats["Agent statistics"] --> Evidence["Trace · Conversation · Error"]
    Keys["/api-keys"] --> Name["Agent name"]
    Name --> API["POST /v1/agents"]
    API --> Credential["Agent-bound API key"]
    Keys --> Credential
    Keys --> ModelConfig["Owner LLM config<br/>Provider · Base URL · Model · API Key"]
    ModelConfig --> Go["GET/PUT/DELETE /v1/me/llm-config"]
    Credential --> Ingest["OTLP · Conversation ingest"]
    Ingest --> Stats
    Settings["Settings"] --> Preferences["Language · Theme · account details"]
    Shell --> Sidebar["Product navigation · utilities"]
    Sidebar --> AccountMenu["labeled identity<br/>switch · sign out"]
    AccountMenu --> Go
    Outputs --> AssetLibrary["asset-first library<br/>Agent · kind · package"]
    AssetLibrary --> AssetDocument["package tree · readable files · copy · download"]
    AssetDocument --> DSH["DSH Plugin bundle<br/>package.json · Cordis patch"]
    AssetDocument --> Provenance["source analysis · Trace evidence"]
    Outputs --> AnalysisHistory["secondary analysis history"]
    AnalysisHistory --> JobProgress["Inspector · Evolution · Reviewer"]
```

The Agent page is observation-only. `/api-keys` owns Agent identity and
credential creation, reveal, copy and revocation. Runtime is never a form
field; the UI only displays the server's inferred result after evidence arrives.

## UX invariants

- Connections use a restrained, readable type scale: 14px labels, values and
  actions, 13px status text, and 18px section headings. URLs and masked keys
  use the UI font; monospaced type is reserved for executable configuration.
  Endpoint rows give values the available width and wrap on narrow screens
  without shrinking the type. No explanatory copy or credential behavior is
  added by this typography refinement.

- Routine screens put real content before explanatory copy. Each page has
  one main heading; avoid slogans, eyebrow subtitles, duplicate counts and
  generic instructions next to self-explanatory controls. Empty states retain
  one useful next step, errors retain recovery, and source/credential/delete
  information remains available where it affects a decision.

- A route requests only its own data. An unrelated API failure cannot block
  navigation or Settings; failed reads show retry without silently claiming
  that evidence is empty. Obsolete route reads are cancelled.
- The authenticated root opens Overview using existing Agent, Trace and Job
  endpoints. A failed Overview section shows a recoverable error while the
  other sections remain usable. Legacy Run, Issue, Case and Release APIs are
  not part of startup. Overview opens on recent evidence without metric tiles
  or promotional cards; no activity or analysis finding is invented.
- Agent and Trace deep links preserve the exact selection on reload and back.
  Opening one recent Trace must never silently select a different Trace.
- Evolution detail polling updates the shared Job record. Active analyses
  continue refreshing while the library is visible, so completed assets appear
  without reloading the browser. Polling is bounded and pauses in hidden tabs.
- Agent connection guidance offers POSIX and PowerShell configuration, with a
  placeholder in the preview. The actual credential is recovered only when
  copying. First-data checks stop on success, failure, timeout or unmount.

- The primary navigation is Overview → History → Memory → Outputs. Agents,
  connections and settings are utilities. Existing deep links stay valid;
  Trace and Conversation are tabs inside History.
- At mobile widths, four labeled destinations remain visible without
  horizontal scrolling, and utility/account actions remain discoverable.
- Warm neutral surfaces, one restrained accent, consistent icons and a compact
  heading scale define the shared visual system. Long technical explanations,
  raw attributes and process diagnostics are secondary disclosures.
- Memory opens on readable cards with search, clear empty/error states and
  a return-to-all action. Graph inspection is an explicit optional view. No
  hidden graph or extraction list should overwhelm the first screen.
- API management asks only for an Agent name when creating a credential.
- A generated key is presented as that Agent's credential, never as an
  independent settings object.
- Agent statistics never creates, reveals or revokes credentials.
- API management keeps one visible row per Agent with status, masked key and
  copy/revoke actions. Copy recovers the secret only for the clipboard and
  never renders a second plaintext credential card.
- Revoking the last credential stops future ingestion but does not delete the
  Agent or retained evidence. The row must say that ingestion is paused and
  history is retained; historical evidence must never be labeled connected.
- Evidence-dependent actions such as Trace Farm are disabled until the Agent
  has enough retained evidence.
- Agent lists prioritize registered product identities. Historical unbound
  telemetry aliases stay available in Trace, not as duplicate primary Agents.
- Agent ID, identity source and raw service aliases live under advanced details.
- Empty states explain the next action rather than exposing internal engine names.
- The empty Agent workspace offers one direct path to API Management for first-time connection.
- Trace detail prioritizes a human-readable Turn narrative: user request,
  model attempts, exact Tool calls/results and final answer. The Span
  waterfall remains complete but is a diagnostics lens, not the default view.
- Catena Canonical Event Graph spans use `catena.node.kind` as their semantic
  source of truth. `subagent`, `retry`, `context_compact` and
  `unmatched_tool_result` keep distinct presentation kinds. Failed Tools remain
  Tool evidence while carrying their error/incomplete state.
- Parallel Tools share one visible branch group. A Subagent opens a nested
  thread with its own Turn, Model and Tool chain while preserving parentage.
- Retry, Context Compact, Abort and Incomplete are visible narrative events;
  none may be reduced to a generic Model row or a successful trace badge.
- Model steps show model, token and timing facts by default. Full request
  history is secondary evidence so repeated context does not drown the causal
  delta between model attempts.
- Cross-process Barena/XiaoBaOS traces are semantic product evidence, not raw
  telemetry noise: `barena.simulation`, `barena.turn`, `xiaoba.model.call` and
  `barena.assertion` render as Run, Turn, Model and Check steps. Runtime wrapper
  spans such as `xiaoba.session` retain their true parentage in Raw Span while
  staying folded in the default chain.
- The default selected step is the first evidence-bearing Turn, not an empty
  model or Runtime wrapper Span. Run and Check details render test facts rather
  than generic input/output envelopes.
- Trace evidence renders semantic payloads rather than transport envelopes:
  `chat_messages` input becomes visible role/content message cards, while its
  `type` and `value` wrapper remains available only under raw data.
- Trace navigation preserves the evidence hierarchy: Agent → Session → Trace →
  Span. Session groups use exported identity only; missing identity appears in
  one explicit ungrouped bucket rather than a fabricated session.
- A Session's scan label is derived locally from its earliest retained valid
  user request. It uses no model call; the exported Session ID remains visible
  as evidence identity and is the fallback when no request text is available.
- Session is rendered as a group container, not as a sibling of Trace. Its
  expanded state is communicated by the group surface and disclosure; the
  selected Trace alone receives the accent selection treatment. Trace root
  names are primary scan labels while opaque Trace IDs remain secondary.
- Each Trace row is one execution from a user request to a terminal answer.
  Its primary title comes from the request evidence; protocol labels such as
  `agent.turn` and Runtime-specific fallback names remain secondary metadata.
- Conversation and Trace use the same master/detail hierarchy: the index selects
  a record, while the detail owns a distinct header, summary and evidence surface.
- At narrow widths, index and detail are separate states with an explicit back
  action; they must never be stacked into one continuous document.
- At 390px, every primary journey keeps `scrollWidth === innerWidth`; long
  Conversation titles wrap inside the detail header instead of widening the page.
- Conversation messages use visibly distinct user and Agent transcript cards.
- Memory distillation never ends at an ambiguous “submitted” label. The
  Conversation detail shows current step and percentage, explains terminal
  failure, permits retry, and links completed work to Memory. The Memory page
  also lists recent tasks so status remains visible after navigation or reload.
- An isolated Fact is not presented as a meaningful graph. When GauzMem has no
  semantic edge, the graph shows only provenance-backed Conversation, Agent or
  same-Conversation context, and labels that context distinctly from semantic
  relations.
- Trace metrics stay compact; narrative steps preserve a stable reading
  position while selected raw Span evidence remains one diagnostic inspection
  unit.
- Trace Farm starts from an Agent and bounded time window, never one isolated Trace.
- Starting an analysis sends the current UI language as an explicit asset-output
  language; Chinese UI produces Chinese assets and English UI produces English assets.
- Trace Farm opens on accumulated Agent assets, not analysis jobs. A completed
  Job is implementation provenance for an asset, not the primary product object.
- Every asset exposes its stable package root, file tree, readable selected
  file, Agent, kind, source Trace count, copy/download, direct deletion and
  source analysis.
- DSH Agents display `DeepSeek Harness`, and `dsh_plugin` appears as a distinct
  Asset Library kind with a whole-package download suitable for local Barena
  acceptance.
- Analysis history, role progress, raw outputs and deletion remain available as
  a secondary audit view without competing with the Asset Library.
- Completed and failed analyses and their generated assets expose a destructive
  two-step delete action in their own context.
  The confirmation states that generated assets are removed while source Trace
  evidence remains; queued and running analyses expose no delete action.
- Candidate content is copyable and clearly labeled as a proposal.
- Chinese and English share the same information architecture.
- API management clearly separates Agent ingestion credentials from the
  owner-provided LLM configuration used by Trace Farm.
- The saved model API Key is never rendered or returned; an empty key while
  editing preserves the current credential.
- Language and theme appear only in Settings, not in the global navigation.
- The current identity lives in the sidebar's utility/account area alongside
  API Management and Settings. It stays visibly labeled at desktop and compact
  widths; switching account and signing out never require opening Settings.
