package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type traceLoadResult struct {
	Mode           string  `json:"mode"`
	Round          int     `json:"round"`
	Spans          int     `json:"spans"`
	BatchSize      int     `json:"batch_size"`
	Clients        int     `json:"clients"`
	PayloadSHA256  string  `json:"payload_sha256"`
	PayloadBytes   int     `json:"payload_bytes"`
	Seconds        float64 `json:"seconds"`
	SpansPerSecond float64 `json:"accepted_spans_per_second"`
	Accepted       int     `json:"accepted_requests"`
	Overload       int     `json:"overload_responses"`
	Errors         int     `json:"error_responses"`
	P50            float64 `json:"accepted_request_p50_ms"`
	P95            float64 `json:"accepted_request_p95_ms"`
	P99            float64 `json:"accepted_request_p99_ms"`
	PeakHeap       uint64  `json:"peak_go_heap_bytes"`
	PeakGoroutines int     `json:"peak_go_goroutines"`
	LogicalRows    uint64  `json:"logical_rows_final"`
	PhysicalRows   uint64  `json:"physical_rows"`
	ListP95        float64 `json:"warm_list_100_p95_ms"`
	DetailP95      float64 `json:"warm_detail_p95_ms"`
}

func loadPercentile(v []float64, p float64) float64 {
	sort.Float64s(v)
	if len(v) == 0 {
		return 0
	}
	i := max(0, int(math.Ceil(float64(len(v))*p))-1)
	return v[i]
}

