package control

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestCollectorForwardingIdentityAndAcknowledgement(t *testing.T) {
	span := TraceSpan{TraceID: "00112233445566778899aabbccddeeff", SpanID: "0011223344556677", AgentID: "forged", StartTime: time.Now(), EndTime: time.Now(), Attributes: map[string]any{"catena.owner.id": "foreign", "catena.agent.id": "forged", "catena.internal.meta.v1": "forged", "count": int64(42)}, ResourceAttributes: map[string]any{"catena.owner.id": "foreign", "catena.agent.id": "forged"}}
	bindTraceIdentity(&span, "owner-a", "agent-a", "A")
	var reject bool
	var code = 200
	var calls int
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/traces" || r.Header.Get("Authorization") != "" {
			t.Error("invalid internal request")
		}
		body, _ := io.ReadAll(r.Body)
		var req collectortracev1.ExportTraceServiceRequest
		if e := proto.Unmarshal(body, &req); e != nil {
			t.Error(e)
		}

		for _, rs := range req.ResourceSpans {
			resource := keyValues(rs.Resource.Attributes)
			if resource["catena.owner.id"] != "owner-a" || resource["catena.agent.id"] != "agent-a" {
				t.Error("resource identity forged")
			}
			for _, ss := range rs.ScopeSpans {
				for _, s := range ss.Spans {
					a := keyValues(s.Attributes)
					if a["catena.owner.id"] != "owner-a" || a["catena.agent.id"] != "agent-a" || a["catena.internal.meta.v1"] == "forged" {
						t.Error("span identity forged")
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(code)
		ack := &collectortracev1.ExportTraceServiceResponse{}
		if reject {
			ack.PartialSuccess = &collectortracev1.ExportTracePartialSuccess{RejectedSpans: 1}
		}
		b, _ := proto.Marshal(ack)
		_, _ = w.Write(b)
	}))
	defer gateway.Close()
	sink, e := NewCollectorTraceSink(gateway.URL)
	if e != nil {
		t.Fatal(e)
	}
	if e = sink.InsertSpans(context.Background(), "owner-a", []TraceSpan{span}); e != nil {
		t.Fatal(e)
	}
	reject = true
	if e = sink.InsertSpans(context.Background(), "owner-a", []TraceSpan{span}); e == nil {
		t.Fatal("partial rejection acknowledged")
	}
	reject = false
	code = 503
	if e = sink.InsertSpans(context.Background(), "owner-a", []TraceSpan{span}); e == nil {
		t.Fatal("upstream failure acknowledged")
	}
	if calls != 3 {
		t.Fatal(calls)
	}
	bindTraceIdentity(&span, "owner-b", "", "")
	if span.AgentID != "" || span.ResourceAttributes["catena.agent.id"] != nil || span.Attributes["catena.agent.id"] != nil {
		t.Fatal("unbound token retained spoofed agent")
	}
}
