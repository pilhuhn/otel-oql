# OQL now() Function Examples

The `now()` function enables dynamic time-based queries in OQL, automatically using the current timestamp at query execution time.

## Basic now() Usage

### Get current spans
```oql
signal=spans | where timestamp > now()
```

Translates to:
```sql
SELECT * FROM otel_spans WHERE tenant_id = 0 AND "timestamp" > now()
```

### Get spans from the last hour
```oql
signal=spans | where timestamp > now() - 1h
```

Translates to:
```sql
SELECT * FROM otel_spans WHERE tenant_id = 0 AND "timestamp" > (now() - 3600000)
```

### Get logs from the last 30 minutes
```oql
signal=logs | where timestamp >= now() - 30m
```

Translates to:
```sql
SELECT * FROM otel_logs WHERE tenant_id = 0 AND "timestamp" >= (now() - 1800000)
```

### Get metrics from the last 5 seconds
```oql
signal=metrics | where timestamp > now() - 5s
```

Translates to:
```sql
SELECT * FROM otel_metrics WHERE tenant_id = 0 AND "timestamp" > (now() - 5000)
```

## Time Ranges

### Recent time window (last hour to now)
```oql
signal=spans | where timestamp > now() - 1h and timestamp < now()
```

### Between two relative times (2 hours ago to 1 hour ago)
```oql
signal=logs | where timestamp >= now() - 2h and timestamp <= now() - 1h
```

## Future Queries (Scheduled Events)

### Events scheduled within the next hour
```oql
signal=spans | where timestamp < now() + 1h
```

### Events between now and 5 minutes in the future
```oql
signal=logs | where timestamp >= now() and timestamp <= now() + 5m
```

## Combining with Other Conditions

### Recent errors (last 30 minutes)
```oql
signal=spans | where name == "checkout" and timestamp > now() - 30m
```

Translates to:
```sql
SELECT * FROM otel_spans 
WHERE tenant_id = 0 
  AND (name = 'checkout' AND "timestamp" > (now() - 1800000))
```

### Recent high-severity logs
```oql
signal=logs | where log_level == "error" and timestamp > now() - 5m
```

### OR condition with time filter
```oql
signal=logs | where log_level == "error" or timestamp > now() - 5m
```

## Supported Duration Units

- `ns` - nanoseconds
- `us` - microseconds  
- `ms` - milliseconds
- `s` - seconds
- `m` - minutes
- `h` - hours

## Complex Durations

```oql
# 1 hour and 30 minutes ago
signal=spans | where timestamp > now() - 1h30m

# 2 hours, 15 minutes, and 30 seconds ago
signal=logs | where timestamp > now() - 2h15m30s
```

## Note on Pinot Translation

All durations are converted to milliseconds in the generated Pinot SQL since Pinot's `now()` function returns milliseconds since epoch.

Examples:
- `1h` → `3600000` milliseconds
- `30m` → `1800000` milliseconds
- `5s` → `5000` milliseconds
- `100ms` → `100` milliseconds
