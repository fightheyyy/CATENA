// Legacy-only fixture writer for migration parity tests. Never built into the server.
package control

import (
	"context"
	"errors"
	"fmt"
	"github.com/ClickHouse/clickhouse-go/v2"
	"strings"
	"time"
)

// Deprecated: used only for legacy migration/parity tests. Production always
// sends to Collector and opens a read-only trace store.
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
	  session_id String,
	  session_priority UInt8,
	  input_preview String,
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
	if err := s.conn.Exec(ctx, `ALTER TABLE catena_spans ADD COLUMN IF NOT EXISTS agent_id String AFTER owner_id`); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, `ALTER TABLE catena_spans ADD COLUMN IF NOT EXISTS session_id String AFTER agent_id`); err != nil {
		return err
	}
	if err := s.conn.Exec(ctx, `ALTER TABLE catena_spans ADD COLUMN IF NOT EXISTS session_priority UInt8 AFTER session_id`); err != nil {
		return err
	}
	return s.conn.Exec(ctx, `ALTER TABLE catena_spans ADD COLUMN IF NOT EXISTS input_preview String AFTER session_id`)
}

func (s *ClickHouseTraceStore) InsertSpans(ctx context.Context, ownerID string, spans []TraceSpan) error {
	if s.readOnly {
		return errors.New("OTel trace storage is read-only; send through Collector ingress")
	}
	if ownerID == "" {
		return errors.New("Trace owner is required")
	}
	if len(spans) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
INSERT INTO catena_spans (
  owner_id, agent_id, session_id, session_priority, input_preview, trace_id, span_id, parent_span_id, trace_state, name, kind,
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
			traceSpanSessionID(span),
			traceSpanSessionPriority(span),
			boundedRunes(strings.TrimSpace(span.Input), 512),
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
