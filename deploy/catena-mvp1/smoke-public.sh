#!/usr/bin/env bash
set -euo pipefail

deploy_dir="$(cd "$(dirname "$0")" && pwd)"
env_file="$deploy_dir/.env"
set -a
# shellcheck disable=SC1090
source "$env_file"
set +a

compose=(
  docker compose
  --project-directory "$deploy_dir"
  -f "$deploy_dir/compose.yml"
  -f "$deploy_dir/compose.public.yml"
  --env-file "$env_file"
)

required_services=(catena-core catena-runner caddy clickhouse postgres)
for service in "${required_services[@]}"; do
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

origin="https://${CATENA_DOMAIN}"
redirect="$(curl -fsSI "http://${CATENA_DOMAIN}/readyz" | awk 'BEGIN{IGNORECASE=1} /^location:/ {gsub(/\r/,""); print $2}')"
if [[ "$redirect" != "${origin}/readyz" ]]; then
  echo "HTTP did not redirect to the expected HTTPS origin" >&2
  exit 1
fi

curl -fsS "${origin}/readyz" >/dev/null
curl -fsS "${origin}/v1/system/status" | grep -q '"trace_store":"available"'
curl -fsS "${origin}/" >/dev/null

oauth_headers="$(mktemp)"
trap 'rm -f "$oauth_headers"' EXIT
curl -fsS -D "$oauth_headers" -o /dev/null "${origin}/v1/auth/github?callback_url=/"
if ! grep -qi '^location: https://github.com/login/oauth/authorize' "$oauth_headers"; then
  echo "GitHub OAuth did not reach GitHub authorization" >&2
  exit 1
fi

ingest_status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
  -H 'content-type: application/json' --data '{"operation":"explore","input":{}}' \
  "${origin}/v1/ingest/runs")"
if [[ "$ingest_status" != "401" && "$ingest_status" != "404" ]]; then
  echo "public ingestion accepted a missing API key: $ingest_status" >&2
  exit 1
fi

echo "Catena public smoke passed at ${origin}"
