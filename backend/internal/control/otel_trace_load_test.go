package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// Opt-in real data-plane study. Does not use or mutate PostgreSQL/RSI.
// Inputs and outputs are deliberately explicit so skipped tests never count
// as readiness and a failed request cannot be hidden by the load generator.
func TestOTelDataPlaneLoad(t *testing.T) {
	if os.Getenv("CATENA_OTEL_LOAD") != "1" {
		t.Skip("set CATENA_OTEL_LOAD=1 for the real Collector/Kafka/ClickHouse study")
	}
	prefix := os.Getenv("CATENA_OTEL_LOAD_RUN")
	if prefix == "" {
		t.Fatal("CATENA_OTEL_LOAD_RUN must identify a fresh dataset")
	}
	integer := func(name string, fallback int) int {
		v := os.Getenv(name)
		if v == "" {
			return fallback
		}
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 {
			t.Fatalf("invalid %s", name)
		}
		return n
	}
	seconds := integer("CATENA_OTEL_LOAD_SECONDS", 60)
	qps := integer("CATENA_OTEL_LOAD_RPS", 10)
	batchSize := integer("CATENA_OTEL_LOAD_BATCH", 100)
	tenants := integer("CATENA_OTEL_LOAD_TENANTS", 20)
	retries := 0
	if os.Getenv("CATENA_OTEL_LOAD_RETRIES") != "" {
		retries = integer("CATENA_OTEL_LOAD_RETRIES", 0)
	}
	requests := seconds * qps
	expected := requests * batchSize
	ctx := context.Background()
	if os.Getenv("CATENA_OTEL_LOAD_MODE") == "verify" {
		reader := otelIntegrationStore(t)
		start := time.Now()
		otelLoadWait(t, reader, prefix, uint64(expected))
		otelVerifyIdentities(t, reader, prefix, requests, batchSize, tenants)
		otelLoadResult(t, map[string]any{"run": prefix, "mode": "verify", "expected_unique_spans": expected, "recovery_wait_seconds": time.Since(start).Seconds()})
		return
	}
	endpoints := strings.Split(os.Getenv("CATENA_OTEL_TEST_GATEWAYS"), ",")
	if endpoints[0] == "" {
		t.Fatal("CATENA_OTEL_TEST_GATEWAYS is required")
	}
	var memory Store = NewMemoryStore()
	authBackend := "memory-fixture"
	if os.Getenv("BARENA_TEST_DATABASE_URL") != "" {
		memory, _ = isolatedQueuePostgres(t)
		authBackend = "postgresql"
	}
	for i := 0; i < tenants; i++ {
		owner := fmt.Sprintf("%s-owner-%03d", prefix, i)
		_, e := memory.UpsertUser(ctx, User{ID: owner, GitHubID: int64(i + 1), Login: owner, CreatedAt: time.Now(), UpdatedAt: time.Now()})
		if e != nil {
			t.Fatal(e)
		}
		e = memory.CreateAgentWithAPIToken(ctx, RegisteredAgent{ID: fmt.Sprintf("%s-agent-%03d", prefix, i), OwnerUserID: owner, DisplayName: owner, CreatedAt: time.Now(), UpdatedAt: time.Now()}, APIToken{ID: owner, UserID: owner, AgentID: fmt.Sprintf("%s-agent-%03d", prefix, i), TokenHash: sessionTokenHash("catena_agent_fixture_" + owner), CreatedAt: time.Now()})
		if e != nil {
			t.Fatal(e)
		}
	}
	servers := []*httptest.Server{}
	ingestion := DefaultTraceIngestConfig()
	ingestion.Concurrency = 16
	for _, endpoint := range endpoints {
		sink, e := NewCollectorTraceSink(endpoint)
		if e != nil {
			t.Fatal(e)
		}
		h, e := NewHTTPHandler(memory, HTTPConfig{Auth: testAgentAuthConfig(), Ingress: sink, Ingestion: ingestion})
		if e != nil {
			t.Fatal(e)
		}
		defer h.Close()
		server := httptest.NewServer(h)
		defer server.Close()
		servers = append(servers, server)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	var mu sync.Mutex
	latencies := []float64{}
	statuses := map[int]int{}
	attemptStatuses := map[int]int{}
	starts := map[string]time.Time{}
	errors := []string{}
	start := time.Now()
	var wg sync.WaitGroup
	slots := make(chan struct{}, 32)
	ticker := time.NewTicker(time.Second / time.Duration(qps))
	defer ticker.Stop()
	for i := 0; i < requests; i++ {
		<-ticker.C
		i := i
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			owner := fmt.Sprintf("%s-owner-%03d", prefix, i%tenants)
			digest := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", prefix, i)))
			traceID := hex.EncodeToString(digest[:16])
			now := time.Now().UTC()
			spans := make([]TraceSpan, batchSize)
			for k := range spans {
				spans[k] = TraceSpan{TraceID: traceID, SpanID: fmt.Sprintf("%016x", k+1), Name: "agent.turn", StartTime: now, EndTime: now.Add(time.Millisecond), Attributes: map[string]any{"gen_ai.conversation.id": owner, "gen_ai.input.messages": "synthetic benchmark input", "catena.owner.id": "spoofed-owner"}, ResourceAttributes: map[string]any{"service.name": "catena-trace-study", "catena.owner.id": "spoofed-owner", "catena.agent.id": "spoofed-agent"}}
			}
			wire, e := collectorTraceRequest("spoofed-owner", spans)
			if e != nil {
				t.Error(e)
				return
			}
			body, e := proto.Marshal(wire)
			if e != nil {
				t.Error(e)
				return
			}
			sent := time.Now()
			status := 0
			batchContext, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			for attempt := 0; attempt <= retries; attempt++ {
				req, requestErr := http.NewRequestWithContext(batchContext, "POST", servers[i%len(servers)].URL+"/v1/otlp/v1/traces", bytes.NewReader(body))
				if requestErr != nil {
					t.Error(requestErr)
					return
				}
				req.Header.Set("Content-Type", "application/x-protobuf")
				req.Header.Set("Authorization", "Bearer catena_agent_fixture_"+owner)
				response, sendErr := client.Do(req)
				e = sendErr
				status = 0
				if sendErr == nil {
					status = response.StatusCode
					_, _ = io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}
				mu.Lock()
				attemptStatuses[status]++
				mu.Unlock()
				if status == 200 || attempt == retries || batchContext.Err() != nil || (sendErr == nil && status != 429 && status != 503) {
					break
				}
				select {
				case <-batchContext.Done():
				case <-time.After(min(time.Duration(1<<min(attempt, 3))*250*time.Millisecond, 2*time.Second)):
				}
			}
			elapsed := time.Since(sent).Seconds()
			mu.Lock()
			defer mu.Unlock()
			latencies = append(latencies, elapsed)
			statuses[status]++
			starts[traceID] = sent
			if e != nil {
				errors = append(errors, e.Error())
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start).Seconds()
	result := map[string]any{"run": prefix, "gateways": len(servers), "tenants": tenants, "batch_size": batchSize, "offered_rps": qps, "scheduled_seconds": seconds, "requests": requests, "expected_unique_spans": expected, "elapsed_seconds": elapsed, "acknowledged_spans_per_second": float64(statuses[200]*batchSize) / elapsed, "http_statuses": statuses, "transport_errors": errors, "ingress_kafka_ack_p95_seconds": otelLoadQuantile(latencies, .95), "ingress_kafka_ack_p99_seconds": otelLoadQuantile(latencies, .99)}
	result["auth_backend"] = authBackend
	result["retry_limit"] = retries
	result["http_attempt_statuses"] = attemptStatuses
	result["ingress_concurrency_per_process"] = ingestion.Concurrency
	if statuses[200] != requests {
		otelLoadResult(t, result)
		t.Fatalf("accepted %d/%d requests", statuses[200], requests)
	}
	if os.Getenv("CATENA_OTEL_LOAD_MODE") != "enqueue" {
		reader := otelIntegrationStore(t)
		otelLoadWait(t, reader, prefix, uint64(expected))
		otelVerifyIdentities(t, reader, prefix, requests, batchSize, tenants)
		rows, e := reader.conn.Query(ctx, "SELECT TraceId, count(), max(CatenaIngestedAt) FROM otel_traces FINAL WHERE startsWith(CatenaOwnerId, ?) GROUP BY TraceId", prefix+"-owner-")
		if e != nil {
			t.Fatal(e)
		}
		storage := []float64{}
		for rows.Next() {
			var id string
			var n uint64
			var ingested time.Time
			if e = rows.Scan(&id, &n, &ingested); e != nil {
				t.Fatal(e)
			}
			if n != uint64(batchSize) {
				t.Errorf("trace %s has %d spans", id, n)
			}
			storage = append(storage, ingested.Sub(starts[id]).Seconds())
			if _, ok := starts[id]; !ok {
				t.Fatalf("unexpected trace identity %s", id)
			}
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		rows.Close()
		if len(storage) != requests {
			t.Fatal("missing/extra traces", len(storage), requests)
		}
		result["storage_p95_seconds"] = otelLoadQuantile(storage, .95)
		result["storage_p99_seconds"] = otelLoadQuantile(storage, .99)
		result["verified_unique_spans"] = expected
		query := []float64{}
		for i := 0; i < 100; i++ {
			owner := fmt.Sprintf("%s-owner-%03d", prefix, i%tenants)
			begin := time.Now()
			traces, e := reader.ListTraces(ctx, owner, 100)
			if e != nil || len(traces) == 0 {
				t.Fatal("owner query", e, len(traces))
			}
			for _, tr := range traces {
				if tr.AgentID != fmt.Sprintf("%s-agent-%03d", prefix, i%tenants) {
					t.Fatal("cross-owner query contamination")
				}
			}
			query = append(query, time.Since(begin).Seconds())
		}
		result["owner_list_p95_seconds"] = otelLoadQuantile(query, .95)
		result["owner_list_p99_seconds"] = otelLoadQuantile(query, .99)
	}
	otelLoadResult(t, result)
}
func otelLoadWait(t *testing.T, s *ClickHouseTraceStore, prefix string, expected uint64) {
	t.Helper()
	end := time.Now().Add(90 * time.Second)
	var count uint64
	var e error
	for time.Now().Before(end) {
		e = s.conn.QueryRow(context.Background(), "SELECT count() FROM otel_traces FINAL WHERE startsWith(CatenaOwnerId, ?)", prefix+"-owner-").Scan(&count)
		if e == nil && count == expected {
			return
		}
		if count > expected {
			t.Fatalf("extra logical spans %d > %d", count, expected)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("logical spans %d != %d: %v", count, expected, e)
}

func otelVerifyIdentities(t *testing.T, s *ClickHouseTraceStore, prefix string, requests, batch, tenants int) {
	t.Helper()
	expected := make(map[string]string, requests)
	for i := 0; i < requests; i++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", prefix, i)))
		expected[hex.EncodeToString(digest[:16])] = fmt.Sprintf("%s-owner-%03d", prefix, i%tenants)
	}
	rows, e := s.conn.Query(context.Background(), "SELECT CatenaOwnerId, TraceId, groupArray(SpanId) FROM otel_traces FINAL WHERE startsWith(CatenaOwnerId, ?) GROUP BY CatenaOwnerId, TraceId", prefix+"-owner-")
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var owner, id string
		var spans []string
		if e = rows.Scan(&owner, &id, &spans); e != nil {
			t.Fatal(e)
		}
		if expected[id] != owner || len(spans) != batch {
			t.Fatalf("identity mismatch owner=%s trace=%s spans=%d", owner, id, len(spans))
		}
		sort.Strings(spans)
		for i, v := range spans {
			if v != fmt.Sprintf("%016x", i+1) {
				t.Fatalf("missing/extra span identity %s/%s/%s", owner, id, v)
			}
		}
		delete(expected, id)
		seen++
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	if seen != requests || len(expected) != 0 {
		t.Fatalf("trace identity mismatch seen=%d missing=%d", seen, len(expected))
	}
}
func otelLoadQuantile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[int(float64(len(v)-1)*p)]
}
func otelLoadResult(t *testing.T, v map[string]any) {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(string(b))
	if p := os.Getenv("CATENA_OTEL_LOAD_RESULTS"); p != "" {
		f, e := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		if _, e = f.Write(append(b, '\n')); e != nil {
			t.Fatal(e)
		}
	}
}
