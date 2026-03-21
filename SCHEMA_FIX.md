# Schema Implementation Fix Required

## Current Issue

The current `pkg/pinot/schema.go` is incomplete. It only defines:
- Partition configuration
- Index configuration
- Table metadata

**Missing**: Actual column definitions (FieldSpecs)

## Complete Schema Structure Needed

```go
type PinotSchema struct {
    SchemaName string `json:"schemaName"`

    // Dimension columns (filterable, groupable)
    DimensionFieldSpecs []FieldSpec `json:"dimensionFieldSpecs"`

    // Metric columns (aggregatable numbers)
    MetricFieldSpecs []FieldSpec `json:"metricFieldSpecs"`

    // DateTime columns (time-based queries)
    DateTimeFieldSpecs []DateTimeFieldSpec `json:"dateTimeFieldSpecs"`
}

type FieldSpec struct {
    Name             string      `json:"name"`
    DataType         string      `json:"dataType"` // STRING, INT, LONG, DOUBLE, BOOLEAN, JSON
    DefaultNullValue interface{} `json:"defaultNullValue,omitempty"`
}

type DateTimeFieldSpec struct {
    Name             string `json:"name"`
    DataType         string `json:"dataType"`
    Format           string `json:"format"` // e.g., "1:MILLISECONDS:EPOCH"
    Granularity      string `json:"granularity"`
}
```

## Recommended Implementation: Hybrid Approach

### Spans Table Schema

```go
func getSpansSchema() *PinotSchema {
    return &PinotSchema{
        SchemaName: "otel_spans",

        DimensionFieldSpecs: []FieldSpec{
            // Tenant & Identity
            {Name: "tenant_id", DataType: "INT"},
            {Name: "trace_id", DataType: "STRING"},
            {Name: "span_id", DataType: "STRING"},
            {Name: "parent_span_id", DataType: "STRING"},
            {Name: "name", DataType: "STRING"},
            {Name: "kind", DataType: "STRING"},

            // Common OTel Semantic Conventions
            {Name: "service_name", DataType: "STRING"},
            {Name: "http_method", DataType: "STRING"},
            {Name: "http_status_code", DataType: "INT"},
            {Name: "http_route", DataType: "STRING"},
            {Name: "db_system", DataType: "STRING"},
            {Name: "db_statement", DataType: "STRING"},
            {Name: "error", DataType: "BOOLEAN"},

            // Status
            {Name: "status_code", DataType: "STRING"},
            {Name: "status_message", DataType: "STRING"},

            // Flexible attributes (everything else)
            {Name: "attributes", DataType: "JSON"},
            {Name: "resource_attributes", DataType: "JSON"},
        },

        MetricFieldSpecs: []FieldSpec{
            {Name: "duration", DataType: "LONG"}, // nanoseconds
        },

        DateTimeFieldSpecs: []DateTimeFieldSpec{
            {
                Name:        "timestamp",
                DataType:    "LONG",
                Format:      "1:MILLISECONDS:EPOCH",
                Granularity: "1:MILLISECONDS",
            },
        },
    }
}
```

### Metrics Table Schema

```go
func getMetricsSchema() *PinotSchema {
    return &PinotSchema{
        SchemaName: "otel_metrics",

        DimensionFieldSpecs: []FieldSpec{
            {Name: "tenant_id", DataType: "INT"},
            {Name: "metric_name", DataType: "STRING"},
            {Name: "metric_type", DataType: "STRING"}, // gauge, sum, histogram

            // Common metric labels
            {Name: "service_name", DataType: "STRING"},
            {Name: "host_name", DataType: "STRING"},
            {Name: "environment", DataType: "STRING"},

            // Exemplar support (the "wormhole")
            {Name: "exemplar_trace_id", DataType: "STRING"},
            {Name: "exemplar_span_id", DataType: "STRING"},

            // Flexible attributes
            {Name: "attributes", DataType: "JSON"},
            {Name: "resource_attributes", DataType: "JSON"},
        },

        MetricFieldSpecs: []FieldSpec{
            {Name: "value", DataType: "DOUBLE"},
            {Name: "count", DataType: "LONG"},  // for histograms
            {Name: "sum", DataType: "DOUBLE"},  // for histograms
        },

        DateTimeFieldSpecs: []DateTimeFieldSpec{
            {
                Name:        "timestamp",
                DataType:    "LONG",
                Format:      "1:MILLISECONDS:EPOCH",
                Granularity: "1:MILLISECONDS",
            },
        },
    }
}
```

### Logs Table Schema

```go
func getLogsSchema() *PinotSchema {
    return &PinotSchema{
        SchemaName: "otel_logs",

        DimensionFieldSpecs: []FieldSpec{
            {Name: "tenant_id", DataType: "INT"},
            {Name: "trace_id", DataType: "STRING"},
            {Name: "span_id", DataType: "STRING"},
            {Name: "severity_text", DataType: "STRING"},
            {Name: "body", DataType: "STRING"},

            // Common log attributes
            {Name: "service_name", DataType: "STRING"},
            {Name: "log_level", DataType: "STRING"},

            // Flexible attributes
            {Name: "attributes", DataType: "JSON"},
            {Name: "resource_attributes", DataType: "JSON"},
        },

        MetricFieldSpecs: []FieldSpec{
            {Name: "severity_number", DataType: "INT"},
        },

        DateTimeFieldSpecs: []DateTimeFieldSpec{
            {
                Name:        "timestamp",
                DataType:    "LONG",
                Format:      "1:MILLISECONDS:EPOCH",
                Granularity: "1:MILLISECONDS",
            },
        },
    }
}
```

