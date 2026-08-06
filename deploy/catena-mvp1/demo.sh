#!/usr/bin/env bash
set -euo pipefail

deploy_dir="$(cd "$(dirname "$0")" && pwd)"
compose_file="$deploy_dir/compose.yml"
compose=(docker compose --project-directory "$deploy_dir" -f "$compose_file")

if [[ -f "$deploy_dir/.env" ]]; then
  compose+=(--env-file "$deploy_dir/.env")
fi

action="${1:-up}"
case "$action" in
  config)
    "${compose[@]}" config
    ;;
  build)
    "${compose[@]}" build catena-runner catena-core
    ;;
  up)
    "${compose[@]}" up -d --build --wait --wait-timeout 600
    "$deploy_dir/smoke.sh"
    ;;
  smoke)
    "$deploy_dir/smoke.sh"
    ;;
  logs)
    "${compose[@]}" logs -f --tail=200 catena-core catena-runner
    ;;
  legacy-up)
    "${compose[@]}" --profile legacy up -d --build --wait --wait-timeout 600
    ;;
  down)
    "${compose[@]}" down --remove-orphans
    ;;
  *)
    echo "usage: $0 {config|build|up|smoke|logs|legacy-up|down}" >&2
    exit 2
    ;;
esac
