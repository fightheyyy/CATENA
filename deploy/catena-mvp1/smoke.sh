#!/usr/bin/env bash
set -euo pipefail

deploy_dir="$(cd "$(dirname "$0")" && pwd)"
compose_file="$deploy_dir/compose.yml"
compose=(docker compose --project-directory "$deploy_dir" -f "$compose_file")
if [[ -f "$deploy_dir/.env" ]]; then
  compose+=(--env-file "$deploy_dir/.env")
fi

expected=$'catena-app\ncatena-core\ncatena-runner\nclickhouse\npostgres\nredis'
actual="$("${compose[@]}" config --services | sort)"
if [[ "$actual" != "$expected" ]]; then
  echo "unexpected service set:" >&2
  printf '%s\n' "$actual" >&2
  exit 1
fi

for service in catena-app catena-core catena-runner clickhouse postgres redis; do
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

core_uid="$("${compose[@]}" exec -T catena-core id -u)"
runner_uid="$("${compose[@]}" exec -T catena-runner id -u)"
if [[ "$core_uid" != "$runner_uid" ]]; then
  echo "catena-core and catena-runner must share the same volume UID" >&2
  exit 1
fi
"${compose[@]}" exec -T catena-core sh -c \
  'mkdir -p /var/lib/catena/evolution/.compose-write-smoke && rmdir /var/lib/catena/evolution/.compose-write-smoke'

app_port="$("${compose[@]}" port catena-app 5560 | awk -F: '{print $NF}')"
core_port="$("${compose[@]}" port catena-core 8787 | awk -F: '{print $NF}')"
runner_port="$("${compose[@]}" port catena-runner 8790 | awk -F: '{print $NF}')"

curl -fsS "http://127.0.0.1:${runner_port}/readyz"
curl -fsS "http://127.0.0.1:${core_port}/readyz"
curl -fsS "http://127.0.0.1:${core_port}/v1/system/status"
curl -fsSL -o /dev/null "http://127.0.0.1:${app_port}/"

auth_session_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  "http://127.0.0.1:${app_port}/api/auth/session")"
if [[ "$auth_session_status" != "200" ]]; then
  echo "catena-app auth session endpoint returned $auth_session_status" >&2
  exit 1
fi

unauthenticated_ingest_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X POST \
  -H 'content-type: application/json' \
  --data '{"operation":"explore","input":{}}' \
  "http://127.0.0.1:${app_port}/api/barena/v1/ingest/runs")"
if [[ "$unauthenticated_ingest_status" != "401" ]]; then
  echo "Barena ingress must reject a missing project key; got $unauthenticated_ingest_status" >&2
  exit 1
fi

"${compose[@]}" exec -T postgres pg_isready -U postgres -d postgres
"${compose[@]}" exec -T clickhouse wget -qO- http://127.0.0.1:8123/ping
"${compose[@]}" exec -T redis redis-cli ping

app_logs="$("${compose[@]}" logs --no-color catena-app)"
if grep -Eq 'prisma:error|Unknown argument `[^`]+`' <<<"$app_logs"; then
  echo "catena-app logged a Prisma schema/client mismatch" >&2
  exit 1
fi

echo
echo "Catena MVP1 smoke passed: six healthy services, auth, and protected Barena ingress."
