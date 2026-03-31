# OQL (Observability Query Language) Reference

## Overview

OQL is a query language designed for observability data that enables powerful cross-signal correlation and analysis. Pipes (`|`) are completely optional - use them for readability or omit them entirely.

## Signal Types

Start every query by specifying which signal to query:

```oql
signal=spans      # Trace spans
signal=metrics    # Metrics
signal=logs       # Log records
signal=traces     # Alias for spans
```

---

## Core Operations

### 1. `where` - Filter Data

Filter data based on conditions.

**Comparison Operators**: `==`, `!=`, `>`, `<`, `>=`, `<=`
**Logical Operators**: `and`, `or`

```oql
# Simple filter
signal=spans where duration > 500ms

# Multiple conditions with AND
signal=spans where name == "checkout" and http_status_code >= 500

# OR conditions
signal=logs where severity == "ERROR" or severity == "FATAL"

# Nested attribute access
signal=spans where attributes.user_id == "12345"
```

### 2. `limit` - Limit Results

Limit the number of rows returned.

```oql
signal=spans where duration > 1s limit 100
```

### 3. `sort` - Order Results

Sort query results by one or more fields in ascending or descending order.

```oql
# Sort by duration (ascending by default)
signal=spans | sort duration

# Sort by duration descending (slowest first)
signal=spans | sort duration desc

# Sort by multiple fields
signal=spans | sort duration desc, name asc

# Combined with where and limit
signal=spans | where duration > 500ms | sort duration desc | limit 10

# Get most recent errors
signal=logs | where level="error" | sort timestamp desc | limit 100
```

**Syntax**: `sort field1 [asc|desc], field2 [asc|desc], ...`
- **asc**: Ascending order (default)
- **desc**: Descending order

### 4. `expand trace` - Reconstruct Full Traces

Fetch all spans sharing the same `trace_id` (the "magic" operator for trace reconstruction).

```oql
# Find a slow span, then get entire trace
signal=spans where name == "checkout" and duration > 500ms limit 1 expand trace
```

### 5. `correlate` - Cross-Signal Correlation

Find matching data from other signals based on `trace_id`.

```oql
# Find errors and get correlated logs and metrics
signal=spans where attributes.error == "true" correlate logs, metrics

# Find slow requests and correlate with logs
signal=spans where duration > 5s correlate logs
```

### 6. `extract` - Extract Field Values

Extract a specific field value into an alias for later use.

```oql
# Extract exemplar trace_id for debugging
signal=metrics where value > 5000 extract exemplar.trace_id as bad_trace
```

### 7. `switch_context` - Jump Between Signals

Explicitly switch from one signal type to another.

```oql
# Start with metrics, switch to spans
signal=metrics where metric_name == "http.server.duration" and value > 5000
switch_context signal=spans
where trace_id == {extracted_id}
```

### 8. `filter` - Progressive Refinement

Refine existing result set without starting a new query (useful for interactive exploration).

```oql
# First query
signal=spans where duration > 5s

# Then refine (in a follow-up query)
filter attributes.error == true
```

### 9. `get_exemplars()` - The Wormhole

Extract exemplar `trace_ids` from aggregated metrics - this is the "wormhole" from aggregation space to event space.

```oql
# Find slow metrics and get their exemplar trace_ids
signal=metrics where metric_name == "http.server.duration" and value > 2s get_exemplars()

# Full wormhole: metrics → traces → logs
signal=metrics where value > 5000 get_exemplars() expand trace correlate logs
```

---

## Aggregation Operations

### 10. `avg` / `min` / `max` / `sum` / `count`

Aggregate data with statistical functions.

```oql
# Average duration
signal=spans avg(duration)

# With alias
signal=spans avg(duration) as avg_duration

# Count all spans
signal=spans count()

# Count specific field
signal=spans count(name)

# Min/max duration
signal=spans min(duration)
signal=spans max(duration)

# Sum of values
signal=metrics sum(value)
```

### 11. `group by` - Group Results

Group data by one or more fields (typically used with aggregations).

```oql
# Average duration by service
signal=spans group by service_name avg(duration)

# Count by status code
signal=spans group by http_status_code count()

# Multiple grouping fields
signal=spans group by service_name, http_method avg(duration)
```

---

## Time Range Operations

### 12. `since` - Relative Time Range

Filter data from a relative time in the past.

```oql
# Last hour
signal=spans since 1h

# Last 30 minutes
signal=logs since 30m

# Last 2 days
signal=metrics since 48h

# Specific date
signal=spans since 2024-03-20
```

### 13. `between` - Absolute Time Range

Filter data between two specific timestamps.

```oql
# Between specific dates
signal=spans between 2024-03-20 and 2024-03-21

# With timestamps
signal=logs between 2024-03-20T10:00:00 and 2024-03-20T11:00:00
```

---

## Complete Examples

### Example 1: Basic Latency Investigation

```oql
# Find slow requests in the last hour
signal=spans where duration > 1s since 1h limit 100
```

### Example 2: Error Correlation

