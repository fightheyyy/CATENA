package control

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const maxOTLPRequestBytes = 16 * 1024 * 1024

type TraceStore interface {
	Ping(context.Context) error
	InsertSpans(context.Context, string, []TraceSpan) error
	ListTraces(context.Context, string, int) ([]TraceSummary, error)
	ListAgentTraces(context.Context, string, string, time.Time, time.Time, int) ([]TraceSummary, error)
	GetTrace(context.Context, string, string) (TraceDetail, error)
	ListAgents(context.Context, string, int) ([]AgentSummary, error)
	Close() error
}

type TraceSpan struct {
	AgentID                   string         `json:"agent_id,omitempty"`
	TraceID                   string         `json:"trace_id"`
	SpanID                    string         `json:"span_id"`
	ParentSpanID              string         `json:"parent_span_id,omitempty"`
	TraceState                string         `json:"trace_state,omitempty"`
	Name                      string         `json:"name"`
	Kind                      int32          `json:"kind"`
	ServiceName               string         `json:"service_name"`
	ScopeName                 string         `json:"scope_name,omitempty"`
	ScopeVersion              string         `json:"scope_version,omitempty"`
	ResourceSchemaURL         string         `json:"resource_schema_url,omitempty"`
	ScopeSchemaURL            string         `json:"scope_schema_url,omitempty"`
	StartTime                 time.Time      `json:"start_time"`
	EndTime                   time.Time      `json:"end_time"`
	StatusCode                int32          `json:"status_code"`
	StatusMessage             string         `json:"status_message,omitempty"`
	Attributes                map[string]any `json:"attributes"`
	ResourceAttributes        map[string]any `json:"resource_attributes"`
	Events                    []TraceEvent   `json:"events"`
	Links                     []TraceLink    `json:"links"`
	Flags                     uint32         `json:"flags"`
	DroppedAttributesCount    uint32         `json:"dropped_attributes_count"`
	DroppedEventsCount        uint32         `json:"dropped_events_count"`
	DroppedLinksCount         uint32         `json:"dropped_links_count"`
	ResourceDroppedAttributes uint32         `json:"resource_dropped_attributes_count"`
	Model                     string         `json:"model,omitempty"`
	Input                     string         `json:"input,omitempty"`
	Output                    string         `json:"output,omitempty"`
}

type TraceEvent struct {
	Name                   string         `json:"name"`
	Time                   time.Time      `json:"time"`
	Attributes             map[string]any `json:"attributes"`
	DroppedAttributesCount uint32         `json:"dropped_attributes_count"`
}

type TraceLink struct {
	TraceID                string         `json:"trace_id"`
	SpanID                 string         `json:"span_id"`
	TraceState             string         `json:"trace_state,omitempty"`
	Attributes             map[string]any `json:"attributes"`
	DroppedAttributesCount uint32         `json:"dropped_attributes_count"`
	Flags                  uint32         `json:"flags"`
}

type TraceSummary struct {
	AgentID      string    `json:"agent_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	TraceID      string    `json:"trace_id"`
	RootName     string    `json:"root_name"`
	InputPreview string    `json:"input_preview,omitempty"`
	ServiceName  string    `json:"service_name"`
	Model        string    `json:"model,omitempty"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	DurationMS   int64     `json:"duration_ms"`
	SpanCount    uint64    `json:"span_count"`
	ErrorCount   uint64    `json:"error_count"`
	LastIngested time.Time `json:"last_ingested_at"`
}

type TraceDetail struct {
	Summary TraceSummary `json:"summary"`
	Spans   []TraceSpan  `json:"spans"`
}

type AgentSummary struct {
	AgentID           string        `json:"agent_id"`
	DisplayName       string        `json:"display_name"`
	IdentitySource    string        `json:"identity_source"`
	RuntimeKind       string        `json:"runtime_kind,omitempty"`
	Registered        bool          `json:"registered"`
	Connected         bool          `json:"connected"`
	ConversationCount uint64        `json:"conversation_count"`
	Credential        *APIToken     `json:"credential,omitempty"`
	TraceCount        uint64        `json:"trace_count"`
	SpanCount         uint64        `json:"span_count"`
	ErrorCount        uint64        `json:"error_count"`
	LastSeenAt        time.Time     `json:"last_seen_at"`
	Sources           []AgentSource `json:"sources"`
}

