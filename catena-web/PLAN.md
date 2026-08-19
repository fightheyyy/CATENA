# Catena Web Plan

Updated: 2026-08-19

## Current state

- [x] Standalone React/Vite application served by Go.
- [x] Agent, Conversation, Memory, Trace and Trace Farm journeys.
- [x] GitHub identity, Agent-bound credential management and bilingual UI.
- [x] Agent Trace-window selection and role-stage progress.
- [x] Turn narrative, diagnostic Span waterfall and memory graph rendering.

## Active milestone — DSH asset experience

- [x] Render Runtime `dsh` as DeepSeek Harness in Agent surfaces.
- [x] Normalize `dsh_plugin` as a usable two-file Agent asset.
- [x] Add DSH Plugin filtering, bilingual labels, readable YAML and one-click
      whole-package download.
- [x] Verify existing agent.md/Skill/Role assets and historical kinds remain
      readable.

## Completed milestone — readable Coding Agent traces

- [x] Replace the default semantic Span waterfall with a Turn narrative built
      from canonical node kind, state, parent ID and accurate timestamps.
- [x] Render parallel Tool groups, nested Subagent threads, Retry, Compact,
      Abort and Incomplete without losing strict Tool call evidence.
- [x] Move raw Span timing and attributes into a diagnostics lens and keep
      model history collapsed behind step evidence.
- [x] Validate the new presentation against freshly imported Codex and Claude
      Runtime traces at desktop and narrow widths.

- [x] Replace the analysis-first Trace Farm overview with an accumulated Agent
      Asset Library and keep analysis history as a secondary view.
- [x] Render generated assets as readable files with kind/Agent filters,
      provenance, copy and download actions.
- [x] Render Skill and Role as package trees rather than fictional single files.
- [x] Bind generated asset language to the selected Catena interface language.
- [x] Expose direct two-step deletion from the selected asset document.
- [x] Keep the global account menu in the sidebar utility/account area so
      identity and navigation share one predictable shell.
- [x] Separate durable Agent identity from revocable ingestion credentials;
      a revoked key preserves history without claiming the Agent is connected.
- [x] Title every Trace row from its user request evidence instead of exposing
      Runtime fallback labels such as “Codex user turn”.
- [x] Derive each Session's scan title from its earliest retained user request
      without an LLM, while keeping Session ID visible as secondary identity.

- [x] Replace Settings API-key management with Agent-first onboarding.
- [x] Render registered-but-not-yet-connected Agents and inferred Runtime.
- [x] Move Agent credential create/reveal/copy/revoke actions to `/api-keys`.
- [x] Add guided first-run connection: name, copy-ready config, automatic
      evidence detection and success handoff.
- [x] Remove the onboarding route and keep Agent pages observation-only.
- [x] Move raw IDs and service aliases into advanced details.
- [x] Hide evidence-only aliases from the registered Agent list.
- [x] Add one-click copy actions beside the OTLP and Conversation endpoints.
- [x] Replace Trace Farm's implicit latest-job selection with an explicit analysis overview.
- [x] Added former deployment model visibility to Settings (superseded).
- [x] Move editable owner LLM configuration to API management.
- [x] Move language and theme controls exclusively to Settings.
- [x] Collapse each Agent credential into one row; copy recovers directly to
      the clipboard without rendering a duplicate plaintext key card.
- [x] Separate Conversation and Trace indices from their detail workspaces with
      a shared responsive master/detail interaction.
- [x] Give Conversation messages and selected Trace Span evidence explicit
      visual hierarchy without changing stored evidence or API contracts.
- [x] Replace the Conversation “submitted” dead end with step progress,
      terminal failure/retry, and a completed link to Memory.
- [x] Add a sidebar GitHub account area with direct account switch, account
      settings and sign-out actions.
- [x] Keep the account menu labeled at narrow widths instead of exposing an
      unexplained avatar-only control.
- [x] Add a Memory task center backed by server-listed extraction tasks.
- [x] Distinguish GauzMem semantic edges from Catena provenance edges.
- [x] Replace the flat Trace index with Agent → Session → Trace → Span navigation.
- [x] Render missing Session identity as an explicit ungrouped bucket.
- [x] Unwrap Runtime `chat_messages` envelopes into visible message cards;
      keep `type` and `value` only in the raw-data disclosure.
- [x] Make Session a visually explicit group container and Trace an inset,
      compact child list with an independent selected state.
