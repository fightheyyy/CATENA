package control

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type gatedTraceStore struct {
	TraceStore
	insert func(context.Context, string, []TraceSpan) error
}

func (s gatedTraceStore) InsertSpans(ctx context.Context, owner string, spans []TraceSpan) error {
	return s.insert(ctx, owner, spans)
}

type countedBody struct {
	io.Reader
	reads atomic.Int64
}

func (b *countedBody) Read(p []byte) (int, error) { b.reads.Add(1); return b.Reader.Read(p) }
func (b *countedBody) Close() error               { return nil }

func TestTraceAdmissionRejectsBeforeReadingAndNeverAcknowledgesFailedWrite(t *testing.T) {
	store, _, _, token := registeredAgentFixture(t, "bounded ingest")
	entered := make(chan struct{})
	release := make(chan struct{})
	traces := gatedTraceStore{TraceStore: &recordingTraceStore{}, insert: func(ctx context.Context, _ string, _ []TraceSpan) error {
		close(entered)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return errors.New("database unavailable")
		}
	}}
	c := DefaultTraceIngestConfig()
	c.Concurrency = 1
	c.PerOwner = 1
	h, err := NewHTTPHandler(store, HTTPConfig{Auth: testAgentAuthConfig(), Engine: nil, Traces: traces, Memory: nil, Workers: DefaultEvolutionWorkerConfig(), Ingestion: c})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	body := mustProtoMarshal(t, testOTLPRequest(false))
	request := func() *http.Request {
		r := httptest.NewRequest("POST", "/v1/otlp/v1/traces", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/x-protobuf")
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}
	out := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(out, request()) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("write not entered")
	}
	second := request()
	spy := &countedBody{Reader: bytes.NewReader(body)}
	second.Body = spy
	rejected := httptest.NewRecorder()
	h.ServeHTTP(rejected, second)
	if rejected.Code != 429 || rejected.Header().Get("Retry-After") == "" || spy.reads.Load() != 0 {
		t.Fatalf("overload code=%d reads=%d", rejected.Code, spy.reads.Load())
	}
	select {
	case <-done:
		t.Fatal("acknowledged before storage returned")
	default:
	}
	close(release)
	<-done
	if out.Code != 503 {
		t.Fatalf("failed storage acknowledged HTTP %d", out.Code)
	}
}

func TestTraceAdmissionOwnerSlotsReleasedAndBounded(t *testing.T) {
	c := DefaultTraceIngestConfig()
	c.Concurrency = 2
	c.PerOwner = 1
	a := &traceAdmission{config: c, owners: make(map[string]int)}
	if !a.acquire("a") || a.acquire("a") || !a.acquire("b") || a.acquire("c") {
		t.Fatal("global or owner limit bypassed")
	}
	a.release("a")
	if !a.acquire("c") {
		t.Fatal("slot leaked")
	}
	a.release("b")
	a.release("c")
	if a.active != 0 || len(a.owners) != 0 {
		t.Fatal("owner counters retained")
	}
}

func TestTraceAdmissionSpanLimitAndCancellation(t *testing.T) {
	store, _, _, token := registeredAgentFixture(t, "limited batch")
	var calls atomic.Int64
	traces := gatedTraceStore{TraceStore: &recordingTraceStore{}, insert: func(ctx context.Context, _ string, _ []TraceSpan) error { calls.Add(1); <-ctx.Done(); return ctx.Err() }}
	c := DefaultTraceIngestConfig()
	c.MaxSpans = 1
	h, err := NewHTTPHandler(store, HTTPConfig{Auth: testAgentAuthConfig(), Engine: nil, Traces: traces, Memory: nil, Workers: DefaultEvolutionWorkerConfig(), Ingestion: c})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	payload := testOTLPRequest(false)
	payload.ResourceSpans[0].ScopeSpans[0].Spans = append(payload.ResourceSpans[0].ScopeSpans[0].Spans, payload.ResourceSpans[0].ScopeSpans[0].Spans[0])
	r := httptest.NewRequest("POST", "/v1/otlp/v1/traces", bytes.NewReader(mustProtoMarshal(t, payload)))
	r.Header.Set("Content-Type", "application/x-protobuf")
	r.Header.Set("Authorization", "Bearer "+token)
	out := httptest.NewRecorder()
	h.ServeHTTP(out, r)
	if out.Code != 413 || calls.Load() != 0 {
		t.Fatalf("oversize batch written: %d calls=%d", out.Code, calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = httptest.NewRequest("POST", "/v1/otlp/v1/traces", bytes.NewReader(mustProtoMarshal(t, testOTLPRequest(false)))).WithContext(ctx)
	r.Header.Set("Content-Type", "application/x-protobuf")
	r.Header.Set("Authorization", "Bearer "+token)
	out = httptest.NewRecorder()
	h.ServeHTTP(out, r)
	if out.Code >= 200 && out.Code < 300 {
		t.Fatal("cancelled request acknowledged")
	}
}
