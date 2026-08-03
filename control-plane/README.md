# Catena Control Plane

This directory contains Catena's Go evidence control plane and embedded Web
compatibility client. The user's XiaoBaOS and other target Agent Runtimes
execute at the endpoint; the Platform receives ordered Run Events and OTLP
evidence, persists history, and makes evaluation results inspectable. A
separate, restricted XiaoBaOS Runtime is managed by the Platform for UserCat,
InspectorCat, ReviewerCat, and EvolutionCat role turns only.

The LangWatch-derived Scenario runtime owns browser-based Explore for a
registered reachable HTTP Agent, including user simulation and its trace-aware
Judge. The TypeScript Engine owns local/private Explore, deterministic Replay,
artifact verification, scorecards, and Release Check. The Platform UI compares
compatible terminal Explore facts without inventing a winner or release
decision. The existing Go-managed Node worker remains a loopback development
and compatibility path, not the target cloud deployment topology.

## MVP1

MVP1 closes one real, locally demonstrable evolution loop:

```text
XiaoBaOS Explore
  -> genuine OTLP Trace
  -> evidence-backed Issue
  -> reviewed immutable Case
  -> TypeScript Engine Replay
  -> Evaluation
  -> cleared / held / rejected Release Gate
```

The product surface is the fork's **Evolution** page, not the embedded
compatibility client. From a Trace, **Create Barena issue** opens a prefilled
review. Promotion freezes the Case. **Replay Case** starts the existing Engine
Worker, and the Evaluation/Release tabs retain Case, Run, Harness, source Trace,
replay Trace, Engine result, and decision lineage.

The current acceptance retained a nine-span Scenario Trace, including a
W3C-parented `xiaoba.role.turn` span exported by the target fixture. Its
reviewed Case then cleared two isolated Replay attempts in one 16-span Trace:
each native `xiaoba.role.turn` is a direct child of its corresponding
`barena.http_agent.replay` boundary, and both attempts belong to the same
`barena.replay` root. The entire browser loop ran against PostgreSQL and the
configured OpenAI-compatible `gpt-5.5` model. See [SPEC.md](./SPEC.md) for the
exact boundary and [PLAN.md](./PLAN.md) for the verification record.

## Selected platform substrate

The public Web, authentication/project boundary, OTLP ingestion, Trace storage,
search, and waterfall UI now live in the Apache-2.0 downstream fork:

[`fightheyyy/CATENA`](https://github.com/fightheyyy/CATENA)

The fork keeps LangWatch's proven platform infrastructure and adds Barena's
Explore/Replay/Compare and Release CI experience. This directory retains the Go
Run Control + Evidence Ledger for ordered Runs, Events, Cases, immutable Run
Packages, scorecards, and `cleared / held / rejected` decision records. The
TypeScript Engine computes those facts and decisions; Go validates, persists,
and audits them without implementing a second Judge or Release Check. The fork
and Go API join data by `project_id`, `barena.run.id`, `trace_id`, and
`attempt_id`.

The target request path is:

```text
browser / Runner
      -> Barena Platform fork (login, project, API key, gateway)
      -> OTLP Trace store or internal Go Run Control / Evidence Ledger
```

Go is not the target public login surface and does not replace the Trace
backend. It stores neither Platform OAuth credentials nor public project API
keys. The custom embedded Web, personal tokens, and Go-managed Node worker below
remain migration/compatibility paths until the forked gateway, endpoint-push
execution, and Release Workbench reach feature parity.

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

## Legacy standalone identity compatibility

The six-container Catena deployment does **not** expose this Go server as the
public auth surface. GitHub OAuth, sessions, projects, API-key issuance, and
OTLP authentication belong to `catena-app`; the Go control plane accepts only
its HMAC-signed project context. See `deploy/catena-mvp1/README.md` for the
supported deployment flow.

The direct Go OAuth and `barena_pat_*` path below remains available only for
standalone control-plane development and compatibility with older clients. New
Runner integrations must use `BARENA_PLATFORM_API_KEY=sk-lw-...` against the
public Catena app instead of this path.

Local mode remains zero-login. To enable account isolation and accept evidence
from endpoint Runners, create a GitHub OAuth App and configure:

```bash
export BARENA_GITHUB_CLIENT_ID='<oauth-client-id>'
export BARENA_GITHUB_CLIENT_SECRET='<oauth-client-secret>'
export BARENA_GITHUB_REDIRECT_URL='http://127.0.0.1:8787/v1/auth/github/callback'
go run ./cmd/barena-server
```

The OAuth App needs only `read:user`. Barena uses state + PKCE, discards the
temporary GitHub access token after loading `/user`, and stores only opaque
hashes for sessions and personal API tokens. Raw Runs, prompts, artifacts, and
Trace evidence stay private.

After signing in, open **Settings**, create a legacy token for the endpoint,
and configure an older local Barena Runner:

```bash
export BARENA_PLATFORM_URL='https://barena.example.com'
export BARENA_PLATFORM_TOKEN='barena_pat_...'
barena explore
```

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
