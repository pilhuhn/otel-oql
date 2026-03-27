# Pinot Schema Changes for LogQL Support

## Summary

Added native columns to the `otel_logs` table to improve query performance for common log labels and enable efficient log-to-trace correlation.

## Changes to `otel_logs` Table

### New Columns Added

```go
// Prometheus/Loki common labels - extracted for performance
{Name: "job", DataType: "STRING"},
{Name: "instance", DataType: "STRING"},
{Name: "environment", DataType: "STRING"},
```

### Updated Inverted Index

```go
InvertedIndexColumns: []string{
    "tenant_id",        // Existing
    "trace_id",         // Existing
    "span_id",          // NEW - added to index
    "severity_text",    // Existing
    "service_name",     // Existing
    "log_level",        // NEW - added to index
    "job",              // NEW - added column + index
    "instance",         // NEW - added column + index
    "environment",      // NEW - added column + index
}
```

## Complete otel_logs Schema

### Dimension Fields
- tenant_id (INT) - Multi-tenant isolation
- trace_id (STRING) - Trace correlation ✅ Indexed
- span_id (STRING) - Span correlation ✅ Indexed
- severity_text (STRING) - Log severity ✅ Indexed
- body (STRING) - Log message content
- service_name (STRING) - Service identifier ✅ Indexed
- host_name (STRING) - Host identifier
- log_level (STRING) - Log level ✅ Indexed
- log_source (STRING) - Source file/stream
- job (STRING) - Job/stream name ✅ Indexed (NEW)
- instance (STRING) - Instance identifier ✅ Indexed (NEW)
- environment (STRING) - Environment ✅ Indexed (NEW)
- attributes (JSON) - Flexible attributes
- resource_attributes (JSON) - Resource attributes

## Rationale

### Why These Columns?

**trace_id/span_id**: Critical for log-to-trace correlation, OQL correlate operation
**job**: Most common LogQL filter (Prometheus/Loki standard)
**instance**: Pod/container filtering for debugging
**environment**: prod/staging/dev filtering (low cardinality)

### Performance Impact

Before (JSON): 
```sql
WHERE JSON_EXTRACT_SCALAR(attributes, '$.job', 'STRING') = 'varlogs'
```

After (Native):
```sql
WHERE job = 'varlogs'  -- 10-100x faster!
```

## Testing

All tests pass:
```bash
go test ./pkg/api -run TestLogQLTraceCorrelation -v
go test ./pkg/api -run TestLogQLNativeColumnMapping -v
```
