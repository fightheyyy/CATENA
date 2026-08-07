# Catena Control Plane

The Go control plane is Catena's only public backend. It serves the React build and owns:

- GitHub OAuth, sessions and personal API keys;
- owner/project isolation;
- OTLP ingestion and Trace/Span queries;
- XiaoBaOS Conversation ingestion;
- canonical Agent grouping;
- Run Bundles, Evolution Jobs, Candidate lineage and audit;
- calls to the private XiaoBaOS Evolution Runtime and optional GauzMem service.

The target Agent and Barena run in the user environment or CI. The embedded Runtime consumes Evidence Packs only.

## Run locally

The supported path is the repository Compose stack:

```bash
./deploy/catena-mvp1/demo.sh up
```

For backend-only development:

```bash
export BARENA_DATABASE_URL='postgres://catena_core:catena-core-local@127.0.0.1:54329/catena_core?sslmode=disable'
export CATENA_CLICKHOUSE_DSN='http://default:langwatch@127.0.0.1:8123/langwatch?dial_timeout=5s'
go run ./cmd/barena-server
```

The server binds to `127.0.0.1:8787` by default. Remote binding requires explicit opt-in, GitHub OAuth and an HTTPS callback.

## Verification

```bash
go test ./...
go vet ./...
go test -race ./internal/control
```

Architecture and invariants are defined in [SPEC.md](./SPEC.md); active work is tracked in [PLAN.md](./PLAN.md).
