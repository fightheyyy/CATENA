package control

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Only API-key-derived identities may reach the internal telemetry pipeline.
// Never trust either resource- or span-level identity attributes from clients.
func bindTraceIdentity(s *TraceSpan, owner, agent, name string) {
	clean := func(in map[string]any) map[string]any {
		out := make(map[string]any, len(in)+2)
		for k, v := range in {
			if k == "catena.owner.id" || k == "catena.agent.id" || k == "catena.agent.name" || strings.HasPrefix(k, "catena.internal.") {
				continue
			}
			out[k] = v
		}
		out["catena.owner.id"] = owner
		if agent != "" {
			out["catena.agent.id"] = agent
			out["catena.agent.name"] = name
		}
		return out
	}
	s.AgentID = agent
	s.Attributes = clean(s.Attributes)
	s.ResourceAttributes = clean(s.ResourceAttributes)
}

type CollectorTraceSink struct {
	endpoint string
	client   *http.Client
}

var ErrCollectorBatchTooLarge = errors.New("normalized OTLP batch exceeds 30 MiB; split the batch")

func NewCollectorTraceSink(endpoint string) (*CollectorTraceSink, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("Collector endpoint must be an HTTP(S) URL without credentials, query or fragment")
	}
	// Do not follow redirects with trusted tenant payloads.
	return &CollectorTraceSink{endpoint: strings.TrimRight(endpoint, "/") + "/v1/traces", client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (s *CollectorTraceSink) InsertSpans(ctx context.Context, owner string, spans []TraceSpan) error {
	if owner == "" {
		return errors.New("Trace owner is required")
	}
	if len(spans) == 0 {
		return nil
	}
	request, e := collectorTraceRequest(owner, spans)
	if e != nil {
		return e
	}
	body, e := proto.Marshal(request)
	if e != nil {
		return e
	}
	if len(body) > 30*1024*1024 {
		return ErrCollectorBatchTooLarge
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	res, e := s.client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return fmt.Errorf("Collector returned HTTP %d", res.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if e != nil {
		return e
	}
	if len(data) > 1024*1024 {
		return errors.New("Collector response exceeds limit")
	}
	var ack collectortracev1.ExportTraceServiceResponse
	if e = proto.Unmarshal(data, &ack); e != nil {
		return fmt.Errorf("invalid Collector acknowledgement: %w", e)
	}
	if ack.GetPartialSuccess().GetRejectedSpans() > 0 {
		return errors.New("Collector rejected spans; retry batch")
	}
	return nil // Gateway responds only after its synchronous Kafka export succeeds.
}

// Standard OTLP on the wire, standard official Exporter columns in storage.
// A versioned metadata attribute preserves Catena's typed evidence contract:
// the official Exporter otherwise stringifies attribute values and omits flags,
// schema URLs and dropped counts. It is overwritten here, never client-owned.
func collectorTraceRequest(owner string, spans []TraceSpan) (*collectortracev1.ExportTraceServiceRequest, error) {
	out := &collectortracev1.ExportTraceServiceRequest{}
	groups := make(map[string]*tracev1.ResourceSpans)
	for _, span := range spans {
		// Identity was bound by ingress; callers cannot change Owner via attributes.
		bindTraceIdentity(&span, owner, span.AgentID, firstAttributeString(span.ResourceAttributes, "catena.agent.name"))
		meta := map[string]any{
			"attributes":                        span.Attributes,
			"resource_attributes":               span.ResourceAttributes,
			"events":                            span.Events,
			"links":                             span.Links,
			"flags":                             span.Flags,
			"dropped_attributes_count":          span.DroppedAttributesCount,
			"dropped_events_count":              span.DroppedEventsCount,
			"dropped_links_count":               span.DroppedLinksCount,
			"resource_dropped_attributes_count": span.ResourceDroppedAttributes,
			"resource_schema_url":               span.ResourceSchemaURL,
			"scope_schema_url":                  span.ScopeSchemaURL,
			"model":                             span.Model,
			"input":                             span.Input,
			"output":                            span.Output,
			"session_id":                        traceSpanSessionID(span),
			"session_priority":                  traceSpanSessionPriority(span),
			"input_preview":                     boundedRunes(strings.TrimSpace(span.Input), 512),
		}
		encoded, e := json.Marshal(meta)
		if e != nil {
			return nil, e
		}
		attributes := otlpAttributes(span.Attributes)
		attributes = append(attributes, &commonv1.KeyValue{Key: "catena.internal.meta.v1", Value: otlpValue(string(encoded))})
		traceID, e := hex.DecodeString(span.TraceID)
		if e != nil {
			return nil, e
		}
		spanID, e := hex.DecodeString(span.SpanID)
		if e != nil {
			return nil, e
		}
		parent, e := hex.DecodeString(span.ParentSpanID)
		if e != nil {
			return nil, e
		}
		p := &tracev1.Span{TraceId: traceID, SpanId: spanID, ParentSpanId: parent, TraceState: span.TraceState, Name: span.Name, Kind: tracev1.Span_SpanKind(span.Kind), StartTimeUnixNano: uint64(span.StartTime.UnixNano()), EndTimeUnixNano: uint64(span.EndTime.UnixNano()), Attributes: attributes, Status: &tracev1.Status{Code: tracev1.Status_StatusCode(span.StatusCode), Message: span.StatusMessage}, Flags: span.Flags, DroppedAttributesCount: span.DroppedAttributesCount, DroppedEventsCount: span.DroppedEventsCount, DroppedLinksCount: span.DroppedLinksCount}
		for _, ev := range span.Events {
			p.Events = append(p.Events, &tracev1.Span_Event{Name: ev.Name, TimeUnixNano: uint64(ev.Time.UnixNano()), Attributes: otlpAttributes(ev.Attributes), DroppedAttributesCount: ev.DroppedAttributesCount})
		}
		for _, link := range span.Links {
			t, _ := hex.DecodeString(link.TraceID)
			id, _ := hex.DecodeString(link.SpanID)
			p.Links = append(p.Links, &tracev1.Span_Link{TraceId: t, SpanId: id, TraceState: link.TraceState, Attributes: otlpAttributes(link.Attributes), DroppedAttributesCount: link.DroppedAttributesCount, Flags: link.Flags})
		}
		key, e := json.Marshal([]any{span.ResourceAttributes, span.ResourceDroppedAttributes, span.ResourceSchemaURL, span.ScopeName, span.ScopeVersion, span.ScopeSchemaURL})
		if e != nil {
			return nil, e
		}
		group := groups[string(key)]
		if group == nil {
			group = &tracev1.ResourceSpans{SchemaUrl: span.ResourceSchemaURL, Resource: &resourcev1.Resource{Attributes: otlpAttributes(span.ResourceAttributes), DroppedAttributesCount: span.ResourceDroppedAttributes}, ScopeSpans: []*tracev1.ScopeSpans{{SchemaUrl: span.ScopeSchemaURL, Scope: &commonv1.InstrumentationScope{Name: span.ScopeName, Version: span.ScopeVersion}}}}
			groups[string(key)] = group
			out.ResourceSpans = append(out.ResourceSpans, group)
		}
		group.ScopeSpans[0].Spans = append(group.ScopeSpans[0].Spans, p)
	}
	return out, nil
}

func otlpAttributes(values map[string]any) []*commonv1.KeyValue {
	out := make([]*commonv1.KeyValue, 0, len(values))
	for k, v := range values {
		out = append(out, &commonv1.KeyValue{Key: k, Value: otlpValue(v)})
	}
	return out
}
func otlpValue(value any) *commonv1.AnyValue {
	v := &commonv1.AnyValue{}
	switch x := value.(type) {
	case string:
		v.Value = &commonv1.AnyValue_StringValue{StringValue: x}
	case bool:
		v.Value = &commonv1.AnyValue_BoolValue{BoolValue: x}
	case int64:
		v.Value = &commonv1.AnyValue_IntValue{IntValue: x}
	case int:
		v.Value = &commonv1.AnyValue_IntValue{IntValue: int64(x)}
	case float64:
		v.Value = &commonv1.AnyValue_DoubleValue{DoubleValue: x}
	case []byte:
		v.Value = &commonv1.AnyValue_BytesValue{BytesValue: x}
	case []any:
		a := &commonv1.ArrayValue{}
		for _, item := range x {
			a.Values = append(a.Values, otlpValue(item))
		}
		v.Value = &commonv1.AnyValue_ArrayValue{ArrayValue: a}
	case map[string]any:
		v.Value = &commonv1.AnyValue_KvlistValue{KvlistValue: &commonv1.KeyValueList{Values: otlpAttributes(x)}}
	default:
		if value != nil {
			v.Value = &commonv1.AnyValue_StringValue{StringValue: fmt.Sprint(value)}
		}
	}
	return v
}
