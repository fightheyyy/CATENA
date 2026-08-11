package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestDecodeOTLPRequestPreservesToolEvidence(t *testing.T) {
	request := testOTLPRequest(true)
	tests := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{
			name:        "protobuf",
			contentType: "application/x-protobuf",
			body:        mustProtoMarshal(t, request),
		},
		{
			name:        "json",
			contentType: "application/json; charset=utf-8",
			body:        mustProtoJSONMarshal(t, request),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded, encoding, err := decodeOTLPRequest(test.body, test.contentType)
			if err != nil {
				t.Fatal(err)
			}
			if encoding != test.name {
				t.Fatalf("encoding = %q, want %q", encoding, test.name)
			}
			if decoded.Rejected != 1 || len(decoded.Spans) != 1 {
				t.Fatalf("decoded = %+v, want one retained and one rejected span", decoded)
			}
			span := decoded.Spans[0]
			if span.TraceID != "00112233445566778899aabbccddeeff" ||
				span.SpanID != "0011223344556677" ||
				span.ServiceName != "codex" ||
				span.Model != "gpt-5.6-sol" ||
				span.Input != `{"path":"README.md"}` ||
				span.Output != "file contents" {
				t.Fatalf("unexpected converted span: %+v", span)
			}
			if len(span.Events) != 1 || span.Events[0].Name != "tool.result" ||
				span.Events[0].Attributes["tool.name"] != "read_file" {
				t.Fatalf("tool event was not preserved: %+v", span.Events)
			}
		})
	}
}

func TestSummarizeTracePreservesAgentSessionHierarchy(t *testing.T) {
	start := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	spans := []TraceSpan{
		{
			TraceID: "00112233445566778899aabbccddeeff", SpanID: "0011223344556677",
			Name: "agent.turn", ServiceName: "catena-tap-codex", StartTime: start, EndTime: start.Add(time.Second),
			Attributes: map[string]any{"agent.session.id": "session-from-root"}, ResourceAttributes: map[string]any{},
		},
		{
			AgentID: "agent-codex", TraceID: "00112233445566778899aabbccddeeff", SpanID: "8899aabbccddeeff",
			ParentSpanID: "0011223344556677", Name: "gen_ai.model.call", ServiceName: "catena-tap-codex",
			StartTime: start.Add(100 * time.Millisecond), EndTime: start.Add(900 * time.Millisecond),
			Attributes: map[string]any{}, ResourceAttributes: map[string]any{},
		},
	}
	summary := summarizeTrace(spans, start.Add(2*time.Second))
	if summary.AgentID != "agent-codex" || summary.SessionID != "session-from-root" ||
		summary.TraceID != spans[0].TraceID || summary.SpanCount != 2 {
		t.Fatalf("unexpected hierarchy summary: %+v", summary)
	}
}

func TestTraceSpanSessionIDUsesSupportedAttributesWithoutGuessing(t *testing.T) {
	span := TraceSpan{
		Attributes:         map[string]any{"gen_ai.conversation.id": " conversation-42 "},
		ResourceAttributes: map[string]any{"session.id": "resource-fallback"},
	}
	if got := traceSpanSessionID(span); got != "conversation-42" {
		t.Fatalf("unexpected Session identity %q", got)
	}
	if got := traceSpanSessionID(TraceSpan{Attributes: map[string]any{}, ResourceAttributes: map[string]any{}}); got != "" {
		t.Fatalf("missing Session identity must stay empty, got %q", got)
	}
}

func TestDecodeOTLPJSONAcceptsCatenaTapProtoJSONIDs(t *testing.T) {
	body := []byte(`{
  "resourceSpans": [{
    "resource": {"attributes": [{"key":"service.name","value":{"stringValue":"catena-tap-codex"}}]},
    "scopeSpans": [{
      "scope": {"name":"catena.tap","version":"0.1.0"},
      "spans": [{
        "traceId":"ABEiM0RVZneImaq7zN3u/w==",
        "spanId":"ABEiM0RVZnc=",
        "name":"agent.turn",
        "kind":1,
        "startTimeUnixNano":"1785800000000000000",
        "endTimeUnixNano":"1785800001000000000",
        "attributes":[{"key":"input.value","value":{"stringValue":"hello"}}],
        "status":{"code":1}
      }]
    }]
  }]
}`)
	decoded, encoding, err := decodeOTLPRequest(body, "application/json")
	if err != nil {
		t.Fatal(err)
	}
	if encoding != "json" || len(decoded.Spans) != 1 {
		t.Fatalf("encoding/spans = %q/%d", encoding, len(decoded.Spans))
	}
	span := decoded.Spans[0]
	if span.TraceID != "00112233445566778899aabbccddeeff" ||
		span.SpanID != "0011223344556677" || span.Input != "hello" {
		t.Fatalf("unexpected Catena Tap span: %+v", span)
	}
}

