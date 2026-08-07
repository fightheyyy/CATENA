# Catena Web Plan

Updated: 2026-08-07

## Current state

- [x] Standalone React/Vite application served by Go.
- [x] Agent, Conversation, Memory, Trace and Trace Farm journeys.
- [x] GitHub identity, Agent-bound credential management and bilingual UI.
- [x] Agent Trace-window selection and role-stage progress.
- [x] Span waterfall and memory graph rendering.

## Next

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
- [ ] Add responsive browser acceptance to CI.
- [ ] Improve accessibility and keyboard navigation.
- [ ] Add durable optimistic/retry states for long-running jobs.

## Acceptance

`pnpm test`, `pnpm typecheck` and `pnpm build` pass; desktop and mobile journeys expose no console errors.

## Verification log

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