```oql
# Find errors and correlate with logs and metrics
signal=spans where http_status_code >= 500 correlate logs, metrics
```

### Example 3: Trace Reconstruction

```oql
# Find a specific slow trace and expand it
signal=spans where name == "payment_process" and duration > 2s limit 1 expand trace
```

### Example 4: The Wormhole - Metrics to Traces

```oql
# Find latency spike in metrics, jump to traces
signal=metrics
where metric_name == "http.server.duration" and value > 5000
get_exemplars()
expand trace
correlate logs
```

### Example 5: Aggregation by Service

```oql
# Average response time by service
signal=spans since 1h group by service_name avg(duration) as avg_response_time
```

### Example 6: Error Rate Analysis

```oql
# Count errors by endpoint
signal=spans
where http_status_code >= 500
since 24h
group by http_route
count() as error_count
```

### Example 7: Progressive Investigation

```oql
# Step 1: Find slow traces
signal=spans where duration > 5s since 1h

# Step 2 (follow-up query): Refine to errors only
filter attributes.error == true

# Step 3: Expand one trace
limit 1 expand trace

# Step 4: Get correlated logs
correlate logs
```

### Example 8: Complex Latency Debugging

```oql
# Full debugging workflow
signal=metrics
where metric_name == "http.server.duration" and value > 5000ms
since 1h
extract exemplar.trace_id as bad_trace
switch_context signal=spans
where trace_id == bad_trace
expand trace
correlate logs
where severity == "ERROR"
```

### Example 9: Database Performance Analysis

```oql
# Find slow database queries
signal=spans
where db_system == "postgresql" and duration > 500ms
since 6h
group by db_statement
avg(duration) as avg_query_time
limit 20
```

### Example 10: Time-Based Analysis

```oql
# Compare morning vs afternoon traffic
signal=spans
between 2024-03-20T08:00:00 and 2024-03-20T12:00:00
group by service_name
count() as morning_requests
```

---

## Query Syntax Notes

### Pipes are Optional

Both syntaxes are equivalent:

```oql
# With pipes (readable)
signal=spans | where duration > 500ms | limit 100

# Without pipes (also valid)
signal=spans where duration > 500ms limit 100
```

### Field Access

- **Native columns**: Direct access (e.g., `duration`, `http_status_code`, `service_name`)
- **Attributes**: Use dot notation (e.g., `attributes.user_id`, `attributes.custom_field`)
- **Resource attributes**: Use dot notation (e.g., `resource_attributes.host.name`)

### Value Types

- **Strings**: Use quotes `"value"` or `'value'`
- **Numbers**: Plain numbers `500`, `1.5`
- **Durations**: Use suffixes `500ms`, `1s`, `5m`, `2h`
- **Booleans**: `true`, `false`
- **Timestamps**: ISO format `2024-03-20T10:00:00` or date `2024-03-20`

---

## Operator Precedence

Operations are evaluated left-to-right, except for logical operators:

1. Comparison operators (`==`, `>`, etc.)
2. `AND` (higher precedence)
3. `OR` (lower precedence)

Use parentheses in complex conditions (handled automatically by parser for `and`/`or`).

---

## Signal-Specific Fields

### Spans
- `trace_id`, `span_id`, `parent_span_id`
- `name`, `kind`, `status_code`
- `duration`, `timestamp`
- `service_name`, `http_status_code`, `http_method`, `http_route`
- `db_system`, `db_statement`
- `messaging_system`, `messaging_destination`
- `error` (boolean)

### Metrics
- `metric_name`, `value`
- `service_name`, `job`, `instance`
- `exemplar_trace_id`, `exemplar_span_id` (the wormhole!)

### Logs
- `trace_id`, `span_id`
- `body`, `severity_text`, `severity_number`
- `service_name`

All signals have:
- `tenant_id` (automatically filtered)
- `timestamp` (milliseconds since epoch)
- `attributes` (JSON object with custom fields)
- `resource_attributes` (JSON object with resource fields)

---

## Best Practices

1. **Start Broad, Then Refine**: Use `where` to narrow down, then `filter` to refine
2. **Use Time Ranges**: Always add `since` or `between` for better performance
3. **Limit Results**: Use `limit` to avoid overwhelming queries
4. **Leverage Correlate**: Don't query signals separately - use `correlate` for related data
5. **Use the Wormhole**: When debugging latency spikes, `get_exemplars()` is your friend
6. **Group for Analysis**: Combine `group by` with aggregations for insights
7. **Expand Strategically**: Use `expand trace` on a small set (`limit 1`) before expanding

---

## Common Patterns

### Debugging a Latency Spike
```oql
signal=metrics where value > threshold get_exemplars() expand trace correlate logs
```

### Finding Error Patterns
```oql
signal=spans where http_status_code >= 500 since 1h group by http_route count()
```

### Service Health Check
```oql
signal=spans since 5m group by service_name avg(duration) as avg_latency
```

### Trace Forensics
```oql
signal=spans where trace_id == "known-trace-id" expand trace correlate logs, metrics
```