func TestConvertOTLPSpanRecognizesCodexHistoryEvidenceAttributes(t *testing.T) {
	start := uint64(1_785_800_000_000_000_000)
	tests := []struct {
		name       string
		attributes []*commonv1.KeyValue
		input      string
		output     string
	}{
		{
			name: "turn root",
			attributes: []*commonv1.KeyValue{
				stringAttribute("input.value", `{"type":"chat_messages","value":[]}`),
				stringAttribute("output.value", "completed response"),
			},
			input:  `{"type":"chat_messages","value":[]}`,
			output: "completed response",
		},
		{
			name: "tool child",
			attributes: []*commonv1.KeyValue{
				stringAttribute("gen_ai.tool.call.arguments", `{"cmd":"pwd"}`),
				stringAttribute("gen_ai.tool.call.result", "/workspace"),
			},
			input:  `{"cmd":"pwd"}`,
			output: "/workspace",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			span, ok := convertOTLPSpan(&tracev1.Span{
				TraceId:           []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
				SpanId:            []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77},
				Name:              "codex.history",
				StartTimeUnixNano: start,
				EndTimeUnixNano:   start + 1,
				Attributes:        test.attributes,
			}, nil, 0, "", "", "codex", "catena-test", "1.0")
			if !ok {
				t.Fatal("span was rejected")
			}
			if span.Input != test.input || span.Output != test.output {
				t.Fatalf("input/output = %q/%q, want %q/%q", span.Input, span.Output, test.input, test.output)
			}
		})
	}
}