type decodedOTLP struct {
	Spans        []TraceSpan
	Rejected     int64
	ErrorMessage string
}

func (s *HTTPServer) ingestOTLPTraces(w http.ResponseWriter, r *http.Request) {
	if s.traces == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Trace storage is not configured")
		return
	}
	principal, ok := s.requireAgentAPITokenPrincipal(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOTLPRequestBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeProblem(w, http.StatusRequestEntityTooLarge, "OTLP request exceeds 16 MiB")
		return
	}
	decoded, encoding, err := decodeOTLPRequest(body, r.Header.Get("Content-Type"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(decoded.Spans) > 0 {
		if principal.Agent != nil {
			for index := range decoded.Spans {
				decoded.Spans[index].AgentID = principal.Agent.ID
				if decoded.Spans[index].ResourceAttributes == nil {
					decoded.Spans[index].ResourceAttributes = make(map[string]any)
				}
				decoded.Spans[index].ResourceAttributes["catena.agent.id"] = principal.Agent.ID
				decoded.Spans[index].ResourceAttributes["catena.agent.name"] = principal.Agent.DisplayName
			}
		}
		if err := s.traces.InsertSpans(r.Context(), principal.User.ID, decoded.Spans); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "Trace storage failed")
			return
		}
		if principal.Agent != nil {
			_ = s.store.ObserveRegisteredAgent(
				r.Context(), principal.User.ID, principal.Agent.ID,
				detectOTLPRuntime(decoded.Spans), time.Now().UTC(),
			)
		}
	}
	response := &collectortracev1.ExportTraceServiceResponse{}
	if decoded.Rejected > 0 {
		response.PartialSuccess = &collectortracev1.ExportTracePartialSuccess{
			RejectedSpans: decoded.Rejected,
			ErrorMessage:  decoded.ErrorMessage,
		}
	}
	writeOTLPResponse(w, response, encoding)
}

func (s *HTTPServer) listTraces(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.traces == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "traces": []TraceSummary{}})
		return
	}
	limit, err := traceListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	values, err := s.traces.ListTraces(r.Context(), traceOwnerID(user), limit)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "Trace query failed")
		return
	}
	if values == nil {
		values = []TraceSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "traces": values})
}

func (s *HTTPServer) getTrace(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.traces == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Trace storage is not configured")
		return
	}
	traceID := strings.ToLower(strings.TrimSpace(r.PathValue("trace_id")))
	if len(traceID) != 32 {
		writeProblem(w, http.StatusBadRequest, "trace_id must be 32 lowercase hexadecimal characters")
		return
	}
	if _, err := hex.DecodeString(traceID); err != nil {
		writeProblem(w, http.StatusBadRequest, "trace_id must be 32 lowercase hexadecimal characters")
		return
	}
	value, err := s.traces.GetTrace(r.Context(), traceOwnerID(user), traceID)
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *HTTPServer) listAgents(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit, err := traceListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	values := make([]AgentSummary, 0)
	if s.traces != nil {
		values, err = s.traces.ListAgents(r.Context(), traceOwnerID(user), limit)
		if err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "Agent query failed")
			return
		}
	}
	registered, err := s.store.ListRegisteredAgentsByOwner(r.Context(), traceOwnerID(user))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	tokens, err := s.store.ListAPITokensByUser(r.Context(), traceOwnerID(user))
	if err != nil {
		writeProblem(w, statusFor(err), err.Error())
		return
	}
	values = s.mergeRegisteredAgentSummaries(r, registered, tokens, values, limit)
	writeJSON(w, http.StatusOK, map[string]any{"available": s.traces != nil, "agents": values})
}

