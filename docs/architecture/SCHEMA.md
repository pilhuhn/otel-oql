# Pinot Schema Documentation

## Overview

OTEL-OQL uses a **hybrid schema approach** for Apache Pinot tables, balancing query performance with flexibility. Common OpenTelemetry semantic conventions are extracted to native indexed columns, while custom attributes remain in JSON columns.

**Performance Impact**: Native columns are 10-100x faster than JSON extraction.

## Schema Design Principles

1. **Native Columns for Common Attributes**: High-cardinality, frequently-queried fields
2. **JSON Columns for Flexibility**: Custom/uncommon attributes
3. **Tenant Partitioning**: All tables partitioned by `tenant_id`
4. **REALTIME Tables**: Kafka-based streaming ingestion
5. **Inverted Indexes**: On all filterable native columns

## Table Schemas

### otel_spans Table

**Purpose**: Store distributed trace span data

```json
{
  "schemaName": "otel_spans",
  "dimensionFieldSpecs": [
    // Tenant & Identity
    {"name": "tenant_id", "dataType": "INT"},
    {"name": "trace_id", "dataType": "STRING"},
    {"name": "span_id", "dataType": "STRING"},
    {"name": "parent_span_id", "dataType": "STRING"},
    {"name": "name", "dataType": "STRING"},
    {"name": "kind", "dataType": "STRING"},

    // Service Context
    {"name": "service_name", "dataType": "STRING"},

    // HTTP Semantic Conventions
    {"name": "http_method", "dataType": "STRING"},
    {"name": "http_status_code", "dataType": "INT"},
    {"name": "http_route", "dataType": "STRING"},

    // Status
    {"name": "status_code", "dataType": "STRING"},
    {"name": "status_message", "dataType": "STRING"},

    // Flexible Attributes
    {"name": "attributes", "dataType": "JSON"},
    {"name": "resource_attributes", "dataType": "JSON"}
  ],
  "metricFieldSpecs": [
    {"name": "duration", "dataType": "LONG"}
  ],
  "dateTimeFieldSpecs": [
    {
      "name": "timestamp",
      "dataType": "LONG",
      "format": "1:MILLISECONDS:EPOCH",
      "granularity": "1:MILLISECONDS"
    }
  ]
}
```

**Inverted Indexes**:
```go
InvertedIndexColumns: []string{
    "tenant_id",
    "trace_id",
    "span_id",
    "service_name",
    "http_status_code",
    "http_method",
}
```

### otel_metrics Table

**Purpose**: Store OpenTelemetry metrics (gauge, sum, histogram)

```json
{
  "schemaName": "otel_metrics",
  "dimensionFieldSpecs": [
    // Tenant & Identity
    {"name": "tenant_id", "dataType": "INT"},
    {"name": "metric_name", "dataType": "STRING"},
    {"name": "metric_type", "dataType": "STRING"},

    // Service Context
    {"name": "service_name", "dataType": "STRING"},
    {"name": "host_name", "dataType": "STRING"},

    // Prometheus/Loki Common Labels (for PromQL compatibility)
    {"name": "job", "dataType": "STRING"},
    {"name": "instance", "dataType": "STRING"},
    {"name": "environment", "dataType": "STRING"},

    // Exemplar Support (The "Wormhole")
    {"name": "exemplar_trace_id", "dataType": "STRING"},
    {"name": "exemplar_span_id", "dataType": "STRING"},

    // Flexible Attributes
    {"name": "attributes", "dataType": "JSON"},
    {"name": "resource_attributes", "dataType": "JSON"}
  ],
  "metricFieldSpecs": [
    {"name": "value", "dataType": "DOUBLE"},
    {"name": "count", "dataType": "LONG"},
    {"name": "sum", "dataType": "DOUBLE"}
  ],
  "dateTimeFieldSpecs": [
    {
      "name": "timestamp",
      "dataType": "LONG",
      "format": "1:MILLISECONDS:EPOCH",
      "granularity": "1:MILLISECONDS"
    }
  ]
}
```

