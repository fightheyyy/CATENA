# Catena Control Plane

This directory contains Catena's public Go product backend. It serves the
standalone React build, owns OAuth/session and personal API keys, accepts OTLP,
XiaoBaOS user-visible Conversation batches, and immutable Barena Run Bundles,
persists product state in PostgreSQL and
Trace spans in ClickHouse, and orchestrates the embedded XiaoBaOS Evolution
Runtime.

The user's target Agent and Barena execute in the user environment or CI.
Catena never hosts or impersonates that target Runtime. Its XiaoBaOS Runtime is
restricted to InspectorCat, EvolutionCat, and ReviewerCat evidence-consumption
stages and can emit only `draft/unverified` candidates.

## MVP1

MVP1 demonstrates this retained asset pipeline:

```text
local Barena Explore
  -> genuine OTLP + immutable Run Bundle
  -> Catena Evidence Pack
  -> InspectorCat / EvolutionCat / ReviewerCat
  -> agent.md / Skill / Role / XiaoBaOS Harness
```

The standalone Web selects one Agent and bounded Trace window, starts an
idempotent Evolution Job, shows all three role stages, and exposes the resulting
Agent asset directly.
Local Barena remains an independent E2E/release-CI engine. The retained live
acceptance is documented in
[`docs/acceptance/CATENA_MVP1_DEMO.md`](../docs/acceptance/CATENA_MVP1_DEMO.md).

## Product topology

The default Compose deployment contains four services: `catena-core`, an
evolution-only `catena-runner`, PostgreSQL, and ClickHouse. React is compiled
into `catena-core`; it is not a second backend. LangWatch-derived code remains
only as a migration source behind the optional `legacy` profile.

The target request path is:

```text
browser -> Catena Go (React, OAuth, API key, product APIs)
Agent / Barena -> Catena Go (OTLP + XiaoBaOS Conversation + Run Bundle)
Catena Go -> XiaoBaOS Evolution Runtime (Evidence Pack only)
Catena Go -> PostgreSQL + ClickHouse
```

Go is the only public product backend. React calls it same-origin; the
evolution worker and databases are not browser-accessible.

## Start the compatibility stack locally

From the repository root, build the Engine first:

```bash
npm install
npm run build

cd platform/web
npm install
npm run build
cd ../..

cd platform
docker compose up -d

export BARENA_DATABASE_URL='postgres://barena:barena-local@127.0.0.1:54329/barena?sslmode=disable'
export BARENA_XIAOBA_PROJECT_ROOT='/absolute/path/to/XiaoBa-CLI'
go run ./cmd/barena-server
```

The server probes `xiaoba` by default and resolves `roles/` and `skills/` from
`BARENA_XIAOBA_PROJECT_ROOT`. Deployments may override
`BARENA_XIAOBA_COMMAND`, `BARENA_XIAOBA_ROLES_ROOT`,
`BARENA_XIAOBA_SKILLS_ROOT`, `BARENA_XIAOBA_EVOLUTION_WORKER`, and
`BARENA_XIAOBA_EVOLUTION_ROOT`. `GET /v1/runtimes` reports a sanitized
ready/blocked manifest; host paths never enter the browser response.

