package control

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ClickHouseTraceStore struct {
	conn     driver.Conn
	otel     bool
	readOnly bool
}

// OpenOTelTraceStore never creates or alters tables. Apply the versioned
// ClickHouse migrations before deploying readers or Collector consumers.
func OpenOTelTraceStore(ctx context.Context, dsn string) (*ClickHouseTraceStore, error) {
	options, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, err
	}
	s := &ClickHouseTraceStore{conn: conn, otel: true, readOnly: true}
	if err = s.Ping(ctx); err == nil {
		err = conn.Exec(ctx, "SELECT trace_id FROM catena_otel_spans LIMIT 0")
	}
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

// OpenLegacyTraceReader is a rollback reader for retained historical data.
// It does not migrate/ALTER the old table and cannot resume the old writer.
func OpenLegacyTraceReader(ctx context.Context, dsn string) (*ClickHouseTraceStore, error) {
	options, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, err
	}
	s := &ClickHouseTraceStore{conn: conn, readOnly: true}
	if err = conn.Exec(ctx, "SELECT trace_id FROM catena_spans LIMIT 0"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

func (s *ClickHouseTraceStore) query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	if s.otel {
		query = strings.ReplaceAll(query, "catena_spans FINAL", "catena_otel_spans")
		// View storage rejects explicit PREWHERE. WHERE predicates on the
		// immutable owner/trace keys are pushed into its underlying FINAL read.
		query = strings.ReplaceAll(query, "PREWHERE", "WHERE")
		query = strings.ReplaceAll(query, "\n  WHERE end_time", "\n  AND end_time")
	}
	return s.conn.Query(ctx, query, args...)
}

func (s *ClickHouseTraceStore) Ping(ctx context.Context) error {
	return s.conn.Ping(ctx)
}

func (s *ClickHouseTraceStore) Close() error {
	return s.conn.Close()
}

