# Barena MVP1 local workbench

Barena uses this LangWatch-derived application as its authenticated Web and
Trace subsystem. The downstream addition is intentionally narrow:

- **Trace Explorer** keeps raw OTLP spans, search, metadata, events, and the
  waterfall;
- **Evolution** reviews evidence as Issues, immutable Cases, Evaluations, and
  Release Gates;
- **XiaoBa Evolution Runtime** executes the platform-owned UserCat,
  InspectorCat, ReviewerCat, and EvolutionCat roles through the shared XiaoBaOS
  agent loop;
- the fork's server-side tRPC router calls the Go control plane;
- the browser never treats Trace presentation as an evaluation decision.

The Platform has two honest entry paths:

- **HTTP Explore**: select a registered reachable HTTP Agent and let the
  existing Scenario runtime simulate a user, execute the conversation, and run
  its trace-aware Judge;
- **local/private execution**: run Barena CLI beside XiaoBaOS or another Agent
  Runtime and export Run events plus OTLP to the Platform.

Both enter the same evolution loop:

```text
Completed Explore + retained Trace
  -> Adopt canonical Barena Run
  -> Create Barena issue
  -> Review and promote immutable Case
  -> Replay Case
  -> Engine Evaluation
  -> Release Gate
```

The embedded XiaoBaOS Runtime is the evaluator/evolution worker, not the Agent
under test. Target Agents remain external HTTP endpoints or local/private
Runtimes that export OTLP and Run evidence.

## Prerequisites

Start the normal LangWatch development dependencies (PostgreSQL, ClickHouse,
and Redis) using the upstream repository instructions. Build and start the
Barena Go control plane separately on `127.0.0.1:8787`.

Add this integration variable:

```bash
export BARENA_CONTROL_PLANE_URL='http://127.0.0.1:8787'
export BARENA_GATEWAY_SECRET='<one-random-32+-character-shared-secret>'
```

Configure the exact same `BARENA_GATEWAY_SECRET` on the Go control plane, and
set `BARENA_PLATFORM_INTERNAL_URL` on the Go/Engine side to this application's
origin. The first signs every project-scoped workflow request; the second lets
the Replay Engine send its evaluator and boundary spans to the signed internal
OTLP route. Neither value is browser-visible.

Then start the application from this repository:

```bash
pnpm --dir langwatch install
pnpm --dir langwatch dev
```

When running a second local instance on a non-default port, keep Vite's API
proxy aligned. For example:

```bash
export PORT=5570
export LANGWATCH_API_URL='http://localhost:6570'
pnpm --dir langwatch dev
```

Open `http://localhost:<port>/<project-slug>/evolution`.

## Language

The Barena product shell and Evolution workflow support English and Simplified
Chinese. The first visit follows the browser's primary language; the language
control at the bottom of the sidebar persists a manual choice for later
sessions. Product chrome is localized, while retained Issue content, Trace
attributes, Agent output, and artifacts remain unchanged as execution evidence.

## Send XiaoBaOS OTLP

Use the existing project API key and LangWatch OTLP endpoint:

```bash
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT='http://localhost:<port>/api/otel/v1/traces'
export OTEL_EXPORTER_OTLP_HEADERS='Authorization=Bearer <project-api-key>'
export OTEL_EXPORTER_OTLP_PROTOCOL='http/protobuf'
```

Barena correlation requires the Runtime to retain W3C Trace Context and include
at least:

```text
barena.run.id
barena.attempt.id
barena.actor.role
barena.runtime.id
```

The current browser acceptance used the configured local OpenAI-compatible
`gpt-5.5` provider. Explore Trace
`f6490ee91e796599738d21377493d032` retained nine spans across the User
Simulator, HTTP Agent adapter, target XiaoBaOS Role, and trace-aware Judge. The
target exported `xiaoba.role.turn` as a W3C-parented child of
`SerializedHttpAgentAdapter.call`; Barena did not synthesize it. Retaining the
run in Evolution, promoting its Issue to one immutable Case, and replaying the
safe HTTP contract produced a `cleared` Evaluation and Release Gate.

Replay Trace `b28110fdf9c95219d03487e2218eabca` retained 16 spans in one tree:
two isolated attempts, two `barena.http_agent.replay` boundaries, and two
XiaoBaOS-native `xiaoba.role.turn` children. The verifier checked each retained
`response.txt` for `DONE`. This validates Trace correlation and deterministic
evidence; it is not a benchmark claim about model quality.

## Ownership boundary

| Component | Canonical responsibility |
| --- | --- |
| LangWatch-derived Web + Scenario | login, project boundary, registered HTTP Agent Explore, User Simulator/Judge, live run UI |
| LangWatch Trace subsystem | OTLP ingest and raw Trace storage/query |
| Go control plane | Issue, Case, Harness, Evaluation, Release persistence and embedded Runtime lifecycle |
| Embedded XiaoBaOS Runtime | UserCat, InspectorCat, ReviewerCat, and EvolutionCat role turns only |
| TypeScript Engine | local/private Explore, deterministic Replay, verifier, scorecard, Release Check and XiaoBa worker contract |

Scenario Judge facts explain one HTTP Explore outcome; they are not a Barena
Release Gate. Platform Compare is likewise factual and read-only. A release
status appears only after deterministic Replay and Engine Release Check.

`BARENA_CONTROL_PLANE_URL` is a server-side internal dependency. It must not be
exposed as a second browser authentication surface in production.

## Verify the fork

```bash
pnpm --dir langwatch typecheck:all
pnpm --dir langwatch build
```

For a compile-only local bundle check without deployment secrets, run the build
with `SKIP_ENV_VALIDATION=1`; deployed environments must still provide and
validate their normal runtime configuration.

Browser acceptance must verify the actual Trace action, Issue review, immutable
Case, Replay progress, Evaluation result, Release Gate, and source/replay Trace
links. A static page render is not sufficient.
