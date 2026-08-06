#!/usr/bin/env bash
set -euo pipefail

deploy_dir="$(cd "$(dirname "$0")" && pwd)"
env_file="$deploy_dir/.env"

if [[ ! -f "$env_file" ]]; then
  echo "missing $env_file; start from .env.public.example" >&2
  exit 1
fi

required=(
  CATENA_DOMAIN
  CATENA_GATEWAY_SECRET
  CATENA_API_TOKEN_SECRET
  CATENA_GITHUB_CLIENT_ID
  CATENA_GITHUB_CLIENT_SECRET
  CATENA_POSTGRES_ADMIN_PASSWORD
  CATENA_CORE_DB_PASSWORD
  CATENA_LANGWATCH_DB_PASSWORD
  CATENA_CLICKHOUSE_PASSWORD
  XIAOBA_LLM_API_BASE
  XIAOBA_LLM_API_KEY
  XIAOBA_LLM_MODEL
)

set -a
# shellcheck disable=SC1090
source "$env_file"
set +a

for name in "${required[@]}"; do
  value="${!name:-}"
  if [[ -z "$value" || "$value" == *"change-me"* || "$value" == replace-with-* ]]; then
    echo "$name must be configured with a non-default value" >&2
    exit 1
  fi
done

for name in CATENA_GATEWAY_SECRET CATENA_API_TOKEN_SECRET CATENA_POSTGRES_ADMIN_PASSWORD CATENA_CORE_DB_PASSWORD CATENA_CLICKHOUSE_PASSWORD; do
  value="${!name}"
  if (( ${#value} < 24 )); then
    echo "$name must contain at least 24 characters" >&2
    exit 1
  fi
done

compose=(
  docker compose
  --project-directory "$deploy_dir"
  -f "$deploy_dir/compose.yml"
  -f "$deploy_dir/compose.public.yml"
  --env-file "$env_file"
)

action="${1:-up}"
case "$action" in
  config)
    "${compose[@]}" config
    ;;
  build)
    "${compose[@]}" build catena-runner catena-core
    ;;
  up)
    "${compose[@]}" up -d --build --wait --wait-timeout 1200
    "$deploy_dir/smoke-public.sh"
    ;;
  smoke)
    "$deploy_dir/smoke-public.sh"
    ;;
  logs)
    "${compose[@]}" logs -f --tail=200 caddy catena-core catena-runner
    ;;
  down)
    "${compose[@]}" down --remove-orphans
    ;;
  *)
    echo "usage: $0 {config|build|up|smoke|logs|down}" >&2
    exit 2
    ;;
esac