func TestOTLPHandlerAuthenticatesAndStoresByOwner(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now().UTC()
	user, err := store.UpsertUser(context.Background(), User{
		ID:          "user-trace-owner",
		GitHubID:    91234,
		Login:       "trace-owner",
		DisplayName: "Trace Owner",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := "barena_pat_trace_test"
	if err := store.CreateAPIToken(context.Background(), APIToken{
		ID:        "pat-trace-test",
		TokenHash: sessionTokenHash(token),
		UserID:    user.ID,
		Name:      "OTLP test",
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	traces := &recordingTraceStore{}
	server := &HTTPServer{
		store:  store,
		traces: traces,
		auth: AuthConfig{
			GitHubClientID:     "client",
			GitHubClientSecret: "secret",
			RedirectURL:        "https://catena.example/v1/auth/github/callback",
		}.normalized(),
	}
	body := mustProtoMarshal(t, testOTLPRequest(false))
	request := httptest.NewRequest(http.MethodPost, "/v1/otlp/v1/traces", nil)
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()

	server.ingestOTLPTraces(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if traces.ownerID != user.ID || len(traces.spans) != 1 {
		t.Fatalf("stored owner/spans = %q/%d", traces.ownerID, len(traces.spans))
	}
	response := &collectortracev1.ExportTraceServiceResponse{}
	if err := proto.Unmarshal(recorder.Body.Bytes(), response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.GetPartialSuccess() != nil {
		t.Fatalf("unexpected partial success: %+v", response.GetPartialSuccess())
	}
}

func TestAgentTraceHandlerCanonicalizesCodexAliasAndPreservesTraceSource(t *testing.T) {
	now := time.Now().UTC()
	traces := &recordingTraceStore{agentTraces: []TraceSummary{{
		TraceID:     "00112233445566778899aabbccddeeff",
		RootName:    "codex.turn",
		ServiceName: "Codex Desktop",
		StartTime:   now.Add(-time.Minute),
		EndTime:     now,
		SpanCount:   1,
	}}}
	server := &HTTPServer{store: NewMemoryStore(), traces: traces}
	request := httptest.NewRequest(http.MethodGet, "/v1/agents/Codex%20Desktop/traces", nil)
	request.SetPathValue("agent_id", "  CoDeX DeSkToP  ")
	recorder := httptest.NewRecorder()

	server.listAgentTraces(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if traces.requestedAgentID != "codex" {
		t.Fatalf("store agent_id = %q, want canonical codex", traces.requestedAgentID)
	}
	var response struct {
		AgentID string         `json:"agent_id"`
		Traces  []TraceSummary `json:"traces"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.AgentID != "codex" || len(response.Traces) != 1 ||
		response.Traces[0].ServiceName != "Codex Desktop" {
		t.Fatalf("canonical response lost raw Trace source: %#v", response)
	}
}

func TestAgentTraceHandlerCanonicalizesXiaoBaOSTargetAndRejectsInternalSource(t *testing.T) {
	now := time.Now().UTC()
	traces := &recordingTraceStore{agentTraces: []TraceSummary{{
		TraceID:     "10112233445566778899aabbccddeeff",
		RootName:    "xiaoba.session",
		ServiceName: "barena-xiaoba-target",
		StartTime:   now.Add(-time.Minute),
		EndTime:     now,
		SpanCount:   1,
	}}}
	server := &HTTPServer{store: NewMemoryStore(), traces: traces}
	request := httptest.NewRequest(http.MethodGet, "/v1/agents/barena-xiaoba-target/traces", nil)
	request.SetPathValue("agent_id", "barena-xiaoba-target")
	recorder := httptest.NewRecorder()

	server.listAgentTraces(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if traces.requestedAgentID != "xiaobaos" {
		t.Fatalf("store agent_id = %q, want canonical xiaobaos", traces.requestedAgentID)
	}
	var response struct {
		AgentID string         `json:"agent_id"`
		Traces  []TraceSummary `json:"traces"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.AgentID != "xiaobaos" || len(response.Traces) != 1 ||
		response.Traces[0].ServiceName != "barena-xiaoba-target" {
		t.Fatalf("canonical response lost raw XiaoBaOS source: %#v", response)
	}

	traces.requestedAgentID = ""
	request = httptest.NewRequest(http.MethodGet, "/v1/agents/barena-explore-engine/traces", nil)
	request.SetPathValue("agent_id", "barena-explore-engine")
	recorder = httptest.NewRecorder()
	server.listAgentTraces(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("internal source status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if traces.requestedAgentID != "" {
		t.Fatalf("internal source reached Trace store as %q", traces.requestedAgentID)
	}
}

func testOTLPRequest(includeInvalid bool) *collectortracev1.ExportTraceServiceRequest {
	start := uint64(1_785_800_000_000_000_000)
	span := &tracev1.Span{
		TraceId:           []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		SpanId:            []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77},
		Name:              "codex.tool.read_file",
		Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: start,
		EndTimeUnixNano:   start + 12_000_000,
		Attributes: []*commonv1.KeyValue{
			stringAttribute("gen_ai.request.model", "gpt-5.6-sol"),
			stringAttribute("tool.call.arguments", `{"path":"README.md"}`),
			stringAttribute("tool.call.result", "file contents"),
		},
		Events: []*tracev1.Span_Event{{
			TimeUnixNano: start + 10_000_000,
			Name:         "tool.result",
			Attributes:   []*commonv1.KeyValue{stringAttribute("tool.name", "read_file")},
		}},
		Status: &tracev1.Status{Code: tracev1.Status_STATUS_CODE_OK},
	}
	spans := []*tracev1.Span{span}
	if includeInvalid {
		spans = append(spans, &tracev1.Span{
			TraceId:           []byte{0x01},
			SpanId:            []byte{0x01},
			Name:              "invalid",
			StartTimeUnixNano: start,
			EndTimeUnixNano:   start + 1,
		})
	}
	return &collectortracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
				stringAttribute("service.name", "codex"),
			}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Scope: &commonv1.InstrumentationScope{Name: "catena-test", Version: "1.0"},
				Spans: spans,
			}},
		}},
	}
}

func stringAttribute(key string, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key: key,
		Value: &commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{StringValue: value},
		},
	}
}

func mustProtoMarshal(t *testing.T, value proto.Message) []byte {
	t.Helper()
	encoded, err := proto.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mustProtoJSONMarshal(t *testing.T, value proto.Message) []byte {
	t.Helper()
	encoded, err := protojson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

type recordingTraceStore struct {
	ownerID          string
	spans            []TraceSpan
	requestedAgentID string
	agentTraces      []TraceSummary
}

func (s *recordingTraceStore) Ping(context.Context) error { return nil }
func (s *recordingTraceStore) Close() error               { return nil }
func (s *recordingTraceStore) InsertSpans(_ context.Context, ownerID string, spans []TraceSpan) error {
	s.ownerID = ownerID
	s.spans = append([]TraceSpan(nil), spans...)
	return nil
}
func (s *recordingTraceStore) ListTraces(context.Context, string, int) ([]TraceSummary, error) {
	return nil, nil
}
func (s *recordingTraceStore) ListAgentTraces(_ context.Context, _ string, agentID string, _ time.Time, _ time.Time, _ int) ([]TraceSummary, error) {
	s.requestedAgentID = agentID
	return append([]TraceSummary(nil), s.agentTraces...), nil
}
func (s *recordingTraceStore) GetTrace(context.Context, string, string) (TraceDetail, error) {
	return TraceDetail{}, ErrNotFound
}
func (s *recordingTraceStore) ListAgents(context.Context, string, int) ([]AgentSummary, error) {
	return nil, nil
}
