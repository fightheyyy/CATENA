# Catena MVP1 deployment

The default Compose path runs the Catena product: React embedded in Go,
PostgreSQL/ClickHouse, and one XiaoBaOS Evolution Runtime. The retained
LangWatch source and Redis are available only through the `legacy` profile for
Trace migration and rollback.

This is the smallest complete local deployment of Catena:

| Service | Responsibility |
| --- | --- |
| `catena-core` | Public Go server: React Web, OAuth, API keys, OTLP/Trace and XiaoBaOS Conversation APIs, Run, Evolution, Issue, Case, Evaluation, Release, and audit |
| `catena-runner` | Restricted XiaoBaOS Evolution Runtime; the service runs in evolution-only mode |
| `postgres` | Catena identity, Run, Evolution Job, Candidate, Case, and Release records |
| `clickhouse` | OTLP Trace and Span storage |

The optional `legacy` profile additionally starts `catena-app` and Redis only
for retained LangWatch Trace migration. It is not the Catena product path.

The optional `memory` profile adds three services without exposing another
public product port:

| Service | Responsibility |
| --- | --- |
| `gauzmem` | Pinned GauzRag memory compiler; local Qdrant semantic index |
| `gauzmem-mysql` | GauzMem evidence, Fact, Topic, and temporal source of truth |
| `neo4j` | Entity/relation projection and multi-hop graph expansion |

Catena deliberately does not start GauzMem's duplicate Web/auth, Redis, or
MinIO paths. PostgreSQL remains Catena's product database; GauzMem keeps its
working MySQL authority until a complete PostgreSQL migration can pass memory
lifecycle and recall-parity tests.

Target Agent Runtimes remain external. Catena receives their OTLP telemetry;
local Barena owns Explore, Replay, Compare, verification, and release truth.
The embedded XiaoBaOS Runtime consumes retained Evidence Packs and produces
draft candidates only.

## Run

Docker Desktop with Compose and BuildKit is required:

```bash
git clone https://github.com/fightheyyy/CATENA.git
cd CATENA
./deploy/catena-mvp1/demo.sh up
```

The first build downloads the pinned XiaoBaOS/Barena evolution worker sources.
It then waits for all four product containers and runs a no-model-call smoke
test. Open <http://127.0.0.1:5570> for the React + Go product.

```bash
./deploy/catena-mvp1/demo.sh smoke
./deploy/catena-mvp1/demo.sh logs
./deploy/catena-mvp1/demo.sh down
```

Start the retained migration source only when needed:

```bash
./deploy/catena-mvp1/demo.sh legacy-up
```

`down` preserves database volumes. Copy `.env.example` to `.env` to configure
GitHub OAuth, non-default ports, production-grade secrets, or the XiaoBaOS
model endpoint. Never commit `.env`.

## Public single-node Beta

The public overlay adds Caddy and refuses to start with placeholder OAuth,
provider, service, or database credentials. It is intended for one-host Beta
and demo deployment, not multi-node high availability.

```bash
cp deploy/catena-mvp1/.env.public.example deploy/catena-mvp1/.env
# Fill the HTTPS domain or public IPv4 address, GitHub OAuth credentials,
# independent random secrets,
# and the XiaoBaOS-compatible model provider.
./deploy/catena-mvp1/public.sh config
./deploy/catena-mvp1/public.sh up
```

If the target host cannot reach Docker Hub, preload the exact release images
and use the registry-independent start path:

```bash
./deploy/catena-mvp1/public.sh start
```

`start` refuses to build or pull. It only starts images already present on the
host and then runs the same public smoke test as `up`.

Only ports 80 and 443 are public. Go, the Evolution Runtime, PostgreSQL, and
ClickHouse remain private or loopback-bound. Caddy obtains and renews the TLS
certificate, redirects HTTP to HTTPS, and applies the deployment security
headers. Verify an existing deployment with:

When `CATENA_DOMAIN` is an IPv4 address, `public.sh` automatically uses the
Let's Encrypt `shortlived` profile and HTTP-01 validation. The certificate is
publicly trusted and renewed automatically; update the GitHub OAuth App callback
to `https://<IP>/api/auth/callback/github` before signing in.

```bash
./deploy/catena-mvp1/public.sh smoke
./deploy/catena-mvp1/public.sh logs
```

Back up the `postgres-data`, `clickhouse-data`, `catena-data`, and `caddy-data`
volumes before host or image migration. An in-flight role turn does not yet
resume after host loss; retained facts and completed jobs remain durable.