- [x] Render the Barena/XiaoBaOS chain as Run → Turn → Model → Check;
      fold session wrappers and select the first evidence-bearing Turn by default.
- [x] Render Canonical Event Graph Tool, Retry and Subagent kinds without
      losing failed tools from the Tool lens.
- [ ] Add responsive browser acceptance to CI.
- [ ] Improve accessibility and keyboard navigation.
- [ ] Add durable optimistic/retry states for long-running jobs.

## Acceptance

`pnpm test`, `pnpm typecheck` and `pnpm build` pass; desktop and mobile journeys expose no console errors.

Conversation and Trace details must be visually distinct from their indices at
desktop widths. At 720px and below, opening a record must replace the index with
the detail and expose a working back action without horizontal overflow.

## Verification log

- 2026-08-19: the DSH Agent and generated Plugin render as first-class Runtime
  and Asset types. The Asset Library exposes both files, exact provenance and
  a complete tar download while filtering unsafe or Runtime-mismatched legacy
  records. All 52 Web tests and the production Vite build passed.

- 2026-08-15: Session headers now use the earliest retained valid user request
  as a deterministic title with no LLM call; the exported Session ID remains
  visible beside the Agent name and stays authoritative. Local retained Codex
  evidence rendered `还是没懂。整个算法的逻辑` for Session
  `019fefa2…913529`; browser acceptance reported no console warnings or errors.
  All 52 Web tests, TypeScript typecheck and production build passed.

- 2026-08-15: Agent connection rows now distinguish durable Agent identity from
  revocable ingestion credentials: revoked keys render `ingest paused` while
  retained history remains explicit. Trace summaries derive their scan title
  from existing Turn input evidence, so retained Codex rows display the real
  user request without re-upload. The global identity control moved into the
  sidebar utility/account area. Browser acceptance verified all three changes
  against local retained data with no console warnings or errors; all 51 Web
  tests, TypeScript typecheck, production build, Go tests and `go vet` passed.

- 2026-08-14: Fresh, complete Codex CLI `0.147.0` and Claude Code `2.1.112`
  runs were imported through the new Runtime parsers without post-capture
  content rewriting. The default view now reads request → Model → exact Tool
  call/result → Model → final answer, with Token, duration and state evidence.
  Desktop, constrained 1200px and 390px browser acceptance passed with no
  horizontal overflow and zero console errors or warnings. All 50 Web tests,
  TypeScript typecheck, production build and embedded-bundle parity passed.

- 2026-08-14: Both real Runtime traces were opened in the Web Trace View.
  Codex and Claude each rendered one Turn, two Model calls, one exact Tool,
  four total spans and zero errors; expanding each Tool displayed its real
  command argument and result. Golden-driven Web coverage increased to 48
  tests, and typecheck plus production build passed.

- 2026-08-11: Trace navigation now renders Session as a bordered group surface
  with an inset Trace child list. Trace root names lead scanning while opaque
  IDs remain utility labels; expanded Session and selected Trace use separate
  states. Browser acceptance at 700px verified disclosure collapse/expand,
  no horizontal overflow and no console errors.

- 2026-08-11: Codex Turn input now unwraps the `chat_messages` transport
  envelope into role/content message cards. Public browser acceptance replayed
  the reported Trace and verified that the input visibly contains `用户` and
  the actual message, while `type`/`value` remain only under raw data. The page
  had no horizontal overflow or console errors; all 45 Web tests, typecheck,
  production build, Go tests, `go vet` and the race suite passed.

- 2026-08-11: The Trace workspace groups retained evidence by Agent and
  exported Session before exposing Trace rows and Span detail. Public browser
  acceptance verified a 14-Trace / 416-Span Session, the four-level breadcrumb,
  mobile detail/back behavior, zero horizontal overflow and zero console
  errors. All 44 Web tests, typecheck and the production build passed.

- 2026-08-11: Synchronized the production Vite bundle with Go's embedded
  static assets and added CI enforcement that rejects stale embedded Web
  output.

- 2026-08-11: MVP1 release-candidate verification passed all 39 Web tests,
  TypeScript typecheck and the production Vite build.

- 2026-08-10: The Memory page gained a durable recent-task center and visibly
  distinct provenance graph edges. Browser acceptance verified a real failed
  extraction remains visible after leaving Conversation, refreshing and
  restarting Core. The account control remains labeled at the 842px audit
  viewport. All 39 Web tests, typecheck and production build passed.

