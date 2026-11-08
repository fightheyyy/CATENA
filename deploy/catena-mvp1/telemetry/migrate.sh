#!/bin/sh
set -eu
client() { clickhouse-client --host clickhouse --password "$CLICKHOUSE_PASSWORD" "$@"; }
client --query 'CREATE DATABASE IF NOT EXISTS catena'
client --database catena --query "CREATE TABLE IF NOT EXISTS catena_trace_schema_migrations (version String, checksum String, applied_at DateTime64(9) DEFAULT now64(9)) ENGINE = ReplacingMergeTree(applied_at) ORDER BY version"
for file in /migrations/*.sql; do
  version=$(basename "$file")
  case "$version" in *[!a-zA-Z0-9_.-]*) echo 'Invalid migration filename' >&2; exit 1;; esac
  checksum=$(sha256sum "$file" | awk '{print $1}')
  recorded=$(client --database catena --query "SELECT checksum FROM catena_trace_schema_migrations FINAL WHERE version='$version' LIMIT 1")
  if [ -n "$recorded" ]; then
    if [ "$recorded" != "$checksum" ]; then echo "Migration checksum mismatch: $version" >&2; exit 1; fi
    continue
  fi
  client --database catena --multiquery < "$file"
  client --database catena --query "INSERT INTO catena_trace_schema_migrations (version, checksum) VALUES ('$version', '$checksum')"
done