func (s *HTTPServer) mergeRegisteredAgentSummaries(
	r *http.Request,
	registered []RegisteredAgent,
	tokens []APIToken,
	observed []AgentSummary,
	limit int,
) []AgentSummary {
	byID := make(map[string]AgentSummary, len(registered)+len(observed))
	for _, summary := range observed {
		summary.Connected = !summary.LastSeenAt.IsZero()
		if summary.RuntimeKind == "" {
			summary.RuntimeKind = inferObservedAgentRuntime(summary)
		}
		byID[summary.AgentID] = summary
	}
	credentialByAgent := make(map[string]APIToken)
	for _, token := range tokens {
		if token.AgentID != "" {
			credentialByAgent[token.AgentID] = s.presentAPIToken(token)
		}
	}
	for _, agent := range registered {
		summary := byID[agent.ID]
		summary.AgentID = agent.ID
		summary.DisplayName = agent.DisplayName
		summary.IdentitySource = agentIdentitySourceCredential
		summary.RuntimeKind = agent.RuntimeKind
		summary.Registered = true
		summary.Connected = !agent.LastSeenAt.IsZero()
		if agent.LastSeenAt.After(summary.LastSeenAt) {
			summary.LastSeenAt = agent.LastSeenAt
		}
		if token, ok := credentialByAgent[agent.ID]; ok {
			tokenCopy := token
			summary.Credential = &tokenCopy
		}
		if conversations, err := s.store.ListConversationSummariesByOwner(
			r.Context(), agent.OwnerUserID, agent.ID, 200,
		); err == nil {
			summary.ConversationCount = uint64(len(conversations))
		}
		byID[agent.ID] = summary
	}
	result := make([]AgentSummary, 0, len(byID))
	for _, summary := range byID {
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Registered != result[j].Registered {
			return result[i].Registered
		}
		if !result[i].LastSeenAt.Equal(result[j].LastSeenAt) {
			return result[i].LastSeenAt.After(result[j].LastSeenAt)
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func inferObservedAgentRuntime(summary AgentSummary) string {
	for _, source := range summary.Sources {
		switch normalizedAgentSource(source.ServiceName) {
		case "xiaobaos", "barena-xiaoba-target":
			return "xiaobaos"
		case "catena-runtime-codex":
			return "codex"
		case "catena-runtime-claude-code":
			return "claude_code"
		}
	}
	return "otel"
}

func (s *HTTPServer) listAgentTraces(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if s.traces == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "traces": []TraceSummary{}})
		return
	}
	agentID, err := normalizedAgentID(r.PathValue("agent_id"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := traceListLimit(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	windowStart, windowEnd, err := agentTraceWindowFromQuery(r, time.Now().UTC())
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	values, err := s.traces.ListAgentTraces(
		r.Context(), traceOwnerID(user), agentID, windowStart, windowEnd, limit,
	)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "Agent Trace query failed")
		return
	}
	if values == nil {
		values = []TraceSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available":    true,
		"agent_id":     agentID,
		"window_start": windowStart,
		"window_end":   windowEnd,
		"traces":       values,
	})
}

func normalizedAgentID(value string) (string, error) {
	agentID := canonicalAgentForID(value).AgentID
	if agentID == "" || len(agentID) > 512 {
		return "", errors.New("agent_id must contain from 1 to 512 characters")
	}
	return agentID, nil
}

func agentTraceWindowFromQuery(r *http.Request, now time.Time) (time.Time, time.Time, error) {
	windowEnd := now.UTC()
	windowStart := windowEnd.Add(-7 * 24 * time.Hour)
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		windowStart, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("from must be an RFC3339 timestamp")
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		windowEnd, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("to must be an RFC3339 timestamp")
		}
	}
	request := CreateAgentEvolutionJobRequest{WindowStart: windowStart, WindowEnd: windowEnd}
	if err := request.Validate(now.UTC()); err != nil {
		return time.Time{}, time.Time{}, err
	}
	return windowStart.UTC(), windowEnd.UTC(), nil
}

func traceOwnerID(user *User) string {
	if user == nil {
		return "local"
	}
	return user.ID
}