- 2026-08-09: The authenticated shell gained a responsive account menu that
  displays the GitHub identity and exposes account switch/sign-out without a
  Settings detour. All 38 Web tests, typecheck and production build passed;
  browser acceptance verified GitHub avatar rendering, direct Settings entry,
  Escape dismissal and zero horizontal overflow at 390px.

- 2026-08-09: Route workspaces and the XiaoBaOS memory graph now use lazy
  chunks. The production entry bundle fell from 157 KB to 74 KB gzip, keeping
  the low-bandwidth cold start independent from the 58 KB graph chunk.

- 2026-08-09: Release UX audit covered Home, Agent, Conversation, Memory,
  Trace, Trace Farm, API Management and Settings at desktop and 390px. The
  responsive header was rebuilt as two fixed rows, Conversation detail no
  longer overflows horizontally, the empty Agent state links directly to API
  Management, and Run labels are presented in the selected language.

- 2026-08-09: Trace Farm terminal analyses now support an explicit two-step
  delete flow. Browser acceptance removed a dedicated completed fixture,
  reduced the recent-analysis count immediately, and verified that a waiting
  analysis exposes only the non-terminal protection hint.

- 2026-08-09: Conversation and Trace now switch between index and detail at
  684px with explicit back actions. Browser acceptance verified that hidden
  master/detail surfaces are not simultaneously visible, selected Span
  input/output remains accessible, and both pages have no horizontal overflow.
- 2026-08-09: Conversation previews now truncate on Unicode rune boundaries;
  Go regression coverage prevents replacement characters in Chinese titles.
- 2026-08-09: 34 Web tests, TypeScript typecheck, Vite production build,
  Go control-plane tests and `go vet` passed.

- 2026-08-07: Removed the duplicate expanded API-key presentation. Each Agent
  now renders exactly one credential row with masked value, copy and delete.
  Authenticated Playwright acceptance passed create, clipboard copy, revoke,
  zero console errors and 390px layout with no horizontal overflow.

- 2026-08-07: API Management now edits the owner's Provider, Base URL, Model
  and API Key while rendering only key-present state after save. Browser
  reload acceptance proved the secret is never returned to the form.
- 2026-08-07: Language and system/light/dark theme controls moved to Settings;
  browser acceptance proved both preferences survive reload. The sidebar no
  longer carries global language or theme controls.

- 2026-08-07: 35 Web tests, TypeScript typecheck and Vite production build passed.
- 2026-08-07: Go control-plane tests and `go vet` passed.
- 2026-08-07: Browser acceptance passed for configure, reveal, connection check,
  registered-only Agent list and 390px mobile overflow.
- 2026-08-07: Removed duplicate primary CTA hierarchy on the Agent page; new
  Agent creation is secondary while the selected Agent keeps one contextual action.
- 2026-08-07: Dedicated onboarding route passed new/existing Agent, configuration
  copy, URL refresh recovery and 390px no-overflow browser acceptance.
- 2026-08-07: Replaced the onboarding route with one API Management page and
  verified create form, existing-key reveal/copy, Agent-statistics separation,
  and 694px no-overflow behavior.
- 2026-08-07: Added copy-in-place for both ingest endpoints and replaced
  Trace Farm's implicit document opening with a stable recent-analysis overview;
  34 tests, typecheck, production build and browser entry/open/return checks passed.
- 2026-08-07: Settings now shows the deployment-managed Evolution model
  Provider, Base URL, Model and key readiness; browser layout had zero horizontal
  overflow and no credential content was rendered.
- 2026-08-07: Public browser acceptance passed for Settings model visibility,
  copy-ready ingest endpoints and Trace Farm's empty analysis overview.

- 2026-08-11: Trace detail gained evidence-aware presentation for turns, model
  requests, tool calls and tool results. Large Codex payloads now show the
  visible conversation, command fields and terminal output first; injected
  context and raw JSON are folded. Truncated model requests fail closed with a
  clear evidence warning instead of rendering malformed JSON as user text.
  42 Web tests, typecheck and production build passed.

- 2026-08-12: Trace semantics now render the cross-process Barena/XiaoBaOS
  chain as one Run, two Turns, two model calls and two deterministic Checks.
  Two `xiaoba.session` wrappers remain available under Raw Span but are folded
  from the default chain. Browser acceptance opened real nine-Span Trace
  `0c133a14cdd90f81d39c488b85f78aae`, selected the first evidence-bearing Turn,
  rendered the requested model plus Run/Check facts, and exposed the real
  prompt/answer. All 46 Web tests, typecheck and production build passed.
