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
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type abEvent struct {
	Batch    int     `json:"batch"`
	Enqueued float64 `json:"enqueued"`
	Ack      float64 `json:"ack"`
	Visible  float64 `json:"visible"`
	Attempts int     `json:"attempts"`
}

func abNow() float64 { return float64(time.Now().UnixNano()) / 1e9 }
func abQuantile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[max(0, int(float64(len(v))*p+.999999)-1)]
}
func abID(b int) string { return fmt.Sprintf("%032x", b+1) }

// Shared byte-for-byte with the archived old tree. Adapters alone choose the
// original Writer or current Collector. No model/RSI operations are involved.
func TestTraceWriterCollectorAB(t *testing.T) {
	if os.Getenv("CATENA_TRACE_AB") != "1" {
		t.Skip("opt-in old binary vs Collector study")
	}
	integer := func(k string, d int) int {
		if os.Getenv(k) == "" {
			return d
		}
		n, e := strconv.Atoi(os.Getenv(k))
		if e != nil || n < 1 {
			t.Fatal(k)
		}
		return n
	}
	seconds := integer("CATENA_AB_SECONDS", 60)
	warm := integer("CATENA_AB_WARMUP", 10)
	rps := integer("CATENA_AB_RPS", 10)
	batch := 100
	tenants := 20
	prefix := os.Getenv("CATENA_AB_RUN")
	if prefix == "" {
		t.Fatal("CATENA_AB_RUN required")
	}
	ch, e := abOpen(context.Background(), os.Getenv("CATENA_AB_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	defer ch.Close()
	store, _ := isolatedQueuePostgres(t)
	store.db.SetMaxOpenConns(16)
	for _, phase := range []string{"warm", "measure"} {
		for i := 0; i < tenants; i++ {
			owner := fmt.Sprintf("%s-%s-owner-%03d", prefix, phase, i)
			_, e = store.UpsertUser(context.Background(), User{ID: owner, GitHubID: int64((i + 1) + map[string]int{"warm": 0, "measure": tenants}[phase]), Login: owner, CreatedAt: time.Now(), UpdatedAt: time.Now()})
			if e != nil {
				t.Fatal(e)
			}
			e = store.CreateAgentWithAPIToken(context.Background(), RegisteredAgent{ID: owner + "-agent", OwnerUserID: owner, DisplayName: owner, CreatedAt: time.Now(), UpdatedAt: time.Now()}, APIToken{ID: owner, UserID: owner, AgentID: owner + "-agent", TokenHash: sessionTokenHash("catena_agent_ab_" + owner), CreatedAt: time.Now()})
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	c := DefaultTraceIngestConfig()
	c.Concurrency = 16
	c.PerOwner = 2
	h := abHandler(t, store, ch, c)
	defer h.Close()
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 64}}
	defer client.CloseIdleConnections()
	payloads := make([][]byte, max(seconds, warm)*rps)
	hash := sha256.New()
	payloadBytes := 0
	for b := range payloads {
		p := testOTLPRequest(false)
		template := p.ResourceSpans[0].ScopeSpans[0].Spans[0]
		template.Attributes = append(template.Attributes, stringAttribute("input.value", strings.Repeat("i", 1024)), stringAttribute("output.value", strings.Repeat("o", 2048)))
		template.StartTimeUnixNano = 1785800000000000000
		template.EndTimeUnixNano = 1785800000001000000
		p.ResourceSpans[0].ScopeSpans[0].Spans = nil
		for k := 0; k < batch; k++ {
			s := proto.Clone(template).(*tracev1.Span)
			s.TraceId = make([]byte, 16)
			binary.BigEndian.PutUint64(s.TraceId[8:], uint64(b+1))
			s.SpanId = make([]byte, 8)
			binary.BigEndian.PutUint64(s.SpanId, uint64(k+1))
			if k == 0 {
				s.ParentSpanId = nil
			} else {
				s.ParentSpanId = make([]byte, 8)
				binary.BigEndian.PutUint64(s.ParentSpanId, 1)
			}
			p.ResourceSpans[0].ScopeSpans[0].Spans = append(p.ResourceSpans[0].ScopeSpans[0].Spans, s)
		}
		payloads[b], e = proto.Marshal(p)
		if e != nil {
			t.Fatal(e)
		}
		if b < seconds*rps {
			hash.Write(payloads[b])
			payloadBytes += len(payloads[b])
		}
	}
	run := func(phase string, duration int) ([]abEvent, map[int]int, float64, float64, uint64) {
		count := duration * rps
		events := make([]abEvent, count)
		var mu sync.Mutex
		statuses := map[int]int{}
		halt := make(chan struct{})
		var observer sync.WaitGroup
		var heapPeak uint64
		ownerPrefix := prefix + "-" + phase + "-owner-"
		observer.Add(1)
		go func() {
			defer observer.Done()
			for {
				select {
				case <-halt:
					return
				default:
				}
				call, cancel := context.WithTimeout(context.Background(), time.Second)
				rows, err := ch.conn.Query(call, abObserveSQL, ownerPrefix)
				if err == nil {
					visible := []int{}
					for rows.Next() {
						var id string
						var n uint64
						if err = rows.Scan(&id, &n); err != nil {
							break
						}
						if n == uint64(batch) {
							v, err := strconv.ParseUint(id[16:], 16, 64)
							if err == nil && v > 0 && v <= uint64(count) {
								visible = append(visible, int(v)-1)
							}
						}
					}
					rows.Close()
					now := abNow()
					mu.Lock()
					for _, b := range visible {
						if events[b].Enqueued != 0 && events[b].Visible == 0 {
							events[b].Visible = now
						}
					}
					mu.Unlock()
				}
				cancel()
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > heapPeak {
					heapPeak = m.HeapAlloc
				}
				select {
				case <-halt:
					return
				case <-time.After(50 * time.Millisecond):
				}
			}
		}()
		start := abNow()
		if phase == "measure" && os.Getenv("CATENA_AB_MARKER") != "" {
			b, _ := json.Marshal(map[string]any{"started": start})
			if e = os.WriteFile(os.Getenv("CATENA_AB_MARKER"), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
		jobs := make(chan int, count)
		var workers sync.WaitGroup
		for j := 0; j < 32; j++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for b := range jobs {
					owner := fmt.Sprintf("%s%03d", ownerPrefix, b%tenants)
					deadline, cancel := context.WithTimeout(context.Background(), 90*time.Second)
					for attempt := 0; attempt < 32; attempt++ {
						req, err := http.NewRequestWithContext(deadline, "POST", srv.URL+"/v1/otlp/v1/traces", bytes.NewReader(payloads[b]))
						if err != nil {
							t.Error(err)
							break
						}
						req.Header.Set("Content-Type", "application/x-protobuf")
						req.Header.Set("Authorization", "Bearer catena_agent_ab_"+owner)
						response, err := client.Do(req)
						status := 0
						if err == nil {
							status = response.StatusCode
							_, _ = io.Copy(io.Discard, response.Body)
							response.Body.Close()
						}
						mu.Lock()
						events[b].Attempts++
						statuses[status]++
						if status == 200 {
							events[b].Ack = abNow()
						}
						mu.Unlock()
						if status == 200 || deadline.Err() != nil || (err == nil && status != 429 && status != 503) {
							break
						}
						select {
						case <-deadline.Done():
						case <-time.After(min(time.Duration(1<<min(attempt, 3))*250*time.Millisecond, 2*time.Second)):
						}
					}
					cancel()
				}
			}()
		}
		for b := 0; b < count; b++ {
			target := time.Unix(0, int64((start+float64(b)/float64(rps))*1e9))
			if wait := time.Until(target); wait > 0 {
				time.Sleep(wait)
			}
			mu.Lock()
			events[b] = abEvent{Batch: b, Enqueued: abNow()}
			mu.Unlock()
			jobs <- b
		}
		close(jobs)
		workers.Wait()
		limit := time.Now().Add(60 * time.Second)
		for {
			mu.Lock()
			done := 0
			for _, v := range events {
				if v.Ack > 0 && v.Visible > 0 {
					done++
				}
			}
			mu.Unlock()
			if done == count {
				break
			}
			if time.Now().After(limit) {
				close(halt)
				observer.Wait()
				t.Fatalf("%s completed %d/%d", phase, done, count)
			}
			time.Sleep(20 * time.Millisecond)
		}
		end := abNow()
		close(halt)
		observer.Wait()
		return events, statuses, start, end, heapPeak
	}
	run("warm", warm)
	time.Sleep(time.Second)
	events, statuses, start, end, heap := run("measure", seconds)
	// Enumerate every identity once after the timed workload, independently of
	// row-count polling. Same trace/span IDs exist across rounds under new owners.
	rows, e := ch.conn.Query(context.Background(), abIdentitySQL, prefix+"-measure-owner-")
	if e != nil {
		t.Fatal(e)
	}
	seen := 0
	for rows.Next() {
		var owner, id string
		var ids []string
		if e = rows.Scan(&owner, &id, &ids); e != nil {
			t.Fatal(e)
		}
		v, e := strconv.ParseUint(id[16:], 16, 64)
		if e != nil || v < 1 || v > uint64(len(events)) {
			t.Fatal("unexpected trace", id)
		}
		b := int(v) - 1
		if owner != fmt.Sprintf("%s-measure-owner-%03d", prefix, b%tenants) || len(ids) != batch {
			t.Fatal("owner/span count mismatch", owner, id, len(ids))
		}
		sort.Strings(ids)
		for k, id := range ids {
			if id != fmt.Sprintf("%016x", k+1) {
				t.Fatal("span identity mismatch")
			}
		}
		seen++
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	if seen != len(events) {
		t.Fatal("trace completeness", seen, len(events))
	}
	latencies := []float64{}
	ackLatencies := []float64{}
	last := start
	for _, v := range events {
		complete := max(v.Ack, v.Visible)
		latencies = append(latencies, (complete-v.Enqueued)*1000)
		ackLatencies = append(ackLatencies, (v.Ack-v.Enqueued)*1000)
		last = max(last, complete)
	}
	result := map[string]any{"run": prefix, "mode": os.Getenv("CATENA_AB_MODE"), "seconds": seconds, "warmup_seconds": warm, "rps": rps, "batch_size": batch, "tenants": tenants, "payload_sha256": hex.EncodeToString(hash.Sum(nil)), "payload_bytes": payloadBytes, "span_count": len(events) * batch, "verified_unique_spans": seen * batch, "measurement_started": start, "measurement_finished": end, "last_completion": last, "effective_stored_spans_per_second": float64(seen*batch) / (last - start), "e2e_p50_ms": abQuantile(latencies, .5), "e2e_p95_ms": abQuantile(latencies, .95), "e2e_p99_ms": abQuantile(latencies, .99), "ack_p95_ms": abQuantile(ackLatencies, .95), "http_attempt_statuses": statuses, "peak_go_heap_bytes": heap, "events": events}
	data, e := json.Marshal(result)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(os.Getenv("CATENA_AB_RESULT"), data, 0600); e != nil {
		t.Fatal(e)
	}
	t.Logf("%s: %d spans complete; stored %.2f Span/s, E2E P95 %.2fms", prefix, seen*batch, result["effective_stored_spans_per_second"], result["e2e_p95_ms"])
}
