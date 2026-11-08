package control

import (
	"bytes"
	"context"
	"encoding/binary"
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

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type capacityRequest struct {
	Index   int     `json:"index"`
	Sent    float64 `json:"sent"`
	Ack     float64 `json:"ack"`
	Visible float64 `json:"visible"`
	Status  int     `json:"status"`
}

// Open-loop attempt schedule with bounded clients. A full generator is counted
// separately from server rejection; neither is reported as achieved throughput.
func TestTraceCollectorCapacity(t *testing.T) {
	if os.Getenv("CATENA_TRACE_CAPACITY") != "1" {
		t.Skip("opt-in Collector capacity staircase")
	}
	rps, e := strconv.Atoi(os.Getenv("CATENA_CAPACITY_RPS"))
	if e != nil || rps < 1 {
		t.Fatal("CATENA_CAPACITY_RPS")
	}
	seconds := 60
	prefix := os.Getenv("CATENA_CAPACITY_RUN")
	batch := 100
	tenants := 20
	reader, e := OpenOTelTraceStore(context.Background(), os.Getenv("CATENA_CAPACITY_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	identity, _ := isolatedQueuePostgres(t)
	identity.db.SetMaxOpenConns(16)
	for _, phase := range []string{"warm", "measure"} {
		for i := 0; i < tenants; i++ {
			owner := fmt.Sprintf("%s-%s-owner-%03d", prefix, phase, i)
			id := i + 1
			if phase == "measure" {
				id += tenants
			}
			_, e = identity.UpsertUser(context.Background(), User{ID: owner, GitHubID: int64(id), Login: owner, CreatedAt: time.Now(), UpdatedAt: time.Now()})
			if e != nil {
				t.Fatal(e)
			}
			e = identity.CreateAgentWithAPIToken(context.Background(), RegisteredAgent{ID: owner + "-agent", OwnerUserID: owner, DisplayName: owner, CreatedAt: time.Now(), UpdatedAt: time.Now()}, APIToken{ID: owner, UserID: owner, AgentID: owner + "-agent", TokenHash: sessionTokenHash("catena_agent_cap_" + owner), CreatedAt: time.Now()})
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	endpoints := strings.Split(os.Getenv("CATENA_CAPACITY_GATEWAYS"), ",")
	servers := []*httptest.Server{}
	c := DefaultTraceIngestConfig()
	c.Concurrency = 16
	c.PerOwner = 2
	for _, endpoint := range endpoints {
		sink, e := NewCollectorTraceSink(endpoint)
		if e != nil {
			t.Fatal(e)
		}
		h, e := NewHTTPHandler(identity, HTTPConfig{Auth: testAgentAuthConfig(), Ingress: sink, Ingestion: c})
		if e != nil {
			t.Fatal(e)
		}
		defer h.Close()
		s := httptest.NewServer(h)
		defer s.Close()
		servers = append(servers, s)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{MaxIdleConns: 256, MaxIdleConnsPerHost: 128}}
	defer client.CloseIdleConnections()
	// Same 1KiB input + 2KiB output structure as the A/B test. Only the fixed-size
	// TraceId bytes change, avoiding gigabytes of prebuilt packets at 20k Span/s.
	sentinel := []byte{0xfa, 0x91, 0x08, 0xb7, 0x63, 0xd2, 0x4e, 0xa9, 0x85, 0x70, 0x11, 0x3c, 0xf2, 0x6d, 0x89, 0xe0}
	p := testOTLPRequest(false)
	base := p.ResourceSpans[0].ScopeSpans[0].Spans[0]
	base.Attributes = append(base.Attributes, stringAttribute("input.value", strings.Repeat("i", 1024)), stringAttribute("output.value", strings.Repeat("o", 2048)))
	base.StartTimeUnixNano = 1785800000000000000
	base.EndTimeUnixNano = 1785800000001000000
	p.ResourceSpans[0].ScopeSpans[0].Spans = nil
	for i := 0; i < batch; i++ {
		s := proto.Clone(base).(*tracev1.Span)
		s.TraceId = sentinel
		s.SpanId = make([]byte, 8)
		binary.BigEndian.PutUint64(s.SpanId, uint64(i+1))
		if i == 0 {
			s.ParentSpanId = nil
		} else {
			s.ParentSpanId = make([]byte, 8)
			binary.BigEndian.PutUint64(s.ParentSpanId, 1)
		}
		p.ResourceSpans[0].ScopeSpans[0].Spans = append(p.ResourceSpans[0].ScopeSpans[0].Spans, s)
	}
	template, e := proto.Marshal(p)
	if e != nil || bytes.Count(template, sentinel) != batch {
		t.Fatal("invalid payload template", e)
	}
	send := func(phase string, i int) (int, float64, float64) {
		owner := fmt.Sprintf("%s-%s-owner-%03d", prefix, phase, i%tenants)
		id := make([]byte, 16)
		binary.BigEndian.PutUint64(id[8:], uint64(i+1))
		body := bytes.ReplaceAll(template, sentinel, id)
		req, e := http.NewRequest("POST", servers[i%len(servers)].URL+"/v1/otlp/v1/traces", bytes.NewReader(body))
		if e != nil {
			t.Error(e)
			return 0, 0, 0
		}
		req.Header.Set("Content-Type", "application/x-protobuf")
		req.Header.Set("Authorization", "Bearer catena_agent_cap_"+owner)
		sent := abNow()
		res, e := client.Do(req)
		if e != nil {
			return 0, sent, abNow()
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode, sent, abNow()
	}
	// Warm up with 100 baseline-size batches before the timed pressure tier.
	for i := 0; i < 100; i++ {
		if status, _, _ := send("warm", i); status != 200 {
			t.Fatalf("warmup HTTP %d", status)
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	count := seconds * rps
	events := make([]capacityRequest, count)
	var mu sync.Mutex
	halt := make(chan struct{})
	var observer sync.WaitGroup
	samples := []map[string]any{}
	observationErrors := 0
	observer.Add(1)
	go func() {
		defer observer.Done()
		for {
			select {
			case <-halt:
				return
			default:
			}
			begin := abNow()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			rows, err := reader.conn.Query(ctx, "SELECT TraceId, count() FROM otel_traces FINAL WHERE startsWith(CatenaOwnerId, ?) GROUP BY TraceId", prefix+"-measure-owner-")
			visible := []int{}
			logical := uint64(0)
			if err == nil {
				for rows.Next() {
					var id string
					var n uint64
					if e = rows.Scan(&id, &n); e != nil {
						err = e
						break
					}
					logical += n
					if n == 100 {
						v, x := strconv.ParseUint(id[16:], 16, 64)
						if x == nil && v > 0 && v <= uint64(count) {
							visible = append(visible, int(v)-1)
						}
					}
				}
				if x := rows.Err(); x != nil {
					err = x
				}
				rows.Close()
			}
			cancel()
			now := abNow()
			mu.Lock()
			if err == nil {
				for _, i := range visible {
					if events[i].Sent != 0 && events[i].Visible == 0 {
						events[i].Visible = now
					}
				}
				samples = append(samples, map[string]any{"at": now, "query_seconds": now - begin, "logical_spans": logical})
			} else {
				observationErrors++
			}
			mu.Unlock()
			select {
			case <-halt:
				return
			case <-time.After(200 * time.Millisecond):
			}
		}
	}()
	start := abNow()
	var wg sync.WaitGroup
	slots := make(chan struct{}, 128)
	visited, skipped := 0, 0
	for i := 0; i < count; i++ {
		target := start + float64(i)/float64(rps)
		if wait := time.Duration((target - abNow()) * 1e9); wait > 0 {
			time.Sleep(wait)
		}
		if abNow() >= start+float64(seconds) {
			break
		}
		visited++
		select {
		case slots <- struct{}{}:
		default:
			skipped++
			continue
		}
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			status, sent, ack := send("measure", i)
			mu.Lock()
			old := events[i].Visible
			events[i] = capacityRequest{Index: i, Sent: sent, Ack: ack, Visible: old, Status: status}
			mu.Unlock()
		}()
	}
	wg.Wait()
	responsesDone := abNow()
	deadline := time.Now().Add(120 * time.Second)
	accepted, completed := 0, 0
	for {
		mu.Lock()
		accepted, completed = 0, 0
		for _, v := range events {
			if v.Status == 200 {
				accepted++
				if v.Visible > 0 {
					completed++
				}
			}
		}
		mu.Unlock()
		if accepted == completed {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	close(halt)
	observer.Wait()
	finish := abNow()
	// Check full identities for every acknowledged request, not all configured
	// offers: generator/server rejections never masquerade as stored data loss.
	expected := map[string]int{}
	statuses := map[int]int{}
	ackLat, e2e := []float64{}, []float64{}
	for i, v := range events {
		if v.Sent == 0 {
			continue
		}
		statuses[v.Status]++
		if v.Status == 200 {
			expected[abID(i)] = i
			ackLat = append(ackLat, (v.Ack-v.Sent)*1000)
			if v.Visible > 0 {
				e2e = append(e2e, (max(v.Ack, v.Visible)-v.Sent)*1000)
			}
		}
	}
	rows, e := reader.conn.Query(context.Background(), "SELECT CatenaOwnerId, TraceId, groupArray(SpanId) FROM otel_traces FINAL WHERE startsWith(CatenaOwnerId, ?) GROUP BY CatenaOwnerId, TraceId", prefix+"-measure-owner-")
	if e != nil {
		t.Fatal(e)
	}
	ackedSeen, unackedStored := 0, 0
	for rows.Next() {
		var owner, id string
		var ids []string
		if e = rows.Scan(&owner, &id, &ids); e != nil {
			t.Fatal(e)
		}
		v, x := strconv.ParseUint(id[16:], 16, 64)
		if x != nil || v < 1 || v > uint64(count) {
			t.Fatal("unexpected trace")
		}
		i := int(v) - 1
		if owner != fmt.Sprintf("%s-measure-owner-%03d", prefix, i%tenants) || len(ids) != 100 {
			t.Fatal("identity/count mismatch")
		}
		sort.Strings(ids)
		for k, id := range ids {
			if id != fmt.Sprintf("%016x", k+1) {
				t.Fatal("span identity mismatch")
			}
		}
		if _, ok := expected[id]; ok {
			ackedSeen++
		} else {
			unackedStored += 100
		}
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	rows.Close()
	visibleWindow := uint64(0)
	sampleAt := start
	for _, s := range samples {
		at := s["at"].(float64)
		if at >= start && at <= start+60 {
			visibleWindow = s["logical_spans"].(uint64)
			sampleAt = at
		}
	}
	result := map[string]any{"run": prefix, "target_spans_per_second": rps * 100, "seconds": 60, "span_bytes_including_envelope": float64(len(template)) / 100, "configured_requests": count, "schedule_visited": visited, "schedule_missed_deadline": count - visited, "generator_inflight_rejections": skipped, "attempted_requests": visited - skipped, "http_statuses": statuses, "acknowledged_spans": accepted * 100, "verified_acknowledged_spans": ackedSeen * 100, "visible_completed_requests": completed, "unacknowledged_but_stored_spans": unackedStored, "started": start, "responses_done": responsesDone, "finished": finish, "last_in_window_observation_seconds": sampleAt - start, "visible_spans_in_window": visibleWindow, "sampled_window_stored_spans_per_second": float64(visibleWindow) / max(sampleAt-start, .001), "drain_after_offer_seconds": max(0, finish-start-60), "effective_acknowledged_stored_spans_per_second": float64(ackedSeen*100) / (finish - start), "ack_p95_ms": abQuantile(ackLat, .95), "e2e_p95_ms": abQuantile(e2e, .95), "e2e_p99_ms": abQuantile(e2e, .99), "observation_query_errors": observationErrors, "samples": samples, "events": events, "ingress_count": len(servers), "ingress_concurrency_per_process": 16, "per_owner_per_process": 2, "client_inflight_limit": 128}
	confirmedWindow := 0
	for _, v := range events {
		if v.Status == 200 && v.Visible > 0 && max(v.Ack, v.Visible) <= start+60 {
			confirmedWindow += 100
		}
	}
	result["confirmed_visible_spans_within_60s"] = confirmedWindow
	result["effective_stored_within_60s_per_second"] = float64(confirmedWindow) / 60
	data, e := json.Marshal(result)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(os.Getenv("CATENA_CAPACITY_RESULT"), data, 0600); e != nil {
		t.Fatal(e)
	}
	t.Logf("target=%d actual attempts=%d/%d; ACK=%d verified=%d; sampled stored %.2f Span/s; P95 %.2fms", rps*100, visited-skipped, count, accepted*100, ackedSeen*100, result["sampled_window_stored_spans_per_second"], result["e2e_p95_ms"])
	if ackedSeen != accepted || completed != accepted {
		t.Fatal("acknowledged data not fully observed/verified")
	}
}
