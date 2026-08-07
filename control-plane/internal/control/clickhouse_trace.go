package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ClickHouseTraceStore struct {
	conn driver.Conn
}

func OpenClickHouseTraceStore(ctx context.Context, dsn string) (*ClickHouseTraceStore, error) {
	options, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse ClickHouse DSN: %w", err)
	}
	connection, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("open ClickHouse: %w", err)
	}
	store := &ClickHouseTraceStore{conn: connection}
	if err := store.Ping(ctx); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("ping ClickHouse: %w", err)
	}
	if err := store.migrate(ctx); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("migrate ClickHouse: %w", err)
	}
	return store, nil
}

func (s *ClickHouseTraceStore) migrate(ctx context.Context) error {
	if err := s.conn.Exec(ctx, `
CREATE TABLE IF NOT EXISTS catena_spans (
  owner_id String,
	  agent_id String,
  trace_id String,
  span_id String,
  parent_span_id String,
  trace_state String,
  name String,
  kind UInt8,
  service_name LowCardinality(String),
  scope_name LowCardinality(String),
  scope_version String,
  resource_schema_url String,
  scope_schema_url String,
  start_time DateTime64(9, 'UTC'),
  end_time DateTime64(9, 'UTC'),
  status_code UInt8,
  status_message String,
  attributes_json String,
  resource_attributes_json String,
  events_json String,
  links_json String,
  flags UInt32,
  dropped_attributes_count UInt32,
  dropped_events_count UInt32,
  dropped_links_count UInt32,
  resource_dropped_attributes UInt32,
  model String,
  input String,
  output String,
  inserted_at DateTime64(9, 'UTC')
) ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY (owner_id, trace_id, span_id)
`); err != nil {
		return err
	}
	return s.conn.Exec(ctx, `ALTER TABLE catena_spans ADD COLUMN IF NOT EXISTS agent_id String AFTER owner_id`)
}

func (s *ClickHouseTraceStore) Ping(ctx context.Context) error {
	return s.conn.Ping(ctx)
}

func (s *ClickHouseTraceStore) Close() error {
	return s.conn.Close()
}

func (s *ClickHouseTraceStore) InsertSpans(ctx context.Context, ownerID string, spans []TraceSpan) error {
	if ownerID == "" {
		return errors.New("Trace owner is required")
	}
	if len(spans) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
INSERT INTO catena_spans (
  owner_id, agent_id, trace_id, span_id, parent_span_id, trace_state, name, kind,
  service_name, scope_name, scope_version, resource_schema_url,
  scope_schema_url, start_time, end_time, status_code, status_message,
  attributes_json, resource_attributes_json, events_json, links_json, flags,
  dropped_attributes_count, dropped_events_count, dropped_links_count,
  resource_dropped_attributes, model, input, output, inserted_at
)`)
	if err != nil {
		return err
	}
	defer func() { _ = batch.Close() }()
	now := time.Now().UTC()
	for _, span := range spans {
		attributes, err := marshalTraceJSON(span.Attributes)
		if err != nil {
			return err
		}
		resourceAttributes, err := marshalTraceJSON(span.ResourceAttributes)
		if err != nil {
			return err
		}
		events, err := marshalTraceJSON(span.Events)
		if err != nil {
			return err
		}
		links, err := marshalTraceJSON(span.Links)
		if err != nil {
			return err
		}
		if err := batch.Append(
			ownerID,
			span.AgentID,
			span.TraceID,
			span.SpanID,
			span.ParentSpanID,
			span.TraceState,
			span.Name,
			uint8(span.Kind),
			span.ServiceName,
			span.ScopeName,
			span.ScopeVersion,
			span.ResourceSchemaURL,
			span.ScopeSchemaURL,
			span.StartTime,
			span.EndTime,
			uint8(span.StatusCode),
			span.StatusMessage,
			attributes,
			resourceAttributes,
			events,
			links,
			span.Flags,
			span.DroppedAttributesCount,
			span.DroppedEventsCount,
			span.DroppedLinksCount,
			span.ResourceDroppedAttributes,
			span.Model,
			span.Input,
			span.Output,
			now,
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

func (s *ClickHouseTraceStore) ListTraces(
	ctx context.Context,
	ownerID string,
	limit int,
) ([]TraceSummary, error) {
	rows, err := s.conn.Query(ctx, `
SELECT
  trace_id,
  argMin(name, tuple(parent_span_id != '', start_time)) AS root_name,
  argMin(service_name, start_time) AS service_name,
  anyIf(model, model != '') AS model,
  min(start_time) AS started_at,
  max(end_time) AS ended_at,
  toInt64(dateDiff('millisecond', min(start_time), max(end_time))) AS duration_ms,
  count() AS span_count,
  countIf(status_code = 2) AS error_count,
  max(inserted_at) AS last_ingested_at
FROM catena_spans FINAL
WHERE owner_id = ?
GROUP BY trace_id
ORDER BY ended_at DESC
LIMIT ?`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TraceSummary, 0)
	for rows.Next() {
		var value TraceSummary
		if err := rows.Scan(
			&value.TraceID,
			&value.RootName,
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
	query := fmt.Sprintf(`
SELECT
  trace_id, root_name, service_name, model, started_at, ended_at,
  duration_ms, span_count, error_count, last_ingested_at
FROM (
  SELECT
    trace_id,
	    anyIf(agent_id, agent_id != '') AS agent_id,
    argMin(name, tuple(parent_span_id != '', start_time)) AS root_name,
    argMin(service_name, tuple(parent_span_id != '', start_time)) AS service_name,
    anyIf(model, model != '') AS model,
    min(start_time) AS started_at,
    max(end_time) AS ended_at,
    toInt64(dateDiff('millisecond', min(start_time), max(end_time))) AS duration_ms,
    count() AS span_count,
    countIf(status_code = 2) AS error_count,
    max(inserted_at) AS last_ingested_at
  FROM catena_spans FINAL
  WHERE owner_id = ? AND end_time >= ? AND start_time <= ?
  GROUP BY trace_id
)
WHERE agent_id = ? OR (agent_id = '' AND %s)
ORDER BY ended_at DESC
LIMIT ?`, legacyFilter)
	args := []any{ownerID, windowStart, windowEnd}
	args = append(args, agentID)
	args = append(args, legacyFilterArgs...)
	args = append(args, limit)
	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TraceSummary, 0)
	for rows.Next() {
		var value TraceSummary
		if err := rows.Scan(
			&value.TraceID,
			&value.RootName,
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
	rows, err := s.conn.Query(ctx, `
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
	rows, err := s.conn.Query(ctx, `
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
	summary := TraceSummary{
		TraceID:      spans[0].TraceID,
		RootName:     spans[0].Name,
		ServiceName:  spans[0].ServiceName,
		StartTime:    spans[0].StartTime,
		EndTime:      spans[0].EndTime,
		SpanCount:    uint64(len(spans)),
		LastIngested: lastIngested,
	}
	for _, span := range spans {
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