## Ingestion Changes Required

Update `pkg/ingestion/ingester.go` to extract common attributes:

```go
func (i *Ingester) IngestTraces(ctx context.Context, tenantID int, traces ptrace.Traces) error {
    records := make([]map[string]interface{}, 0)

    for k := 0; k < traces.ResourceSpans().Len(); k++ {
        rs := traces.ResourceSpans().At(k)
        resourceAttrs := rs.Resource().Attributes().AsRaw()

        for j := 0; j < rs.ScopeSpans().Len(); j++ {
            ss := rs.ScopeSpans().At(j)

            for idx := 0; idx < ss.Spans().Len(); idx++ {
                span := ss.Spans().At(idx)
                attrs := span.Attributes().AsRaw()

                record := map[string]interface{}{
                    "tenant_id":      tenantID,
                    "trace_id":       span.TraceID().String(),
                    "span_id":        span.SpanID().String(),
                    "parent_span_id": span.ParentSpanID().String(),
                    "name":           span.Name(),
                    "kind":           span.Kind().String(),
                    "duration":       span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Nanoseconds(),
                    "timestamp":      span.StartTimestamp().AsTime().UnixMilli(),
                    "status_code":    span.Status().Code().String(),
                    "status_message": span.Status().Message(),

                    // Extract common semantic convention attributes
                    "service_name":     extractString(resourceAttrs, "service.name"),
                    "http_method":      extractString(attrs, "http.method"),
                    "http_status_code": extractInt(attrs, "http.status_code"),
                    "http_route":       extractString(attrs, "http.route"),
                    "db_system":        extractString(attrs, "db.system"),
                    "db_statement":     extractString(attrs, "db.statement"),
                    "error":            extractBool(attrs, "error"),

                    // Store remaining attributes as JSON
                    "attributes":          removeKnownKeys(attrs),
                    "resource_attributes": removeKnownKeys(resourceAttrs),
                }

                records = append(records, record)
            }
        }
    }

    return i.client.Insert(ctx, "otel_spans", records)
}

// Helper functions
func extractString(attrs map[string]interface{}, key string) interface{} {
    if v, ok := attrs[key]; ok {
        return v
    }
    return nil // Pinot handles NULL
}

func extractInt(attrs map[string]interface{}, key string) interface{} {
    if v, ok := attrs[key]; ok {
        // Handle type conversion if needed
        return v
    }
    return nil
}

func extractBool(attrs map[string]interface{}, key string) interface{} {
    if v, ok := attrs[key]; ok {
        if b, ok := v.(bool); ok {
            return b
        }
        // Handle string "true"/"false" if needed
        if s, ok := v.(string); ok {
            return s == "true"
        }
    }
    return nil
}

func removeKnownKeys(attrs map[string]interface{}) map[string]interface{} {
    // Create a copy without the extracted keys
    result := make(map[string]interface{})
    knownKeys := map[string]bool{
        "service.name": true,
        "http.method": true,
        "http.status_code": true,
        "http.route": true,
        "db.system": true,
        "db.statement": true,
        "error": true,
    }

    for k, v := range attrs {
        if !knownKeys[k] {
            result[k] = v
        }
    }

    return result
}
```

## OQL Translator Changes

Update `pkg/translator/translator.go` to handle both native columns and JSON:

```go
func (t *Translator) translateBinaryCondition(cond *oql.BinaryCondition) (string, error) {
    field := cond.Left

    // Check if it's a known native column
    nativeColumns := map[string]bool{
        "http_status_code": true,
        "http_method": true,
        "service_name": true,
        "error": true,
        // ... etc
    }

    // Handle attribute access
    if strings.HasPrefix(field, "attributes.") {
        parts := strings.TrimPrefix(field, "attributes.")

        // Check if it's been extracted to a native column
        if nativeColumns[parts] {
            field = parts
        } else {
            // Use JSON_EXTRACT for other attributes
            field = fmt.Sprintf("JSON_EXTRACT(attributes, '$.%s')", parts)
        }
    }

    valueStr := t.formatValue(cond.Right)
    return fmt.Sprintf("%s %s %s", field, cond.Operator, valueStr), nil
}
```

## Benefits of This Approach

1. **Fast common queries**: `WHERE http_status_code = 500` uses native index
2. **Flexible uncommon queries**: `WHERE attributes.custom_field = 'value'` works via JSON
3. **Future-proof**: New semantic conventions can be added as native columns
4. **Efficient storage**: Common fields compressed better as typed columns
5. **Query optimization**: Pinot can optimize native column queries much better

## Migration Path

1. **Phase 1**: Implement complete schema with JSON columns only (simple, works immediately)
2. **Phase 2**: Add extraction for top 10-20 most common OTel semantic conventions
3. **Phase 3**: Monitor query patterns, promote frequently-queried attributes to native columns

## References

- [OTel Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
- [Pinot JSON Functions](https://docs.pinot.apache.org/users/user-guide-query/supported-transformations#json-functions)
- [Pinot Schema Documentation](https://docs.pinot.apache.org/configuration-reference/schema)
