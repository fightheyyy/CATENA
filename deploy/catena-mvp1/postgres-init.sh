#!/usr/bin/env sh
set -eu

: "${CATENA_CORE_DB_PASSWORD:=catena-core-local}"

psql --set=ON_ERROR_STOP=1 \
  --set=catena_core_password="$CATENA_CORE_DB_PASSWORD" \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" <<'SQL'
CREATE USER catena_core WITH PASSWORD :'catena_core_password';
CREATE DATABASE catena_core OWNER catena_core;
SQL