**Inverted Indexes**:
```go
InvertedIndexColumns: []string{
    "tenant_id",
    "metric_name",
    "service_name",
    "job",
    "instance",
    "environment",
    "exemplar_trace_id",
}
```

### otel_logs Table

**Purpose**: Store OpenTelemetry log events

```json
{
  "schemaName": "otel_logs",
  "dimensionFieldSpecs": [
    // Tenant & Identity
    {"name": "tenant_id", "dataType": "INT"},

    // Trace Correlation (CRITICAL for log-to-trace correlation!)
    {"name": "trace_id", "dataType": "STRING"},
    {"name": "span_id", "dataType": "STRING"},

    // Severity
    {"name": "severity_text", "dataType": "STRING"},
    {"name": "log_level", "dataType": "STRING"},

    // Content
    {"name": "body", "dataType": "STRING"},

    // Service Context
    {"name": "service_name", "dataType": "STRING"},
    {"name": "host_name", "dataType": "STRING"},
    {"name": "log_source", "dataType": "STRING"},

    // Prometheus/Loki Common Labels (for LogQL compatibility)
    {"name": "job", "dataType": "STRING"},
    {"name": "instance", "dataType": "STRING"},
    {"name": "environment", "dataType": "STRING"},

    // Flexible Attributes
    {"name": "attributes", "dataType": "JSON"},
    {"name": "resource_attributes", "dataType": "JSON"}
  ],
  "metricFieldSpecs": [
    {"name": "severity_number", "dataType": "INT"}
  ],
  "dateTimeFieldSpecs": [
    {
      "name": "timestamp",
      "dataType": "LONG",
      "format": "1:MILLISECONDS:EPOCH",
      "granularity": "1:MILLISECONDS"
    }
  ]
}
```

**Inverted Indexes**:
```go
InvertedIndexColumns: []string{
    "tenant_id",
    "trace_id",        // Critical for OQL correlate operation
    "span_id",         // Critical for log-to-trace correlation
    "severity_text",
    "service_name",
    "log_level",
    "job",             // LogQL performance (10-100x faster)
    "instance",        // LogQL performance
    "environment",     // LogQL performance
}
```

## Native Column Selection Rationale

### Why These Columns?

**Tenant Isolation**:
- `tenant_id` (INT) - Partitioning and filtering, absolute requirement

**Trace Correlation** (logs table):
- `trace_id`, `span_id` - Enable instant log-to-trace correlation
- Critical for OQL `correlate` operation
- Without native columns: `JSON_EXTRACT_SCALAR(attributes, '$.trace_id')` ~100ms
- With native columns: `WHERE trace_id = 'abc123'` ~10ms (10x faster!)

**Prometheus/Loki Compatibility** (metrics & logs):
- `job`, `instance`, `environment` - Most common labels in PromQL/LogQL queries
- Grafana dashboards heavily rely on these labels
- Performance: `WHERE job = 'api'` vs `WHERE JSON_EXTRACT(..., '$.job') = 'api'`

**Exemplar Support** (metrics table):
- `exemplar_trace_id`, `exemplar_span_id` - The "wormhole" from aggregation space to event space
- Enables debugging "which specific request caused this spike?"
- Required for OQL `get_exemplars()` operation

**HTTP Semantic Conventions** (spans table):
- `http_method`, `http_status_code`, `http_route` - Most common span filters
- 90% of trace queries filter on HTTP attributes
- Example: Find all 500 errors in /api/checkout

**Severity Filtering** (logs table):
- `severity_text`, `log_level` - Most common log filters
- Example: `{level="error"}` in LogQL

## Performance Impact

### Query Performance Comparison

**Native Column Query** (~10ms):
```sql
SELECT * FROM otel_logs
WHERE tenant_id = 0
  AND job = 'varlogs'
  AND level = 'error'
LIMIT 100
```

**JSON Extraction Query** (~100ms):
```sql
SELECT * FROM otel_logs
WHERE tenant_id = 0
  AND JSON_EXTRACT_SCALAR(attributes, '$.job', 'STRING') = 'varlogs'
  AND JSON_EXTRACT_SCALAR(attributes, '$.level', 'STRING') = 'error'
LIMIT 100
```

