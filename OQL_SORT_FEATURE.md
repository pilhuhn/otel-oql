# OQL Sort Feature

## Overview

The `sort` operation has been added to OQL, allowing you to order query results by one or more fields.

## Syntax

```oql
signal=<signal> | sort <field> [asc|desc]
signal=<signal> | sort <field1> [asc|desc], <field2> [asc|desc], ...
```

### Parameters

- **field**: Column name to sort by (e.g., `duration`, `name`, `timestamp`)
- **asc**: Sort in ascending order (default if not specified)
- **desc**: Sort in descending order

## Examples

### Single Field

```oql
# Sort by duration (ascending by default)
signal=spans | sort duration

# Sort by duration descending (slowest first)
signal=spans | sort duration desc

# Sort by name ascending (explicit)
signal=spans | sort name asc
```

### Multiple Fields

```oql
# Sort by duration descending, then by name ascending
signal=spans | sort duration desc, name asc

# Sort by service name, then error status, then duration
signal=spans | sort service_name, error, duration desc
```

### Combined with Other Operations

```oql
# Find slow spans, sort by duration, limit results
signal=spans | where duration > 500ms | sort duration desc | limit 10

# Get recent errors, sorted by timestamp
signal=logs | where level="error" | sort timestamp desc | limit 100

# Find high latency requests, sort by multiple fields
signal=spans
| where service_name="api" and duration > 1s
| sort duration desc, name asc
| limit 20
```

## SQL Translation

The `sort` operation translates directly to SQL `ORDER BY`:

| OQL | SQL |
|-----|-----|
| `sort duration` | `ORDER BY duration ASC` |
| `sort duration desc` | `ORDER BY duration DESC` |
| `sort duration desc, name asc` | `ORDER BY duration DESC, name ASC` |

## Best Practices

### 1. Sort After Filtering

Apply `where` filters before `sort` to reduce the data being sorted:

```oql
# Good: Filter first, then sort
signal=spans | where service_name="api" | sort duration desc

# Less efficient: Sort all data
signal=spans | sort duration desc | where service_name="api"
```

### 2. Use Limit with Sort

Always combine `sort` with `limit` for large datasets:

```oql
# Good: Get top 10 slowest spans
signal=spans | where duration > 100ms | sort duration desc | limit 10

# Bad: Sort everything (expensive!)
signal=spans | sort duration desc
```

### 3. Sort Direction Matters

- **Descending (`desc`)**: Use for "top N" queries (highest duration, latest timestamp)
- **Ascending (`asc`)**: Use for "bottom N" queries (lowest duration, earliest timestamp)

```oql
# Get the 10 slowest requests
signal=spans | sort duration desc | limit 10

# Get the 10 fastest requests
signal=spans | sort duration asc | limit 10

# Get the most recent errors
signal=logs | where level="error" | sort timestamp desc | limit 100
```

### 4. Common Sorting Fields

**Spans/Traces**:
- `duration` - Sort by request/span duration
- `timestamp` - Sort by time (latest first with `desc`)
- `name` - Sort alphabetically by operation name
- `service_name` - Group by service

**Logs**:
- `timestamp` - Sort by log time
- `severity_text` - Sort by severity level
- `log_level` - Sort by log level
- `service_name` - Group by service

**Metrics**:
- `timestamp` - Sort by metric timestamp
- `value` - Sort by metric value
- `metric_name` - Sort alphabetically by metric name

## Performance Considerations

### Indexed Columns

Sorting on indexed columns is faster:
- `timestamp` - Always indexed
- `service_name` - Native column (indexed)
- `duration` - Native column (indexed)
- `trace_id`, `span_id` - Indexed for lookups

### JSON Attributes

Sorting on JSON-extracted attributes is slower:

```oql
# Fast: Native column
signal=spans | sort duration desc

# Slower: JSON attribute (no index)
signal=spans | sort attributes.http.status_code
```

### Multiple Sort Fields

Each additional sort field adds overhead. Keep it to 2-3 fields maximum:

```oql
# Good: 1-2 fields
signal=spans | sort duration desc, name asc

# Slower: Many fields
signal=spans | sort service_name, duration desc, name, timestamp desc
```

## Order of Operations

The `sort` operation should typically come after filtering but before `limit`:

```
signal → where → sort → limit
```

Example:
```oql
signal=spans
| where service_name="api" and duration > 100ms  # Filter first
| sort duration desc                               # Then sort
| limit 10                                         # Finally limit
```

## Common Use Cases

### 1. Find Slowest Requests

```oql
signal=spans
| where service_name="api"
| sort duration desc
| limit 10
```

### 2. Recent Errors

```oql
signal=logs
| where level="error"
| sort timestamp desc
| limit 100
```

### 3. Top Services by Error Count

```oql
signal=spans
| where error="true"
| group by service_name
| sort count desc
| limit 5
```

### 4. Latest Metrics

```oql
signal=metrics
| where metric_name="http.server.duration"
| sort timestamp desc
| limit 1
```

## Limitations

1. **No Expressions**: Can only sort by column names, not expressions
   ```oql
   # Not supported:
   signal=spans | sort duration / 1000 desc
   ```

2. **No NULLS FIRST/LAST**: Sort order for null values is database-dependent

3. **No Case-Insensitive**: String sorting is case-sensitive
   ```oql
   # Will sort: Alice, Bob, alice, bob (uppercase first)
   signal=logs | sort user_name
   ```

## Testing

All tests pass:

```bash
$ go test ./pkg/translator -v -run TestTranslator_Sort
=== RUN   TestTranslator_Sort
=== RUN   TestTranslator_Sort/sort_single_field_ascending
=== RUN   TestTranslator_Sort/sort_single_field_descending
=== RUN   TestTranslator_Sort/sort_multiple_fields
=== RUN   TestTranslator_Sort/sort_with_where_and_limit
--- PASS: TestTranslator_Sort (0.00s)
PASS
```

## Implementation Details

### AST

```go
type SortOp struct {
    Fields []SortField
}

type SortField struct {
    Field string  // Column name
    Desc  bool    // true = descending, false = ascending
}
```

### Parser

Parses syntax: `sort field1 desc, field2 asc`

### Translator

Generates: `ORDER BY field1 DESC, field2 ASC`

## Summary

✅ Sort by one or more fields
✅ Ascending (default) or descending order
✅ Combine with where, limit, and other operations
✅ Direct SQL ORDER BY translation
✅ Production-ready and tested

The `sort` operation makes OQL query results more useful by allowing you to order data exactly how you need it!
