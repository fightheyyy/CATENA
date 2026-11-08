package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func otelIntegrationStore(t *testing.T) *ClickHouseTraceStore {
	t.Helper()
	dsn := os.Getenv("CATENA_OTEL_TEST_DSN")
	if dsn == "" {
		t.Skip("CATENA_OTEL_TEST_DSN is not configured")
	}
	s, e := OpenOTelTraceStore(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func otelIntegrationSink(t *testing.T) *CollectorTraceSink {
	t.Helper()
	endpoint := strings.Split(os.Getenv("CATENA_OTEL_TEST_GATEWAYS"), ",")[0]
	if endpoint == "" {
		t.Fatal("CATENA_OTEL_TEST_GATEWAYS is required")
	}
	s, e := NewCollectorTraceSink(endpoint)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func otelWait(t *testing.T, f func() bool) {
	t.Helper()
	end := time.Now().Add(45 * time.Second)
	for time.Now().Before(end) {
		if f() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("OTel pipeline did not become consistent within 45s")
}

func TestOTelExporterGoldenParityAndReplay(t *testing.T) {
	reader := otelIntegrationStore(t)
	sink := otelIntegrationSink(t)
	legacy, e := OpenClickHouseTraceStore(context.Background(), os.Getenv("CATENA_OTEL_TEST_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	defer legacy.Close()
	for _, name := range []string{"codex", "claude"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			owner := fmt.Sprintf("otel-golden-%s-%d", name, time.Now().UnixNano())
			body, e := os.ReadFile(filepath.Join("..", "..", "..", "tap", "fixtures", "golden", name+".otlp.json"))
			if e != nil {
				t.Fatal(e)
			}
			var payloads []json.RawMessage
			if e = json.Unmarshal(body, &payloads); e != nil {
				t.Fatal(e)
			}
			all := []TraceSpan{}
			byTrace := map[string]int{}
			for _, body := range payloads {
				d, _, e := decodeOTLPRequest(body, "application/json")
				if e != nil || d.Rejected != 0 {
					t.Fatal(e, d.Rejected)
				}
				for i := range d.Spans {
					bindTraceIdentity(&d.Spans[i], owner, "agent-golden", "Golden")
					byTrace[d.Spans[i].TraceID]++
				}
				all = append(all, d.Spans...)
			}
			if e = legacy.InsertSpans(ctx, owner, all); e != nil {
				t.Fatal(e)
			}
			// Deliver every span twice through the actual Collector/Kafka/Exporter.
			for i := 0; i < 2; i++ {
				if e = sink.InsertSpans(ctx, owner, all); e != nil {
					t.Fatal(e)
				}
			}
			var queryError error
			otelWait(t, func() bool {
				traces, e := reader.ListTraces(ctx, owner, 100)
				if e != nil {
					if queryError == nil {
						t.Logf("query failed: %v", e)
					}
					queryError = e
					return false
				}
				n := 0
				for _, tr := range traces {
					n += int(tr.SpanCount)
				}
				return n == len(all)
			})
			old, e := legacy.ListTraces(ctx, owner, 100)
			if e != nil {
				t.Fatal(e)
			}
			current, e := reader.ListTraces(ctx, owner, 100)
			if e != nil {
				t.Fatal(e)
			}
			for i := range old {
				old[i].LastIngested = time.Time{}
			}
			for i := range current {
				current[i].LastIngested = time.Time{}
			}
			if !reflect.DeepEqual(old, current) {
				a, _ := json.Marshal(old)
				b, _ := json.Marshal(current)
				t.Fatalf("summary parity mismatch\nold=%s\nnew=%s", a, b)
			}
			for id, count := range byTrace {
				a, e := legacy.GetTrace(ctx, owner, id)
				if e != nil {
					t.Fatal(e)
				}
				b, e := reader.GetTrace(ctx, owner, id)
				if e != nil {
					t.Fatal(e)
				}
				if len(b.Spans) != count {
					t.Fatalf("duplicate spans: %d != %d", len(b.Spans), count)
				}
				a.Summary.LastIngested = time.Time{}
				b.Summary.LastIngested = time.Time{}
				if !reflect.DeepEqual(a, b) {
					aj, _ := json.Marshal(a)
					bj, _ := json.Marshal(b)
					t.Fatalf("detail parity mismatch\nold=%s\nnew=%s", aj, bj)
				}
			}
			agents, e := reader.ListAgents(ctx, owner, 100)
			if e != nil || len(agents) != 1 {
				t.Fatal("agent query", e, len(agents))
			}
			selected, e := reader.ListAgentTraces(ctx, owner, "agent-golden", time.Unix(0, 0), time.Now().Add(time.Hour), 100)
			if e != nil || len(selected) != len(byTrace) {
				t.Fatal("agent trace query", e, len(selected))
			}
			if _, e = reader.GetTrace(ctx, "foreign-owner", all[0].TraceID); e != ErrNotFound {
				t.Fatal("owner isolation", e)
			}
			if e = reader.InsertSpans(ctx, owner, all); e == nil {
				t.Fatal("reader accepted custom writer")
			}
			t.Logf("%s: %d unique spans / %d traces, duplicate delivery, all query methods and legacy field parity verified", name, len(all), len(byTrace))
		})
	}
}

func TestOTelAuthenticatedIngressRejectsSpoofedIdentity(t *testing.T) {
	reader := otelIntegrationStore(t)
	sink := otelIntegrationSink(t)
	memory, user, agent, token := registeredAgentFixture(t, "OTel bound agent")
	h, e := NewHTTPHandler(memory, HTTPConfig{Auth: testAgentAuthConfig(), Traces: reader, Ingress: sink})
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	span := TraceSpan{TraceID: fmt.Sprintf("%032x", time.Now().UnixNano()), SpanID: "0011223344556677", Name: "agent.turn", StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Millisecond), Attributes: map[string]any{"catena.owner.id": "victim", "catena.agent.id": "victim-agent"}, ResourceAttributes: map[string]any{"service.name": "isolation-test", "catena.owner.id": "victim", "catena.agent.id": "victim-agent"}}
	// Construct deliberately forged client OTLP, then reinsert forged values after
	// standard serialization to exercise the actual public ingress boundary.
	span.AgentID = "victim-agent"
	wire, e := collectorTraceRequest("victim", []TraceSpan{span})
	if e != nil {
		t.Fatal(e)
	}
	// Protobuf is the wire format; generated structs are not protojson.
	data, e := proto.Marshal(wire)
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"bad-key", token} {
		r := httptest.NewRequest("POST", "/v1/otlp/v1/traces", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/x-protobuf")
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if key == token && w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if key != token && w.Code != 401 {
			t.Fatal("unauthenticated payload accepted", w.Code)
		}
	}
	otelWait(t, func() bool {
		d, e := reader.GetTrace(context.Background(), user.ID, span.TraceID)
		return e == nil && len(d.Spans) == 1
	})
	d, e := reader.GetTrace(context.Background(), user.ID, span.TraceID)
	if e != nil {
		t.Fatal(e)
	}
	stored := d.Spans[0]
	if stored.AgentID != agent.ID || stored.ResourceAttributes["catena.owner.id"] != user.ID || stored.Attributes["catena.owner.id"] != user.ID || stored.Attributes["catena.agent.id"] != agent.ID {
		t.Fatal("trusted identity was overwritten")
	}
	if _, e = reader.GetTrace(context.Background(), "victim", span.TraceID); e != ErrNotFound {
		t.Fatal("forged owner received trace", e)
	}
}

func TestOTelReplayDeduplicatesBeforeMergesAcrossOwners(t *testing.T) {
	reader := otelIntegrationStore(t)
	sink := otelIntegrationSink(t)
	ctx := context.Background()
	if e := reader.conn.Exec(ctx, "SYSTEM STOP MERGES otel_traces"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := reader.conn.Exec(context.Background(), "SYSTEM START MERGES otel_traces"); e != nil {
			t.Error(e)
		}
	})
	owner := fmt.Sprintf("unmerged-replay-%d", time.Now().UnixNano())
	span := TraceSpan{TraceID: fmt.Sprintf("%032x", time.Now().UnixNano()), SpanID: "0011223344556677", Name: "agent.turn", ServiceName: "dedup-fixture", StartTime: time.Now().UTC(), EndTime: time.Now().UTC().Add(time.Millisecond), Attributes: map[string]any{}, ResourceAttributes: map[string]any{"service.name": "dedup-fixture"}}
	for i := 0; i < 3; i++ {
		if e := sink.InsertSpans(ctx, owner, []TraceSpan{span}); e != nil {
			t.Fatal(e)
		}
	}
	if e := sink.InsertSpans(ctx, owner+"-other", []TraceSpan{span}); e != nil {
		t.Fatal(e)
	}
	var physical uint64
	otelWait(t, func() bool {
		e := reader.conn.QueryRow(ctx, "SELECT count() FROM otel_traces WHERE CatenaOwnerId = ?", owner).Scan(&physical)
		return e == nil && physical >= 3
	})
	for _, tenant := range []string{owner, owner + "-other"} {
		otelWait(t, func() bool {
			d, e := reader.GetTrace(ctx, tenant, span.TraceID)
			return e == nil && len(d.Spans) == 1 && d.Summary.SpanCount == 1
		})
		traces, e := reader.ListTraces(ctx, tenant, 10)
		if e != nil || len(traces) != 1 || traces[0].SpanCount != 1 {
			t.Fatal("unmerged replay aggregate", e, traces)
		}
	}
	t.Logf("background merges stopped: %d physical copies, one logical span per owner; same IDs under another owner remain separate", physical)
}