func (s *ClickHouseTraceStore) ListTraces(
	ctx context.Context,
	ownerID string,
	limit int,
) ([]TraceSummary, error) {
	// Select the page using narrow columns. Session IDs and input previews are
	// stored separately so list queries never load potentially multi-MB text.
	rows, err := s.query(ctx, `
WITH recent_traces AS (
  SELECT trace_id
  FROM catena_spans FINAL
  WHERE owner_id = ?
  GROUP BY trace_id
  ORDER BY max(end_time) DESC, trace_id
  LIMIT ?
)
SELECT
  anyIf(agent_id, agent_id != '') AS agent_id,
  argMinIf(session_id, tuple(session_priority, start_time), session_id != '') AS session_id,
  trace_id,
  argMin(name, tuple(parent_span_id != '', start_time)) AS root_name,
  argMinIf(input_preview, start_time, input_preview != '') AS input_preview,
  argMin(service_name, start_time) AS service_name,
  argMinIf(model, tuple(start_time, span_id), model != '') AS model,
  min(start_time) AS started_at,
  max(end_time) AS ended_at,
  toInt64(dateDiff('millisecond', min(start_time), max(end_time))) AS duration_ms,
  count() AS span_count,
  countIf(status_code = 2) AS error_count,
  max(inserted_at) AS last_ingested_at
FROM catena_spans FINAL
PREWHERE owner_id = ? AND trace_id IN (SELECT trace_id FROM recent_traces)
GROUP BY trace_id
ORDER BY ended_at DESC, trace_id`, ownerID, limit, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TraceSummary, 0)
	for rows.Next() {
		var value TraceSummary
		if err := rows.Scan(
			&value.AgentID,
			&value.SessionID,
			&value.TraceID,
			&value.RootName,
			&value.InputPreview,
			&value.ServiceName,
			&value.Model,
			&value.StartTime,
			&value.EndTime,
			&value.DurationMS,
			&value.SpanCount,
			&value.ErrorCount,
			&value.LastIngested,
		); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *ClickHouseTraceStore) ListAgentTraces(
	ctx context.Context,
	ownerID string,
	agentID string,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
) ([]TraceSummary, error) {
	legacyFilter, legacyFilterArgs := agentServiceNameFilter(agentID)
	// Apply Agent and window selection before LIMIT, and keep timestamps out of
	// PREWHERE: only owner_id/trace_id are immutable replacement sorting keys.
	query := fmt.Sprintf(`
WITH recent_traces AS (
  SELECT trace_id
  FROM (
    SELECT
      trace_id,
      anyIf(agent_id, agent_id != '') AS agent_id,
      argMin(service_name, tuple(parent_span_id != '', start_time)) AS service_name,
      max(end_time) AS ended_at
    FROM catena_spans FINAL
    WHERE owner_id = ? AND end_time >= ? AND start_time <= ?
    GROUP BY trace_id
  )
  WHERE agent_id = ? OR (agent_id = '' AND %s)
  ORDER BY ended_at DESC, trace_id
  LIMIT ?
)
SELECT
  agent_id, session_id, trace_id, root_name, input_preview, service_name, model, started_at, ended_at,
  duration_ms, span_count, error_count, last_ingested_at
FROM (
  SELECT
    trace_id,
	    anyIf(agent_id, agent_id != '') AS agent_id,
	    argMinIf(session_id, tuple(session_priority, start_time), session_id != '') AS session_id,
    argMin(name, tuple(parent_span_id != '', start_time)) AS root_name,
    argMinIf(input_preview, start_time, input_preview != '') AS input_preview,
    argMin(service_name, tuple(parent_span_id != '', start_time)) AS service_name,
    argMinIf(model, tuple(start_time, span_id), model != '') AS model,
    min(start_time) AS started_at,
    max(end_time) AS ended_at,
    toInt64(dateDiff('millisecond', min(start_time), max(end_time))) AS duration_ms,
    count() AS span_count,
    countIf(status_code = 2) AS error_count,
    max(inserted_at) AS last_ingested_at
  FROM catena_spans FINAL
  PREWHERE owner_id = ? AND trace_id IN (SELECT trace_id FROM recent_traces)
  WHERE end_time >= ? AND start_time <= ?
  GROUP BY trace_id
)
ORDER BY ended_at DESC, trace_id`, legacyFilter)
	args := []any{ownerID, windowStart, windowEnd}
	args = append(args, agentID)
	args = append(args, legacyFilterArgs...)
	args = append(args, limit, ownerID, windowStart, windowEnd)
	rows, err := s.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TraceSummary, 0)
	for rows.Next() {
		var value TraceSummary
		if err := rows.Scan(
			&value.AgentID,
			&value.SessionID,
			&value.TraceID,
			&value.RootName,
			&value.InputPreview,
			&value.ServiceName,
			&value.Model,
			&value.StartTime,
			&value.EndTime,
			&value.DurationMS,
			&value.SpanCount,
			&value.ErrorCount,
			&value.LastIngested,
		); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *ClickHouseTraceStore) GetTrace(
	ctx context.Context,
	ownerID string,
	traceID string,
) (TraceDetail, error) {
	rows, err := s.query(ctx, `
SELECT
  agent_id, trace_id, span_id, parent_span_id, trace_state, name, kind, service_name,
  scope_name, scope_version, resource_schema_url, scope_schema_url, start_time,
  end_time, status_code, status_message, attributes_json,
  resource_attributes_json, events_json, links_json, flags,
  dropped_attributes_count, dropped_events_count, dropped_links_count,
  resource_dropped_attributes, model, input, output, inserted_at
FROM catena_spans FINAL
WHERE owner_id = ? AND trace_id = ?
ORDER BY start_time, span_id`, ownerID, traceID)
	if err != nil {
		return TraceDetail{}, err
	}
	defer rows.Close()
	spans := make([]TraceSpan, 0)
	var lastIngested time.Time
	for rows.Next() {
		var (
			span               TraceSpan
			kind               uint8
			statusCode         uint8
			attributes         string
			resourceAttributes string
			events             string
			links              string
			insertedAt         time.Time
		)
		if err := rows.Scan(
			&span.AgentID,
			&span.TraceID,
			&span.SpanID,
			&span.ParentSpanID,
			&span.TraceState,
			&span.Name,
			&kind,
			&span.ServiceName,
			&span.ScopeName,
			&span.ScopeVersion,
			&span.ResourceSchemaURL,
			&span.ScopeSchemaURL,
			&span.StartTime,
			&span.EndTime,
			&statusCode,
			&span.StatusMessage,
			&attributes,
			&resourceAttributes,
			&events,
			&links,
			&span.Flags,
			&span.DroppedAttributesCount,
			&span.DroppedEventsCount,
			&span.DroppedLinksCount,
			&span.ResourceDroppedAttributes,
			&span.Model,
			&span.Input,
			&span.Output,
			&insertedAt,
		); err != nil {
			return TraceDetail{}, err
		}
		span.Kind = int32(kind)
		span.StatusCode = int32(statusCode)
		if err := unmarshalTraceJSON(attributes, &span.Attributes); err != nil {
			return TraceDetail{}, err
		}
		if err := unmarshalTraceJSON(resourceAttributes, &span.ResourceAttributes); err != nil {
			return TraceDetail{}, err
		}
		if err := unmarshalTraceJSON(events, &span.Events); err != nil {
			return TraceDetail{}, err
		}
		if err := unmarshalTraceJSON(links, &span.Links); err != nil {
			return TraceDetail{}, err
		}
		if insertedAt.After(lastIngested) {
			lastIngested = insertedAt
		}
		spans = append(spans, span)
	}
	if err := rows.Err(); err != nil {
		return TraceDetail{}, err
	}
	if len(spans) == 0 {
		return TraceDetail{}, ErrNotFound
	}
	return TraceDetail{Summary: summarizeTrace(spans, lastIngested), Spans: spans}, nil
}

func (s *ClickHouseTraceStore) ListAgents(
	ctx context.Context,
	ownerID string,
	limit int,
) ([]AgentSummary, error) {
	rows, err := s.query(ctx, `
SELECT
  agent_id,
  service_name,
  count() AS trace_count,
  sum(span_count) AS span_count,
  sum(error_count) AS error_count,
  max(last_seen_at) AS last_seen_at
FROM (
  SELECT
    trace_id,
	    anyIf(agent_id, agent_id != '') AS agent_id,
    argMin(service_name, tuple(parent_span_id != '', start_time)) AS service_name,
    count() AS span_count,
    countIf(status_code = 2) AS error_count,
    max(end_time) AS last_seen_at
  FROM catena_spans FINAL
  WHERE owner_id = ?
  GROUP BY trace_id
)
WHERE service_name != ''
GROUP BY agent_id, service_name
ORDER BY last_seen_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]agentSourceAggregate, 0)
	for rows.Next() {
		var value agentSourceAggregate
		if err := rows.Scan(
			&value.AgentID,
			&value.ServiceName,
			&value.TraceCount,
			&value.SpanCount,
			&value.ErrorCount,
			&value.LastSeenAt,
		); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return mergeAgentSourceAggregates(values, limit), nil
}

func agentServiceNameFilter(agentID string) (string, []any) {
	identity := canonicalAgentForID(agentID)
	if identity.AgentID == "" || len(identity.Aliases) == 0 {
		return "0", nil
	}
	aliases := identity.Aliases
	column := "trimBoth(service_name)"
	if identity.IdentitySource == agentIdentitySourceAlias {
		column = "lowerUTF8(trimBoth(service_name))"
	}
	placeholders := make([]string, 0, len(aliases))
	args := make([]any, 0, len(aliases))
	for _, alias := range aliases {
		placeholders = append(placeholders, "?")
		args = append(args, alias)
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", ")), args
}

func summarizeTrace(spans []TraceSpan, lastIngested time.Time) TraceSummary {
	sessionPriority := traceSpanSessionPriority(spans[0])
	summary := TraceSummary{
		AgentID:      spans[0].AgentID,
		SessionID:    traceSpanSessionID(spans[0]),
		TraceID:      spans[0].TraceID,
		RootName:     spans[0].Name,
		InputPreview: traceInputPreview(spans),
		ServiceName:  spans[0].ServiceName,
		StartTime:    spans[0].StartTime,
		EndTime:      spans[0].EndTime,
		SpanCount:    uint64(len(spans)),
		LastIngested: lastIngested,
	}
	for _, span := range spans {
		if summary.AgentID == "" && span.AgentID != "" {
			summary.AgentID = span.AgentID
		}
		if priority := traceSpanSessionPriority(span); priority < sessionPriority {
			summary.SessionID = traceSpanSessionID(span)
			sessionPriority = priority
		}
		if span.ParentSpanID == "" {
			summary.RootName = span.Name
		}
		if span.StartTime.Before(summary.StartTime) {
			summary.StartTime = span.StartTime
		}
		if span.EndTime.After(summary.EndTime) {
			summary.EndTime = span.EndTime
		}
		if summary.Model == "" && span.Model != "" {
			summary.Model = span.Model
		}
		if summary.ServiceName == "" && span.ServiceName != "" {
			summary.ServiceName = span.ServiceName
		}
		if span.StatusCode == 2 {
			summary.ErrorCount++
		}
	}
	summary.DurationMS = summary.EndTime.Sub(summary.StartTime).Milliseconds()
	return summary
}

func traceInputPreview(spans []TraceSpan) string {
	var fallback string
	for _, span := range spans {
		input := strings.TrimSpace(span.Input)
		if input == "" {
			continue
		}
		if fallback == "" {
			fallback = input
		}
		if span.Name == "agent.turn" || strings.EqualFold(firstAttributeString(span.Attributes, "catena.node.kind"), "turn") {
			return boundedRunes(input, 512)
		}
	}
	return boundedRunes(fallback, 512)
}

func traceSpanSessionID(span TraceSpan) string {
	id, _ := traceSpanSessionIdentity(span)
	return id
}

func traceSpanSessionPriority(span TraceSpan) uint8 {
	_, priority := traceSpanSessionIdentity(span)
	return priority
}

func traceSpanSessionIdentity(span TraceSpan) (string, uint8) {
	keys := []string{
		"agent.session.id",
		"gen_ai.conversation.id",
		"gen_ai.session.id",
		"session.id",
		"conversation.id",
	}
	for index, key := range keys {
		if value, ok := span.Attributes[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), uint8(index)
		}
	}
	for index, key := range keys {
		if value, ok := span.ResourceAttributes[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), uint8(len(keys) + index)
		}
	}
	return "", 255
}

func marshalTraceJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode Trace JSON: %w", err)
	}
	return string(encoded), nil
}

func unmarshalTraceJSON(value string, target any) error {
	if value == "" {
		value = "null"
	}
	if err := json.Unmarshal([]byte(value), target); err != nil {
		return fmt.Errorf("decode Trace JSON: %w", err)
	}
	return nil
}
