# Catena MVP1 deployment

The default Compose stack runs four services:

| Service | Responsibility |
| --- | --- |
| `catena-core` | Go API, React Web, OAuth, API keys, OTLP, Trace and Conversation APIs |
| `catena-runner` | XiaoBaOS Evolution Runtime in evidence-consumption mode |
| `postgres` | Product identity, workflow and audit records |
| `clickhouse` | Trace and Span storage |

The `memory` profile adds GauzMem, MySQL and Neo4j. Target Agent Runtimes remain external.

## Local run

```bash
./deploy/catena-mvp1/demo.sh up
```

Open <http://127.0.0.1:5570>.

```bash
./deploy/catena-mvp1/demo.sh smoke
./deploy/catena-mvp1/demo.sh logs
./deploy/catena-mvp1/demo.sh down
```

The first build downloads pinned Barena and XiaoBaOS sources for the Runner image. Copy `.env.example` to `.env` to override ports or OAuth. Catena has no deployment-wide model credential; each signed-in owner configures LLM access in **API Management**.

## Public single-node Beta

```bash
cp deploy/catena-mvp1/.env.public.example deploy/catena-mvp1/.env
# Fill every required value.
./deploy/catena-mvp1/public.sh config
./deploy/catena-mvp1/public.sh up
```

Only Caddy exposes 80/443. Go, Runner, PostgreSQL and ClickHouse remain private or loopback-bound. For a public deployment, configure the GitHub callback exactly as:

```text
https://<CATENA_DOMAIN>/api/auth/callback/github
```

An IPv4 `CATENA_DOMAIN` selects the short-lived certificate Caddy configuration automatically.

Use preloaded images on a host without registry access:

```bash
./deploy/catena-mvp1/public.sh start
```

Back up `postgres-data`, `clickhouse-data`, `catena-data` and `caddy-data` before host migration. MVP1 is not multi-node high availability; an in-flight role turn does not yet resume after host loss.

## Optional memory

Configure `DASHSCOPE_API_KEY`, model credentials and memory passwords, then run:

```bash
docker compose --profile memory -f deploy/catena-mvp1/compose.yml up -d --build
```

The browser continues to call Catena only. Go derives tenant scope and calls GauzMem on the private network.

## Trace Farm LLM

Open **API Management → LLM configuration** and save the Provider, Base URL,
Model and API Key owned by the current user. The API Key is encrypted in
PostgreSQL, never returned after saving, and passed only to that owner's
private Runner request. Language and theme are personal browser preferences
under **Settings**.

## Agent connection

Open **Agents → Connect Agent**, name the Agent, and copy the generated configuration. The key is bound to that Agent and is used for OTLP and Barena Run Bundles:

```bash
export CATENA_URL='http://127.0.0.1:5570'
export CATENA_API_KEY='catena_agent_...'
export OTEL_EXPORTER_OTLP_ENDPOINT="${CATENA_URL}/v1/otlp"
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer ${CATENA_API_KEY}"
```

XiaoBaOS additionally uses `CATENA_BASE_URL`, `CATENA_API_KEY` and `XIAOBA_CONVERSATION_AGENT_ID` to send its user-visible Conversation Journal.

## Validation

```bash
docker compose -f deploy/catena-mvp1/compose.yml config >/dev/null
docker compose -f deploy/catena-mvp1/compose.yml -f deploy/catena-mvp1/compose.public.yml config >/dev/null
./deploy/catena-mvp1/demo.sh smoke
```
