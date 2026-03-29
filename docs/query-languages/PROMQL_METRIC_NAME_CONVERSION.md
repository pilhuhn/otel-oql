# Metric Name Conversion for PromQL/OTel Compatibility

## Problem

OpenTelemetry uses dots in metric names (e.g., `jvm.memory.used`), but PromQL only allows underscores and colons in metric names. This creates a conflict:

1. **Database**: Stores metrics as `jvm.memory.used` (OTel format)
2. **Grafana autocomplete**: Gets `jvm.memory.used` from label values endpoint
3. **PromQL parser**: Rejects `jvm.memory.used` with parse error "unexpected character: '.'"

## Solution

**Bidirectional metric name conversion**:

### Query Translation (PromQL → Database)
When translating PromQL queries to SQL:
- **Input**: `jvm_memory_used` (PromQL format with underscores)
- **Output**: `metric_name = 'jvm.memory.used'` (OTel format with dots)

**Implementation** (`pkg/promql/translator.go`):
```go
func (t *Translator) translateMetricName(normalizedName string) string {
    // Convert PromQL format (underscores) to OTel format (dots)
    // Example: jvm_memory_used → jvm.memory.used
    return strings.ReplaceAll(normalizedName, "_", ".")
}
```

### Label Discovery (Database → PromQL)
When returning metric names from `/api/v1/label/__name__/values`:
- **Database**: Returns `jvm.memory.used` (OTel format)
- **Response**: Converts to `jvm_memory_used` (PromQL format)

**Implementation** (`pkg/api/prometheus.go`):
```go
if labelName == "__name__" {
    convertedValues := make([]string, len(response.Data))
    for i, value := range response.Data {
        convertedValues[i] = convertOTelToPromQLMetricName(value)
    }
    response.Data = convertedValues
}

func convertOTelToPromQLMetricName(otelName string) string {
    return strings.ReplaceAll(otelName, ".", "_")
}
```

## Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Grafana requests: GET /api/v1/label/__name__/values      │
└─────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Database returns:                                        │
│    ["jvm.memory.used", "http.server.duration"]              │
└─────────────────────────────────────────────────────────────┘
                                │
                                ▼ (conversion: dots → underscores)
┌─────────────────────────────────────────────────────────────┐
│ 3. API returns:                                             │
│    ["jvm_memory_used", "http_server_duration"]              │
└─────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Grafana autocomplete shows: jvm_memory_used              │
└─────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. User types query: jvm_memory_used                        │
└─────────────────────────────────────────────────────────────┘
                                │
                                ▼ (conversion: underscores → dots)
┌─────────────────────────────────────────────────────────────┐
│ 6. SQL generated: metric_name = 'jvm.memory.used'           │
└─────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────┐
│ 7. Database query succeeds! ✅                              │
└─────────────────────────────────────────────────────────────┘
```

## Both Formats Supported

The system handles both input formats gracefully:

| User Input | Normalization | Database Query | Result |
|------------|---------------|----------------|--------|
| `jvm_memory_used` | None (already underscores) | `metric_name = 'jvm.memory.used'` | ✅ Works |
| `jvm.memory.used` | Dots → underscores | `metric_name = 'jvm.memory.used'` | ✅ Works |

## Test Coverage

### Unit Tests
- `pkg/api/prometheus_metric_name_conversion_test.go`: Tests `convertOTelToPromQLMetricName()`
- `pkg/promql/normalize_test.go`: Tests metric name normalization
- `pkg/promql/translator_test.go`: Verifies SQL generation with converted names

### Integration Tests
- `pkg/promql/metric_name_integration_test.go`:
  - `TestMetricNameConversion_PromQLToOTel`: Verifies underscore → dot conversion
  - `TestMetricNameConversion_OTelInput`: Verifies dot preservation
  - `TestMetricNameConversion_Grafana`: Simulates full Grafana workflow

## Key Files Modified

1. **pkg/promql/translator.go**:
   - Added `normalizeMetricNames()`: Pre-processes query to handle dots before parsing
   - Modified `translateMetricName()`: Converts underscores to dots for database queries

2. **pkg/api/prometheus.go**:
   - Added `convertOTelToPromQLMetricName()`: Converts dots to underscores for API responses
   - Modified `handlePrometheusLabelValues()`: Applies conversion for `__name__` label

## Examples

### PromQL Query with Underscores
```promql
jvm_memory_used{area="heap"}
```
Generates SQL:
```sql
SELECT * FROM otel_metrics
WHERE tenant_id = 0
  AND metric_name = 'jvm.memory.used'
  AND JSON_EXTRACT_SCALAR(attributes, '$.area', 'STRING') = 'heap'
```

### PromQL Query with Dots (also works!)
```promql
jvm.memory.used{area="heap"}
```
Generates the same SQL:
```sql
SELECT * FROM otel_metrics
WHERE tenant_id = 0
  AND metric_name = 'jvm.memory.used'
  AND JSON_EXTRACT_SCALAR(attributes, '$.area', 'STRING') = 'heap'
```

### Label Discovery Response
Database has:
```json
["jvm.memory.used", "http.server.duration", "system.cpu.utilization"]
```

API returns (converted to PromQL format):
```json
{
  "status": "success",
  "data": [
    "jvm_memory_used",
    "http_server_duration",
    "system_cpu_utilization"
  ]
}
```

## Result

✅ Grafana autocomplete works with PromQL-compatible metric names
✅ PromQL queries parse successfully
✅ Database queries use correct OTel metric names
✅ Both `jvm_memory_used` and `jvm.memory.used` inputs work correctly
✅ Seamless integration between PromQL tooling and OTel data