func traceListLimit(r *http.Request) (int, error) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 500 {
			return 0, errors.New("limit must be from 1 to 500")
		}
		limit = value
	}
	return limit, nil
}

func decodeOTLPRequest(body []byte, contentType string) (decodedOTLP, string, error) {
	if len(body) == 0 {
		return decodedOTLP{}, "", errors.New("OTLP request body is empty")
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	request := &collectortracev1.ExportTraceServiceRequest{}
	encoding := "protobuf"
	switch mediaType {
	case "application/json":
		encoding = "json"
		if err := protojson.Unmarshal(body, request); err != nil {
			return decodedOTLP{}, "", fmt.Errorf("invalid OTLP JSON: %w", err)
		}
	case "application/x-protobuf", "application/protobuf", "":
		if err := proto.Unmarshal(body, request); err != nil {
			return decodedOTLP{}, "", fmt.Errorf("invalid OTLP protobuf: %w", err)
		}
	default:
		return decodedOTLP{}, "", errors.New("Content-Type must be application/x-protobuf or application/json")
	}
	decoded := convertOTLPTraces(request)
	return decoded, encoding, nil
}

func writeOTLPResponse(w http.ResponseWriter, response *collectortracev1.ExportTraceServiceResponse, encoding string) {
	var (
		body []byte
		err  error
	)
	if encoding == "json" {
		w.Header().Set("Content-Type", "application/json")
		body, err = protojson.Marshal(response)
	} else {
		w.Header().Set("Content-Type", "application/x-protobuf")
		body, err = proto.Marshal(response)
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "OTLP response encoding failed")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func convertOTLPTraces(request *collectortracev1.ExportTraceServiceRequest) decodedOTLP {
	result := decodedOTLP{Spans: make([]TraceSpan, 0)}
	for _, resourceSpans := range request.GetResourceSpans() {
		resource := resourceSpans.GetResource()
		resourceAttributes := keyValues(resource.GetAttributes())
		serviceName := attributeString(resourceAttributes, "service.name")
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			scope := scopeSpans.GetScope()
			for _, span := range scopeSpans.GetSpans() {
				converted, ok := convertOTLPSpan(
					span,
					resourceAttributes,
					resource.GetDroppedAttributesCount(),
					resourceSpans.GetSchemaUrl(),
					scopeSpans.GetSchemaUrl(),
					serviceName,
					scope.GetName(),
					scope.GetVersion(),
				)
				if !ok {
					result.Rejected++
					continue
				}
				result.Spans = append(result.Spans, converted)
			}
		}
	}
	if result.Rejected > 0 {
		result.ErrorMessage = "spans with invalid identity or timestamps were rejected"
	}
	return result
}

func convertOTLPSpan(
	span *tracev1.Span,
	resourceAttributes map[string]any,
	resourceDropped uint32,
	resourceSchemaURL string,
	scopeSchemaURL string,
	serviceName string,
	scopeName string,
	scopeVersion string,
) (TraceSpan, bool) {
	if span == nil || len(span.GetTraceId()) != 16 || len(span.GetSpanId()) != 8 ||
		span.GetStartTimeUnixNano() == 0 || span.GetStartTimeUnixNano() > math.MaxInt64 ||
		span.GetEndTimeUnixNano() > math.MaxInt64 || span.GetEndTimeUnixNano() < span.GetStartTimeUnixNano() {
		return TraceSpan{}, false
	}
	attributes := keyValues(span.GetAttributes())
	converted := TraceSpan{
		TraceID:                   hex.EncodeToString(span.GetTraceId()),
		SpanID:                    hex.EncodeToString(span.GetSpanId()),
		ParentSpanID:              hex.EncodeToString(span.GetParentSpanId()),
		TraceState:                span.GetTraceState(),
		Name:                      span.GetName(),
		Kind:                      int32(span.GetKind()),
		ServiceName:               serviceName,
		ScopeName:                 scopeName,
		ScopeVersion:              scopeVersion,
		ResourceSchemaURL:         resourceSchemaURL,
		ScopeSchemaURL:            scopeSchemaURL,
		StartTime:                 time.Unix(0, int64(span.GetStartTimeUnixNano())).UTC(),
		EndTime:                   time.Unix(0, int64(span.GetEndTimeUnixNano())).UTC(),
		StatusCode:                int32(span.GetStatus().GetCode()),
		StatusMessage:             span.GetStatus().GetMessage(),
		Attributes:                attributes,
		ResourceAttributes:        cloneMap(resourceAttributes),
		Events:                    convertEvents(span.GetEvents()),
		Links:                     convertLinks(span.GetLinks()),
		Flags:                     span.GetFlags(),
		DroppedAttributesCount:    span.GetDroppedAttributesCount(),
		DroppedEventsCount:        span.GetDroppedEventsCount(),
		DroppedLinksCount:         span.GetDroppedLinksCount(),
		ResourceDroppedAttributes: resourceDropped,
		Model: firstAttributeString(attributes,
			"gen_ai.response.model", "gen_ai.request.model", "llm.response.model", "llm.request.model", "model"),
		Input: firstAttributeString(attributes,
			"input.value", "gen_ai.input.messages", "gen_ai.prompt", "gen_ai.tool.call.arguments", "tool.call.arguments"),
		Output: firstAttributeString(attributes,
			"output.value", "gen_ai.output.messages", "gen_ai.completion", "gen_ai.tool.call.result", "tool.call.result"),
	}
	if converted.ServiceName == "" {
		converted.ServiceName = firstAttributeString(attributes, "service.name", "agent.name", "runtime.name")
	}
	return converted, true
}

func convertEvents(events []*tracev1.Span_Event) []TraceEvent {
	result := make([]TraceEvent, 0, len(events))
	for _, event := range events {
		if event == nil || event.GetTimeUnixNano() > math.MaxInt64 {
			continue
		}
		result = append(result, TraceEvent{
			Name:                   event.GetName(),
			Time:                   time.Unix(0, int64(event.GetTimeUnixNano())).UTC(),
			Attributes:             keyValues(event.GetAttributes()),
			DroppedAttributesCount: event.GetDroppedAttributesCount(),
		})
	}
	return result
}

func convertLinks(links []*tracev1.Span_Link) []TraceLink {
	result := make([]TraceLink, 0, len(links))
	for _, link := range links {
		if link == nil || len(link.GetTraceId()) != 16 || len(link.GetSpanId()) != 8 {
			continue
		}
		result = append(result, TraceLink{
			TraceID:                hex.EncodeToString(link.GetTraceId()),
			SpanID:                 hex.EncodeToString(link.GetSpanId()),
			TraceState:             link.GetTraceState(),
			Attributes:             keyValues(link.GetAttributes()),
			DroppedAttributesCount: link.GetDroppedAttributesCount(),
			Flags:                  link.GetFlags(),
		})
	}
	return result
}

func keyValues(values []*commonv1.KeyValue) map[string]any {
	result := make(map[string]any, len(values))
	for _, item := range values {
		if item == nil || item.GetKey() == "" {
			continue
		}
		result[item.GetKey()] = anyValue(item.GetValue())
	}
	return result
}

func anyValue(value *commonv1.AnyValue) any {
	if value == nil {
		return nil
	}
	switch typed := value.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return typed.StringValue
	case *commonv1.AnyValue_BoolValue:
		return typed.BoolValue
	case *commonv1.AnyValue_IntValue:
		return typed.IntValue
	case *commonv1.AnyValue_DoubleValue:
		return typed.DoubleValue
	case *commonv1.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue)
	case *commonv1.AnyValue_ArrayValue:
		values := typed.ArrayValue.GetValues()
		result := make([]any, 0, len(values))
		for _, item := range values {
			result = append(result, anyValue(item))
		}
		return result
	case *commonv1.AnyValue_KvlistValue:
		return keyValues(typed.KvlistValue.GetValues())
	default:
		return nil
	}
}

func firstAttributeString(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := attributes[key]; ok {
			if text := valueString(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func attributeString(attributes map[string]any, key string) string {
	return valueString(attributes[key])
}

func valueString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
