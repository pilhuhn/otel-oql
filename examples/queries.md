# OQL Query Examples

This document contains example OQL queries demonstrating various use cases.

**Note on Syntax:** Pipes (`|`) are completely optional in OQL. Use them for readability or omit them - both work identically.

## Basic Queries

### Find slow spans

```oql
# With pipes (readable)
signal=spans | where duration > 500ms | limit 100

# Without pipes (also valid)
signal=spans where duration > 500ms limit 100
```

### Find error logs

```oql
signal=logs
| where attributes.error == "true"
| limit 50
```

### Find high-value metrics

```oql
signal=metrics
| where metric_name == "http.server.duration" and value > 2000
```

## Trace Expansion

### Expand a single slow trace

```oql
signal=spans
| where name == "checkout_process" and duration > 500ms
| limit 1
| expand trace
```

This returns all spans belonging to the selected trace.

## Cross-Signal Correlation

### Find errors and correlate with logs and metrics

```oql
signal=spans
| where name == "payment_gateway" and attributes.error == "true"
| correlate logs, metrics
```

Returns the matching spans plus correlated logs and metrics with the same trace_id.

## Latency Debugging with Exemplars

### The Wormhole: From Metrics to Traces

```oql
signal=metrics
| where metric_name == "http.server.duration" and value > 5000
| get_exemplars()
| expand trace
| correlate logs
```

Workflow:
1. Find metrics showing high latency (>5s)
2. Extract exemplar trace_ids (the "wormhole")
3. Jump to trace space and expand full trace
4. Correlate with logs for complete context

### Step-by-step Latency Investigation

```oql
# Step 1: Find the spike
signal=metrics
| where metric_name == "http.server.duration" and value > 5000

# Step 2: Extract trace IDs
| extract exemplar.trace_id as bad_trace

# Step 3: Switch to trace space
| switch_context signal=spans
| where trace_id == bad_trace

# Step 4: Expand full trace
| expand trace

# Step 5: Get error logs
| correlate logs
| where attributes.error == "true"
```

## Progressive Refinement

### Initial broad query

```oql
signal=traces
| where attribute.duration > 5s
```

### Refine results (separate request)

```oql
filter attribute.error = true
```

### Further refinement

```oql
filter attribute.service == "payment-service"
```

## Complex Conditions

### AND conditions

```oql
signal=spans
| where name == "api_call" and duration > 1000ms and attributes.status_code == 500
| limit 10
```

### OR conditions

```oql
signal=logs
| where severity_text == "ERROR" or severity_text == "FATAL"
| limit 100
```

## Service-Specific Queries

### Find all database calls over threshold

```oql
signal=spans
| where attributes.db.system == "postgresql" and duration > 100ms
| limit 50
```

### Find HTTP 5xx errors

```oql
signal=spans
| where attributes.http.status_code >= 500 and attributes.http.status_code < 600
| expand trace
```

## Time-Based Queries

### Recent errors

```oql
signal=logs
| where severity_number >= 17 and timestamp > 1679000000000
| limit 100
```

Note: Timestamp is in Unix milliseconds.

## Combining Multiple Operations

### Complete debugging workflow

```oql
signal=spans
| where name == "checkout" and attributes.error == "true"
| limit 5
| expand trace
| correlate logs, metrics
```

This:
1. Finds checkout spans with errors
2. Limits to 5 examples
3. Expands to full traces
4. Correlates with related logs and metrics

## HTTP API Usage

### Query Request Format

```bash
curl -X POST http://localhost:8080/query \
  -H "X-Tenant-ID: 1" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "signal=spans | where duration > 500ms | limit 10"
  }'
```

### Response Format

```json
{
  "results": [
    {
      "sql": "SELECT * FROM otel_spans WHERE tenant_id = 1 AND duration > 500000000 LIMIT 10",
      "columns": ["tenant_id", "trace_id", "span_id", "name", "duration", ...],
      "rows": [
        [1, "abc123...", "def456...", "checkout", 750000000, ...],
        ...
      ],
      "stats": {
        "numDocsScanned": 1500,
        "totalDocs": 10000,
        "timeUsedMs": 45
      }
    }
  ]
}
```

## Tips and Best Practices

1. **Use limits**: Always add `limit` to prevent overwhelming queries
2. **Start specific**: Begin with specific conditions, then expand
3. **Use exemplars**: For debugging latency, exemplars are your friend
4. **Correlate wisely**: Correlating all signals can be expensive
5. **Progressive refinement**: Use filter to refine large result sets
6. **Test mode**: Use `--test-mode` for development without tenant-id headers

## Operator Reference

| Operator | Purpose | Example |
|----------|---------|---------|
| `signal=` | Start query | `signal=spans` |
| `where` | Filter | `where name == "api"` |
| `expand` | Get full trace | `expand trace` |
| `correlate` | Match signals | `correlate logs` |
| `get_exemplars()` | Extract trace_ids | `get_exemplars()` |
| `switch_context` | Change signal | `switch_context signal=logs` |
| `extract` | Select field | `extract trace_id as id` |
| `filter` | Refine results | `filter error == true` |
| `limit` | Limit rows | `limit 100` |
