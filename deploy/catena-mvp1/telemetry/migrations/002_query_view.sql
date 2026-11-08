-- Deduplicate at read time, before every aggregate or span page.
-- FINAL does not wait for background merges. Owner is a leading sorting key.
CREATE VIEW IF NOT EXISTS catena_otel_spans AS
SELECT
  CatenaOwnerId AS owner_id,
  CatenaAgentId AS agent_id,
  TraceId AS trace_id,
  SpanId AS span_id,
  ParentSpanId AS parent_span_id,
  TraceState AS trace_state,
  SpanName AS name,
  toUInt8(transform(CAST(SpanKind AS String), ['Unspecified','Internal','Server','Client','Producer','Consumer'], [0,1,2,3,4,5], 0)) AS kind,
  ServiceName AS service_name,
  ScopeName AS scope_name,
  ScopeVersion AS scope_version,
  Timestamp AS start_time,
  Timestamp + toIntervalNanosecond(Duration) AS end_time,
  toUInt8(transform(CAST(StatusCode AS String), ['Unset','Ok','Error'], [0,1,2], 0)) AS status_code,
  StatusMessage AS status_message,
  CatenaIngestedAt AS inserted_at,
  JSONExtractString(SpanAttributes['catena.internal.meta.v1'], 'resource_schema_url') AS resource_schema_url,
  JSONExtractString(SpanAttributes['catena.internal.meta.v1'], 'scope_schema_url') AS scope_schema_url,
  CatenaModel AS model,
  JSONExtractString(SpanAttributes['catena.internal.meta.v1'], 'input') AS input,
  JSONExtractString(SpanAttributes['catena.internal.meta.v1'], 'output') AS output,
  CatenaSessionId AS session_id,
  CatenaInputPreview AS input_preview,
  JSONExtractRaw(SpanAttributes['catena.internal.meta.v1'], 'attributes') AS attributes_json,
  JSONExtractRaw(SpanAttributes['catena.internal.meta.v1'], 'resource_attributes') AS resource_attributes_json,
  JSONExtractRaw(SpanAttributes['catena.internal.meta.v1'], 'events') AS events_json,
  JSONExtractRaw(SpanAttributes['catena.internal.meta.v1'], 'links') AS links_json,
  toUInt32(JSONExtractUInt(SpanAttributes['catena.internal.meta.v1'], 'flags')) AS flags,
  toUInt32(JSONExtractUInt(SpanAttributes['catena.internal.meta.v1'], 'dropped_attributes_count')) AS dropped_attributes_count,
  toUInt32(JSONExtractUInt(SpanAttributes['catena.internal.meta.v1'], 'dropped_events_count')) AS dropped_events_count,
  toUInt32(JSONExtractUInt(SpanAttributes['catena.internal.meta.v1'], 'dropped_links_count')) AS dropped_links_count,
  toUInt32(JSONExtractUInt(SpanAttributes['catena.internal.meta.v1'], 'resource_dropped_attributes_count')) AS resource_dropped_attributes,
  CatenaSessionPriority AS session_priority
FROM otel_traces FINAL
WHERE CatenaOwnerId != '';
