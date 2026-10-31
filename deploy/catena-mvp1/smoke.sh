#!/usr/bin/env bash
set -euo pipefail

deploy_dir="$(cd "$(dirname "$0")" && pwd)"
compose_file="$deploy_dir/compose.yml"
compose=(docker compose --project-directory "$deploy_dir" -f "$compose_file")
if [[ -f "$deploy_dir/.env" ]]; then
  compose+=(--env-file "$deploy_dir/.env")
fi

expected=$'catena-core\ncatena-engine\nclickhouse\npostgres'
actual="$("${compose[@]}" config --services | sort)"
if [[ "$actual" != "$expected" ]]; then
  echo "unexpected service set:" >&2
  printf '%s\n' "$actual" >&2
  exit 1
fi

for service in catena-core catena-engine clickhouse postgres; do
  container_id="$("${compose[@]}" ps -q "$service")"
  if [[ -z "$container_id" ]]; then
    echo "$service has no container" >&2
    exit 1
  fi
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id")"
  if [[ "$health" != "healthy" && "$health" != "running" ]]; then
    echo "$service is $health" >&2
    exit 1
  fi
done

app_port="$("${compose[@]}" port catena-core 8787 | awk -F: '{print $NF}')"

curl -fsS "http://127.0.0.1:${app_port}/readyz"
system_status=""
for _ in $(seq 1 15); do
  system_status="$(curl -fsS "http://127.0.0.1:${app_port}/v1/system/status")"
  if grep -q '"analysis_engine":"ready"' <<<"$system_status" \
    && grep -q '"trace_store":"available"' <<<"$system_status"; then
    break
  fi
  sleep 2
done
printf '%s\n' "$system_status"
if ! grep -q '"analysis_engine":"ready"' <<<"$system_status" \
  || ! grep -q '"trace_store":"available"' <<<"$system_status"; then
  echo "catena-server dependencies did not become ready" >&2
  exit 1
fi
curl -fsSL -o /dev/null "http://127.0.0.1:${app_port}/"

auth_session_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  "http://127.0.0.1:${app_port}/v1/auth/session")"
if [[ "$auth_session_status" != "200" ]]; then
  echo "catena-server auth session endpoint returned $auth_session_status" >&2
  exit 1
fi

unauthenticated_ingest_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X POST \
  -H 'content-type: application/json' \
  --data '{"resourceSpans":[]}' \
  "http://127.0.0.1:${app_port}/v1/otlp/v1/traces")"
if [[ "$unauthenticated_ingest_status" != "401" && "$unauthenticated_ingest_status" != "404" ]]; then
  echo "Catena ingress must reject a missing API token; got $unauthenticated_ingest_status" >&2
  exit 1
fi

"${compose[@]}" exec -T postgres pg_isready -U postgres -d postgres
"${compose[@]}" exec -T clickhouse wget -qO- http://127.0.0.1:8123/ping
echo
echo "Catena MVP1 smoke passed: React + Go is public, four services are healthy, and ingestion is protected."
