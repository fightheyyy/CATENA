#!/usr/bin/env bash

# Catena's MVP keeps the Web/API process and LangWatch background consumers
# in one product container. They remain separate processes but share one
# image, lifecycle, and service boundary.
set -euo pipefail

pnpm run start:prepare:db

worker_pid=""
app_pid=""

shutdown() {
  trap - EXIT INT TERM
  if [[ -n "$worker_pid" ]]; then kill -TERM "$worker_pid" 2>/dev/null || true; fi
  if [[ -n "$app_pid" ]]; then kill -TERM "$app_pid" 2>/dev/null || true; fi
  if [[ -n "$worker_pid" ]]; then wait "$worker_pid" 2>/dev/null || true; fi
  if [[ -n "$app_pid" ]]; then wait "$app_pid" 2>/dev/null || true; fi
}

trap shutdown EXIT INT TERM

pnpm -s run start:workers &
worker_pid=$!
pnpm -s run start:app &
app_pid=$!

set +e
wait -n "$worker_pid" "$app_pid"
status=$?
set -e
shutdown
exit "$status"