// Opt-in only: this writes synthetic spans to an explicitly supplied test DB.
// It never uses the production CATENA_CLICKHOUSE_DSN or private model keys.
func TestTraceBackendLoadStudy(t *testing.T) {
	if os.Getenv("CATENA_RUN_TRACE_LOAD_STUDY") != "1" {
		t.Skip("opt-in load study")
	}
	dsn := os.Getenv("CATENA_CLICKHOUSE_TEST_DSN")
	if dsn == "" {
		t.Fatal("CATENA_CLICKHOUSE_TEST_DSN required")
	}
	out := os.Getenv("CATENA_TRACE_LOAD_REPORT")
	if out == "" {
		t.Fatal("CATENA_TRACE_LOAD_REPORT required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ch, err := OpenClickHouseTraceStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	var version string
	if err = ch.conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	var results []traceLoadResult
	for round := 1; round <= 3; round++ {
		// Alternate order to reduce systematic warmup/order bias.
		modes := []string{"admission-64", "bounded-4-owner-2"}
		if round%2 == 0 {
			modes[0], modes[1] = modes[1], modes[0]
		}
		for _, mode := range modes {
			results = append(results, runTraceLoadTrial(t, ctx, ch, dsn, mode, round))
		}
	}
	report := map[string]any{"schema": "catena.trace-backend-load.v1", "measured_at": time.Now().UTC(), "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0), "clickhouse": version, "workload": "synthetic; repeated text; 500 spans/batch; 5 spans/trace; 1 KiB input and 2 KiB output; closed-loop with overload retries; no private corpus", "notes": []string{"admission-64 approximates the previous unrestricted path only at 8 clients; it is not a separate old binary", "Go heap is not process RSS; database memory and CPU are not sampled", "query timings are warm, one warmup then 10 samples; not cold-cache measurements", "no throughput target or production SLO inferred from this host", "HTTP auth uses MemoryStore; this does not benchmark PostgreSQL token lookup"}, "trials": results}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(out, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

func runTraceLoadTrial(t *testing.T, ctx context.Context, admin *ClickHouseTraceStore, dsn, mode string, round int) traceLoadResult {
	t.Helper()
	const batches = 100
	const batchSize = 500
	const clients = 8
	owner := fmt.Sprintf("backend-load-%d-%d", time.Now().UnixNano(), round)
	database := fmt.Sprintf("catena_backend_load_%d", time.Now().UnixNano())
	if err := admin.conn.Exec(ctx, "CREATE DATABASE "+database); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e := admin.conn.Exec(cleanup, "DROP DATABASE "+database+" SYNC"); e != nil {
			t.Error(e)
		}
	}()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	ch, err := OpenClickHouseTraceStore(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	store := NewMemoryStore()
	now := time.Now().UTC()
	_, err = store.UpsertUser(ctx, User{ID: owner, GitHubID: 1, Login: owner, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	token := "catena_agent_backend_test_token"
	if err = store.CreateAPIToken(ctx, APIToken{ID: "load-key", UserID: owner, TokenHash: sessionTokenHash(token), Name: "load", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	c := DefaultTraceIngestConfig()
	if mode == "admission-64" {
		c.Concurrency = 64
		c.PerOwner = 64
	}
	h, err := NewHTTPHandler(store, HTTPConfig{Auth: testAgentAuthConfig(), Engine: nil, Traces: ch, Memory: nil, Workers: DefaultEvolutionWorkerConfig(), Ingestion: c})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	srv := httptest.NewServer(h)
	defer srv.Close()
	tr := &http.Transport{MaxIdleConns: clients, MaxIdleConnsPerHost: clients}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	payloads := make([][]byte, batches)
	hash := sha256.New()
	payloadBytes := 0
	for b := 0; b < batches; b++ {
		p := testOTLPRequest(false)
		template := p.ResourceSpans[0].ScopeSpans[0].Spans[0]
		template.Attributes = append(template.Attributes, stringAttribute("input.value", strings.Repeat("i", 1024)), stringAttribute("output.value", strings.Repeat("o", 2048)))
		template.StartTimeUnixNano = 1785800000000000000
		p.ResourceSpans[0].ScopeSpans[0].Spans = nil
		// Clone via the generated concrete type to keep identifiers deterministic.
		for i := 0; i < batchSize; i++ {
			s := proto.Clone(template).(*tracev1.Span)
			index := uint64(b*batchSize + i)
			s.TraceId = make([]byte, 16)
			binary.BigEndian.PutUint64(s.TraceId[8:], index/5+1)
			s.SpanId = make([]byte, 8)
			binary.BigEndian.PutUint64(s.SpanId, index+1)
			s.StartTimeUnixNano += index * 1000000
			s.EndTimeUnixNano = s.StartTimeUnixNano + 12000000
			p.ResourceSpans[0].ScopeSpans[0].Spans = append(p.ResourceSpans[0].ScopeSpans[0].Spans, s)
		}
		payloads[b], err = proto.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		hash.Write(payloads[b])
		payloadBytes += len(payloads[b])
	}
	var accepted, overload, failures atomic.Int64
	var index atomic.Int64
	var mu sync.Mutex
	var latencies []float64
	runtime.GC()
	var peakHeap atomic.Uint64
	var peakG atomic.Int64
	sampleCtx, stop := context.WithCancel(ctx)
	sampleDone := make(chan struct{})
	go func() {
		defer close(sampleDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-sampleCtx.Done():
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				peakHeap.Store(max(peakHeap.Load(), m.HeapAlloc))
				peakG.Store(max(peakG.Load(), int64(runtime.NumGoroutine())))
			}
		}
	}()
	started := time.Now()
	var wg sync.WaitGroup
	for n := 0; n < clients; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				b := int(index.Add(1)) - 1
				if b >= batches {
					return
				}
				for retry := 0; retry < 100; retry++ {
					began := time.Now()
					req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/v1/otlp/v1/traces", bytes.NewReader(payloads[b]))
					req.Header.Set("Content-Type", "application/x-protobuf")
					req.Header.Set("Authorization", "Bearer "+token)
					res, e := client.Do(req)
					if e != nil {
						t.Logf("load transport error: %v", e)
						failures.Add(1)
						return
					}
					io.Copy(io.Discard, res.Body)
					res.Body.Close()
					if res.StatusCode == 429 {
						overload.Add(1)
						select {
						case <-ctx.Done():
							failures.Add(1)
							return
						case <-time.After(25*time.Millisecond + time.Duration(retry%7)*5*time.Millisecond):
						}
						continue
					}
					if res.StatusCode != 200 {
						t.Logf("load rejected: HTTP %d", res.StatusCode)
						failures.Add(1)
						return
					}
					accepted.Add(1)
					mu.Lock()
					latencies = append(latencies, float64(time.Since(began).Microseconds())/1000)
					mu.Unlock()
					break
				}
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(started).Seconds()
	stop()
	<-sampleDone
	r := traceLoadResult{Mode: mode, Round: round, Spans: batches * batchSize, BatchSize: batchSize, Clients: clients, PayloadSHA256: hex.EncodeToString(hash.Sum(nil)), PayloadBytes: payloadBytes, Seconds: elapsed, SpansPerSecond: float64(accepted.Load()*batchSize) / elapsed, Accepted: int(accepted.Load()), Overload: int(overload.Load()), Errors: int(failures.Load()), P50: loadPercentile(latencies, .5), P95: loadPercentile(latencies, .95), P99: loadPercentile(latencies, .99), PeakHeap: peakHeap.Load(), PeakGoroutines: int(peakG.Load())}
	if err = ch.conn.QueryRow(ctx, "SELECT count() FROM catena_spans FINAL WHERE owner_id = ?", owner).Scan(&r.LogicalRows); err != nil {
		t.Fatal(err)
	}
	if err = ch.conn.QueryRow(ctx, "SELECT count() FROM catena_spans WHERE owner_id = ?", owner).Scan(&r.PhysicalRows); err != nil {
		t.Fatal(err)
	}
	if r.Accepted != batches || r.Errors != 0 || r.LogicalRows != uint64(r.Spans) {
		t.Fatalf("ack/read count mismatch: %+v", r)
	}
	var list, detail []float64
	for n := 0; n < 11; n++ {
		start := time.Now()
		rows, e := ch.ListTraces(ctx, owner, 100)
		if e != nil || len(rows) != 100 {
			t.Fatalf("list: %d %v", len(rows), e)
		}
		ms := float64(time.Since(start).Microseconds()) / 1000
		if n > 0 {
			list = append(list, ms)
		}
		start = time.Now()
		d, e := ch.GetTrace(ctx, owner, rows[0].TraceID)
		if e != nil || len(d.Spans) != 5 {
			t.Fatalf("detail: %d %v", len(d.Spans), e)
		}
		if n > 0 {
			detail = append(detail, float64(time.Since(start).Microseconds())/1000)
		}
	}
	r.ListP95 = loadPercentile(list, .95)
	r.DetailP95 = loadPercentile(detail, .95)
	t.Logf("load %s round=%d spans/s=%.0f p95=%.1fms overload=%d", mode, round, r.SpansPerSecond, r.P95, r.Overload)
	return r
}
