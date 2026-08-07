# Catena Web Plan

Updated: 2026-08-07

## Current state

- [x] Standalone React/Vite application served by Go.
- [x] Agent, Conversation, Memory, Trace and Trace Farm journeys.
- [x] GitHub identity, API-key management and bilingual UI.
- [x] Agent Trace-window selection and role-stage progress.
- [x] Span waterfall and memory graph rendering.

## Next

- [ ] Add first-run connection checklist.
- [ ] Add responsive browser acceptance to CI.
- [ ] Improve accessibility and keyboard navigation.
- [ ] Add durable optimistic/retry states for long-running jobs.

## Acceptance

`pnpm test`, `pnpm typecheck` and `pnpm build` pass; desktop and mobile journeys expose no console errors.