To enable Conversation-derived memory, configure `DASHSCOPE_API_KEY` and the GauzMem
settings shown in `.env.example`, then start the optional profile:

```bash
docker compose --profile memory -f deploy/catena-mvp1/compose.yml up -d --build
```

Neo4j normally installs APOC and Graph Data Science on first boot. On a host
without outbound access to the Neo4j plugin registry, preload the compatible
GDS jar into the `neo4j-plugins` volume and set
`CATENA_NEO4J_PLUGINS='["apoc"]'`; the default remains the self-installing path.

The browser still calls only Catena. Go retrieves an owned XiaoBaOS
user-visible Conversation, removes common credential patterns, bounds the
payload, and submits it to the private GauzMem API. Ordinary Conversation sync
and OTLP ingestion never create memory implicitly.

The pinned GauzMem 2.0.1 image carries two narrow compatibility corrections:
pipeline Step 8 reuses the Qdrant Local vector client already owned by the
Embedding service instead of opening the same storage directory twice, and
bundle search receives the missing bounded async graph-expansion adapter on the
infrastructure Neo4j store. Remove both Dockerfile patches when the pinned
upstream revision contains those fixes.

## OAuth login

Catena supports one identity provider per deployment. GitHub is recommended
for the developer-facing product because it removes password management and
matches the repository/release workflow.

Create a GitHub OAuth App using this local callback:

```text
http://localhost:5570/api/auth/callback/github
```

Then configure `CATENA_AUTH_PROVIDER=github`, `CATENA_GITHUB_CLIENT_ID`, and
`CATENA_GITHUB_CLIENT_SECRET`. For a deployed instance, replace localhost with
its HTTPS origin. The callback must match exactly, including scheme and path.

Google is also supported. Create a Google OAuth client with this callback:

```text
http://localhost:5570/api/auth/callback/google
```

Then configure `CATENA_AUTH_PROVIDER=google`, `CATENA_GOOGLE_CLIENT_ID`, and
`CATENA_GOOGLE_CLIENT_SECRET`. Missing credentials fail at startup instead of
leaving the sign-in page in a redirect loop. Email/password remains only the
zero-configuration local fallback.

## OTLP and edge Runs

Create a project API key in Catena, then use it for both OTLP and Barena Run
event ingestion:

```bash
export BARENA_PLATFORM_URL='http://127.0.0.1:5570'
export BARENA_PLATFORM_API_KEY='barena_pat_...'
export OTEL_EXPORTER_OTLP_ENDPOINT='http://127.0.0.1:5570/v1/otlp'
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer ${BARENA_PLATFORM_API_KEY}"
```

Catena resolves both transports to the same owner in Go. Authentication uses
only the token hash; a separate AES-GCM envelope supports owner-only copy from
the corresponding Settings row after refresh. Full values are never returned
by token lists or forwarded to an Engine. Keep `CATENA_API_TOKEN_SECRET`
stable across restarts and rotate tokens before changing it.

XiaoBaOS can additionally send its user-visible Conversation Journal with the
same key:

```bash
export CATENA_BASE_URL='http://127.0.0.1:5570'
export CATENA_API_KEY="${BARENA_PLATFORM_API_KEY}"
export XIAOBA_CONVERSATION_AGENT_ID='my-xiaoba'
```

This first-party JSON channel is separate from OTLP and appears under
**Conversations**. Catena accepts only `user` and successfully delivered
`assistant` text/file messages; hidden Runtime execution remains Trace data.

## Product walkthrough

1. Send OTLP Traces from any Agent, or connect a local Barena Explore.
2. Open the Agent and start a Trace Farm Job from a bounded time window.
3. Watch InspectorCat, EvolutionCat, and ReviewerCat consume the Evidence Pack.
4. Review and copy the resulting `agent.md`, Skill, or Role asset; XiaoBaOS may
   additionally produce a Harness optimization.

Conversation-derived memory follows the separate GauzMem path. Catena never
mutates or publishes a target Agent automatically.

## Target runtime boundary

The local release topology has four containers: `catena-core`,
`catena-runner` (evolution-only), PostgreSQL, and ClickHouse. Public Beta adds
Caddy as the fifth service. `catena-core`
serves React and owns OAuth, API keys, OTLP ingestion, Trace queries, durable
jobs, candidates, and audit. `catena-runner` runs the embedded XiaoBaOS
Evolution Runtime and never invokes the user's target Agent. React calls only
Go; evolution stages return versioned Events and evidence-linked proposals
instead of writing product databases directly.
