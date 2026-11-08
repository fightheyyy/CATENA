-- Collector Contrib v0.137.0 traces_table.sql / traces_insert.sql compatible.
-- Additive migration; never touches catena_spans. Apply in the Catena database.
CREATE TABLE IF NOT EXISTS otel_traces (
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)),
    TraceState String CODEC(ZSTD(1)),
    SpanName LowCardinality(String) CODEC(ZSTD(1)),
    SpanKind LowCardinality(String) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Duration UInt64 CODEC(ZSTD(1)),
    StatusCode LowCardinality(String) CODEC(ZSTD(1)),
    StatusMessage String CODEC(ZSTD(1)),
    Events Nested (
                      Timestamp DateTime64(9),
    Name LowCardinality(String),
    Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),
    Links Nested (
                     TraceId String,
                     SpanId String,
                     TraceState String,
                     Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),
    INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_res_attr_key mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_res_attr_value mapValues(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_span_attr_key mapKeys(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_span_attr_value mapValues(SpanAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_duration Duration TYPE minmax GRANULARITY 1
,
    CatenaOwnerId String MATERIALIZED ResourceAttributes['catena.owner.id'],
    CatenaAgentId String MATERIALIZED ResourceAttributes['catena.agent.id'],
    CatenaSessionId String MATERIALIZED JSONExtractString(SpanAttributes['catena.internal.meta.v1'], 'session_id'),
    CatenaSessionPriority UInt8 MATERIALIZED JSONExtractUInt(SpanAttributes['catena.internal.meta.v1'], 'session_priority'),
    CatenaInputPreview String MATERIALIZED JSONExtractString(SpanAttributes['catena.internal.meta.v1'], 'input_preview'),
    CatenaModel String MATERIALIZED JSONExtractString(SpanAttributes['catena.internal.meta.v1'], 'model'),
    CatenaIngestedAt DateTime64(9, 'UTC') DEFAULT now64(9)
) ENGINE = ReplacingMergeTree(CatenaIngestedAt)
-- Immutable identity buckets keep duplicate spans together across timestamp edits.
PARTITION BY cityHash64(CatenaOwnerId) % 16
ORDER BY (CatenaOwnerId, TraceId, SpanId)
SETTINGS index_granularity=8192;
