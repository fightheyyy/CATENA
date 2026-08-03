# Catena MVP1 — six-container deployment

This is the smallest complete local deployment of Catena:

| Service | Responsibility |
| --- | --- |
| `catena-app` | LangWatch-derived Web/API, GitHub login, projects/API keys, Scenario Explore, OTLP ingestion, Trace query, and Catena UI |
| `catena-core` | Go workflow control plane for Run, Evolution Job, Issue, Case, Evaluation, Release, idempotency, and audit |
| `catena-runner` | Pinned Barena TypeScript engine plus the restricted XiaoBaOS four-role evaluator/evolution Runtime |
| `postgres` | Separate LangWatch and Catena control-plane databases |
| `clickhouse` | OTLP Trace and Span storage |
| `redis` | Scenario queues, notifications, and cache |

Target Agent Runtimes remain external. Catena receives their OTLP telemetry and
can execute Explore against a registered HTTP Agent endpoint. The embedded
XiaoBaOS Runtime contains evaluation/evolution roles only.

## Run

Docker Desktop with Compose and BuildKit is required:

```bash
git clone https://github.com/fightheyyy/CATENA.git
cd CATENA
./deploy/catena-mvp1/demo.sh up
```

The first build downloads pinned Barena, XiaoBaOS, and LangWatch image
dependencies. It then waits for all six containers and runs a no-model-call
smoke test. Open <http://127.0.0.1:5570>.

```bash
./deploy/catena-mvp1/demo.sh smoke
./deploy/catena-mvp1/demo.sh logs
./deploy/catena-mvp1/demo.sh down
```

`down` preserves database volumes. Copy `.env.example` to `.env` to configure
GitHub OAuth, non-default ports, production-grade secrets, or the XiaoBaOS
model endpoint. Never commit `.env`.

## GitHub login

Create a GitHub OAuth App using this local callback:

```text
http://localhost:5570/api/auth/callback/github
```

Then configure `CATENA_AUTH_PROVIDER=github`, `CATENA_GITHUB_CLIENT_ID`, and
`CATENA_GITHUB_CLIENT_SECRET`. For a deployed instance, replace localhost with
its HTTPS origin.

## OTLP and edge Runs

Create a project API key in Catena, then use it for both OTLP and Barena Run
event ingestion:

```bash
export BARENA_PLATFORM_URL='http://127.0.0.1:5570'
export BARENA_PLATFORM_API_KEY='sk-lw-...'
export OTEL_EXPORTER_OTLP_ENDPOINT='http://127.0.0.1:5570/api/otel'
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer ${BARENA_PLATFORM_API_KEY}"
```

Catena resolves both transports to the same project and never forwards the
public API key to the Go service. Internal requests use a timestamped,
body-bound HMAC project gateway.

## Product walkthrough

1. Send an OTLP Trace or run an Explore Scenario against an HTTP Agent.
2. Open the Trace and retain a concrete failure as an Issue.
3. Start an Evolution Job to inspect InspectorCat, EvolutionCat, and
   ReviewerCat evidence and proposals.
4. Review the proposal into an immutable Regression Case.
5. Replay it through Barena and inspect the resulting Evaluation and Release
   Gate (`cleared`, `held`, or `rejected`).
6. Compare compatible completed Runs when a factual A/B view is useful.

Evolution outputs remain `draft/unverified`; Catena never mutates or publishes
a target Agent automatically.
