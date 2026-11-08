package control

import (
	"context"
	"os"
	"testing"
)

func abOpen(ctx context.Context, dsn string) (*ClickHouseTraceStore, error) {
	return OpenOTelTraceStore(ctx, dsn)
}
func abHandler(t *testing.T, s Store, ch *ClickHouseTraceStore, c TraceIngestConfig) *ManagedHTTPHandler {
	t.Helper()
	sink, e := NewCollectorTraceSink(os.Getenv("CATENA_AB_GATEWAY"))
	if e != nil {
		t.Fatal(e)
	}
	h, e := NewHTTPHandler(s, HTTPConfig{Auth: testAgentAuthConfig(), Traces: ch, Ingress: sink, Ingestion: c})
	if e != nil {
		t.Fatal(e)
	}
	return h
}

const abObserveSQL = "SELECT TraceId, count() FROM otel_traces FINAL WHERE startsWith(CatenaOwnerId, ?) GROUP BY TraceId"
const abIdentitySQL = "SELECT CatenaOwnerId, TraceId, groupArray(SpanId) FROM otel_traces FINAL WHERE startsWith(CatenaOwnerId, ?) GROUP BY CatenaOwnerId, TraceId"