The server binds to `127.0.0.1:8787` by default. Remote binding fails closed
unless the authenticated HTTPS deployment requirements below are satisfied.
Open [http://127.0.0.1:8787](http://127.0.0.1:8787) for Explore, Traces,
History, endpoint Settings, the live actor timeline, and cancellation.

## Identity and endpoint authentication

GitHub OAuth, sessions, `barena_pat_*` personal API keys, OTLP authentication,
and owner isolation are native Go product capabilities. HMAC project context
remains accepted only for the optional legacy migration gateway.

Local mode remains zero-login. To enable account isolation and accept evidence
from endpoint Runners, create a GitHub OAuth App and configure:

```bash
export BARENA_GITHUB_CLIENT_ID='<oauth-client-id>'
export BARENA_GITHUB_CLIENT_SECRET='<oauth-client-secret>'
export BARENA_GITHUB_REDIRECT_URL='http://127.0.0.1:8787/v1/auth/github/callback'
export BARENA_API_TOKEN_ENCRYPTION_KEY='<32-or-more-random-characters>'
go run ./cmd/barena-server
```

The OAuth App needs only `read:user`. Barena uses state + PKCE, discards the
temporary GitHub access token after loading `/user`, and stores only opaque
hashes for sessions and API-token authentication. Each personal token also has
an AES-GCM recovery envelope so its authenticated owner can copy it again from
its Settings row; list responses contain only a mask, and reveal responses are
owner-scoped and non-cacheable. Pre-migration hash-only tokens remain valid but
cannot be recovered. Raw Runs, prompts, artifacts, and Trace evidence stay
private.

After signing in, open **Settings**, create a personal token for the endpoint,
and configure an older local Barena Runner:

```bash
export BARENA_PLATFORM_URL='https://barena.example.com'
export BARENA_PLATFORM_TOKEN='barena_pat_...'
barena explore
```

The same token accepts XiaoBaOS-only visible chat at
`POST /v1/ingest/conversations`. XiaoBaOS configures it with
`CATENA_BASE_URL` and `CATENA_API_KEY`; the browser reads the owner-scoped
result from `GET /v1/conversations` and
`GET /v1/conversations/{conversation_id}?agent_id=...`. Conversation does not
contain system prompts, reasoning, failed deliveries, or Tool internals and is
not reconstructed from OTLP spans.

When both variables are present, `barena explore` asks the Platform for the Run
identity, executes XiaoBaOS locally, uploads `barena.engine_event.v1` records,
and explicitly finishes the remote Run. The token is never forwarded into the
target Agent process unless a user deliberately adds it to a Runtime
environment allowlist.

Remote binding remains opt-in and is intended to sit behind an HTTPS reverse
proxy. It fails closed unless GitHub OAuth and an HTTPS callback are configured:

```bash
export BARENA_SERVER_ADDR='0.0.0.0:8787'
export BARENA_ALLOW_REMOTE='1'
export BARENA_GITHUB_REDIRECT_URL='https://barena.example.com/v1/auth/github/callback'
```

The embedded Web client is built from `platform/web`:

```bash
npm --prefix platform/web install
npm run web:build
```

It adapts LangWatch Scenario presentation components under Apache-2.0 while
using only Barena REST/SSE, Run, Engine Event and evidence contracts. See
`platform/web/NOTICE.md` for the exact source revision and modified files.

Check readiness:

```bash
curl http://127.0.0.1:8787/readyz
curl http://127.0.0.1:8787/v1/system/status
```

Create an Explore Run:

```bash
curl -X POST http://127.0.0.1:8787/v1/runs \
  -H 'content-type: application/json' \
  -d '{
    "operation": "explore",
    "input": {
      "scenario": {
        "schema": "barena.explore_scenario.v1",
        "scenario_id": "local-platform-smoke",
        "target": {"runtime": "xiaobaos", "role": "base"},
        "objective": "测试这个 Agent 如何处理信息不完整的任务",
        "max_turns": 6,
        "timeout_ms": 120000,
        "isolation": {
          "level": "policy_only",
          "network": "disabled",
          "writable_roots": ["workspace"]
        }
      }
    }
  }'
```

Then use the returned `run_id`:

```bash
curl http://127.0.0.1:8787/v1/runs/<run_id>
curl -N http://127.0.0.1:8787/v1/runs/<run_id>/events
curl -X POST http://127.0.0.1:8787/v1/runs/<run_id>/cancel
```

SSE accepts `Last-Event-ID` and replays only later persisted events.

Endpoint Runners use the authenticated ingestion contract:

```text
POST /v1/ingest/runs
POST /v1/ingest/runs/{run_id}/events
POST /v1/ingest/runs/{run_id}/finish
```

Run Events and Agent-native telemetry remain separate channels. Events carry
evaluation state; traces use OTLP and are correlated by Run/Trace context. The
current per-Run OTLP bridge is functional for local evaluation evidence. It is
now a compatibility bridge: the Barena Platform fork is the selected Trace
substrate after passing OTLP ingest, search, metadata/event, and waterfall
acceptance.

The first continuous-evolution API slice turns retained evidence into reviewed
regression assets:

```text
POST /v1/traces/{trace_id}/evolution-jobs
GET  /v1/evolution-jobs
GET  /v1/evolution-jobs/{job_id}
POST /v1/runs/{run_id}/issues
GET  /v1/issues
GET  /v1/issues/{issue_id}
POST /v1/issues/{issue_id}/promote
GET  /v1/cases
GET  /v1/cases/{case_id}
POST /v1/cases/{case_id}/replay
GET  /v1/evaluations
GET  /v1/evaluations/{evaluation_id}
GET  /v1/releases
GET  /v1/releases/{release_id}
```

The Trace endpoint accepts `{ "objective"?: string }` plus an
`Idempotency-Key`. It reads the authenticated owner's stored OTLP Trace
directly; a Barena Run is optional and is never synthesized. The persisted
`catena.evolution_evidence_pack.v1` contains a bounded/redacted Trace summary,
real spans and tool input/output evidence. InspectorCat, EvolutionCat, and
ReviewerCat produce only evidence-linked `agent.md`, Skill, or Role assets;
Harness is accepted only for canonical XiaoBaOS. They do not generate Memory or
Case, run the target Agent, or create Release decisions.

Local Barena may atomically synchronize terminal facts through:

```text
POST /v1/ingest/run-bundles
GET  /v1/run-bundles/{run_bundle_id}
```

The canonical body schema is `barena.run_bundle.v1`; the last ordered Event
must be terminal and `terminal_fact_sha256` must match its exact JSON payload
bytes. The endpoint is owner-scoped and idempotent. Catena stores the opaque
Barena result/scorecard/decision fact without recomputing it. Explore terminal
facts intentionally contain no Release decision. The older create/Event/finish
ingest lifecycle remains available for compatibility.

An Issue may reference a retained `trace_id`; the API rejects a Trace that does
not belong to the source Run. Promotion snapshots the source Run input and
Runtime context into immutable `barena.case.v1` revision 1. Repeating promotion
returns the existing Case instead of creating or mutating another revision.

Replay is idempotent by request key. Go sends exactly
`input.platform_case + case_base_dir` to the Node Worker. The TypeScript Engine
compiles the reviewed Case and remains the only component that verifies
artifacts and computes the result/decision. Go verifies the Run Package and
persists the Engine terminal fact; failed, cancelled, evidence-free, or
semantically invalid Runs cannot create Evaluation or Release records.

## Start the MVP1 Web

Start this Go service first, then start the
[`fightheyyy/barena-platform`](https://github.com/fightheyyy/barena-platform)
fork using its normal LangWatch development infrastructure and:

```bash
export BARENA_CONTROL_PLANE_URL='http://127.0.0.1:8787'
export BARENA_GATEWAY_SECRET='<one-random-32+-character-shared-secret>'
```

Configure the exact same `BARENA_GATEWAY_SECRET` on the Go service. Also set
`BARENA_PLATFORM_INTERNAL_URL` on the Go/Engine side to the fork origin, for
example `http://127.0.0.1:5570`. Browser-to-Go requests and Engine-to-Trace
OTLP requests are then HMAC-bound to the authenticated project; the shared
secret is never exposed to the browser. Agent-native OTLP continues to use the
fork-issued project API key and the public `/api/otel/v1/traces` endpoint.

Open `/<project>/evolution` in the fork. Browser calls remain inside the
authenticated fork; its server-side tRPC router proxies project-scoped workflow
requests to Go. Raw OTLP stays in LangWatch/ClickHouse, while Go stores only the
evolution-domain records.

Registered HTTP Agent Explore propagates W3C Trace Context and expects the
target Runtime to export OTLP directly to the fork, as the current deterministic
acceptance fixture does. Compatibility-mode local Explore still captures OTLP
inside each local Run; older retained protobuf envelopes may be imported for
historical inspection, but that import path is not the Platform execution
architecture.

## Verify

```bash
go test -race ./...
go vet ./...
```

The integration tests launch a deterministic fake Node Worker and exercise the
real HTTP create, endpoint token lifecycle, edge ingestion, owner isolation,
embedded Web surface, ordered Event/SSE reconnect, package verification, and
cancellation paths without model calls. Repository-level acceptance also runs
`npm run build`, all TypeScript tests, the fork's full typecheck and production
bundle build, and the complete browser workflow.