**Performance Ratio**: 10-100x faster with native columns!

### Storage Impact

- **Native columns**: Better compression, smaller storage footprint
- **JSON columns**: More flexible, slightly larger storage
- **Overall impact**: Negligible - native columns compress well

## Schema Evolution

### Adding New Native Columns

When adding a new native column:

1. **Update schema in `pkg/pinot/schema.go`**
2. **Update ingestion in `pkg/ingestion/ingester.go`** to extract the field
3. **Update translator in `pkg/querylangs/common/matcher.go`** to map label to column
4. **Add to inverted index** if filterable
5. **Write tests** to verify native column is used (not JSON extraction)

Example from LogQL implementation:

```go
// pkg/querylangs/common/matcher.go
func GetLogNativeColumn(labelName string) string {
    nativeColumns := map[string]string{
        "job":         "job",         // NEW
        "instance":    "instance",    // NEW
        "environment": "environment", // NEW
        "trace_id":    "trace_id",    // NEW
        "span_id":     "span_id",     // NEW
        // ... existing mappings
    }
    if nativeCol, ok := nativeColumns[labelName]; ok {
        return nativeCol
    }
    return "" // Use JSON extraction
}
```

### Migration Strategy

See [MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md) for detailed migration instructions when schema changes require Pinot table recreation.

**Summary**:
1. **No migration path**: Pinot REALTIME tables must be dropped and recreated
2. **Data loss**: All data in existing tables will be lost
3. **Recommendation**: Add columns in initial deployment, avoid schema changes in production

## Common Patterns

### Hybrid Query Translation

OQL/PromQL/LogQL translators check for native columns first, fall back to JSON:

```go
func translateLabelMatcher(labelName, operator, value string) string {
    // Check if native column exists
    if nativeCol := GetNativeColumn(labelName); nativeCol != "" {
        // Use native column (fast!)
        return fmt.Sprintf("%s %s %s", nativeCol, operator, value)
    }

    // Fall back to JSON extraction (slower)
    return fmt.Sprintf(
        "JSON_EXTRACT_SCALAR(attributes, '$.%s', 'STRING') %s %s",
        labelName, operator, value
    )
}
```

### Attribute Extraction in Ingestion

Extract known attributes to native columns, store rest in JSON:

```go
func transformSpan(span ptrace.Span) map[string]interface{} {
    attrs := span.Attributes().AsRaw()

    return map[string]interface{}{
        // Native columns (extracted)
        "http_method":      extractString(attrs, "http.method"),
        "http_status_code": extractInt(attrs, "http.status_code"),
        "service_name":     extractString(attrs, "service.name"),

        // JSON column (remaining attributes)
        "attributes": removeKnownKeys(attrs, []string{
            "http.method",
            "http.status_code",
            "service.name",
        }),
    }
}
```

## Testing Schema Changes

Always test that native columns are used instead of JSON extraction:

```go
func TestLogQLNativeColumnUsage(t *testing.T) {
    translator := logql.NewTranslator(0)
    sql, _ := translator.TranslateQuery(`{job="varlogs"}`)

    // Verify native column is used
    if !strings.Contains(sql, "job = 'varlogs'") {
        t.Error("Expected native column 'job'")
    }

    // Verify JSON extraction is NOT used
    if strings.Contains(sql, "JSON_EXTRACT_SCALAR(attributes, '$.job'") {
        t.Error("Should use native column, not JSON extraction!")
    }
}
```

See `pkg/api/logql_trace_correlation_test.go` for comprehensive native column verification tests.

## References

- [Apache Pinot Schema Reference](https://docs.pinot.apache.org/configuration-reference/schema)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
- [Pinot JSON Functions](https://docs.pinot.apache.org/users/user-guide-query/supported-transformations#json-functions)
- [MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md) - Schema migration instructions
- [LOGQL_SUPPORT.md](./LOGQL_SUPPORT.md) - LogQL native column usage examples
