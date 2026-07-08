# oql-cli - OTEL-OQL Query Client

A command-line utility for executing OQL (Observability Query Language) queries against the OTEL-OQL service.

## Installation

```bash
go build -o oql-cli ./cmd/oql-cli
```

## Usage

### Basic Query (Command Line)

Execute a query directly from the command line:

```bash
oql-cli --tenant-id=0 "signal=spans limit 10"
```

### Query from stdin

Pipe a query to the CLI:

```bash
echo "signal=spans where duration > 100 limit 5" | oql-cli --tenant-id=0
```

### Interactive Mode

Run the CLI without arguments to enter interactive mode with full command-line editing:

```bash
oql-cli --tenant-id=0
```

Interactive mode features:
- **Command-line editing**: Use arrow keys, backspace, delete
- **History navigation**: Press Up/Down to navigate through previous commands
- **History search**: Press Ctrl+R for reverse-i-search
- **Persistent history**: Commands are saved to `~/.oql_history`

#### History Commands

```bash
!!      # Repeat last command
!h      # Show command history with line numbers
!5      # Run command #5 from history
```

#### Example Session

```
oql> signal=spans limit 10
[results displayed]

oql> !!                           # Repeats: signal=spans limit 10
[results displayed]

oql> !h                           # Show history
Command History:
   1  signal=spans limit 10
   2  signal=spans limit 10

oql> signal=traces where error=true
[results displayed]

oql> [Press Up]                   # Shows previous command
oql> signal=traces where error=true
```

### Verbose Output

Show the generated SQL and query statistics:

```bash
oql-cli --tenant-id=0 --verbose "signal=spans limit 5"
```

Output includes:
- Generated Pinot SQL
- Documents scanned
- Query execution time

### JSON Output

Get raw JSON response for programmatic processing:

```bash
oql-cli --tenant-id=0 --json "signal=spans limit 5" | jq .
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--endpoint` | OTEL-OQL query API endpoint | `http://localhost:8080` |
| `--tenant-id` | Tenant ID for query isolation | `0` |
| `--verbose` | Show verbose output (SQL, stats) | `false` |
| `--json` | Output raw JSON response | `false` |
| `--version` | Show version and exit | - |

## Examples

### Basic Queries

```bash
# Get recent spans
oql-cli --tenant-id=0 "signal=spans since 1h limit 10"

# Query metrics
oql-cli --tenant-id=0 "signal=metrics where metric_name == \"http.server.duration\""

# Query logs with correlation
oql-cli --tenant-id=0 "signal=logs where severity_text == \"ERROR\" correlate spans"
```

### Advanced Queries

```bash
# Aggregation with group by
oql-cli --tenant-id=0 "signal=spans where http_status_code >= 500 group by service_name count()"

# Trace expansion
oql-cli --tenant-id=0 "signal=spans where name == \"checkout\" and duration > 500ms expand trace"

# Time range with aggregation
oql-cli --tenant-id=0 --verbose "signal=spans since 1h group by service_name avg(duration)"
```

### Using Different Endpoints

```bash
# Connect to remote OTEL-OQL instance
oql-cli --endpoint=http://otel-oql.prod.example.com:8080 --tenant-id=42 "signal=spans limit 10"

# Query with different tenant
oql-cli --tenant-id=123 "signal=metrics since 30m"
```

### Scripting with oql-cli

```bash
# Save query to file
cat > query.oql <<EOF
signal=spans
where service_name == "payment-service"
and http_status_code >= 500
since 24h
group by http_status_code
count()
EOF

# Execute saved query
cat query.oql | oql-cli --tenant-id=0 --verbose

# Process results with jq
oql-cli --tenant-id=0 --json "signal=spans limit 100" | \
  jq '.results[0].rows[] | select(.[3] > 1000)'
```

### Debugging

```bash
# Show verbose output to see generated SQL
oql-cli --tenant-id=0 --verbose "signal=spans where duration > 100"

# Get full JSON response for debugging
oql-cli --tenant-id=0 --json "signal=spans limit 1" | jq .
```

## Output Formats

### Table Format (Default)

```
name                duration  http_status_code  service_name
--------------------------------------------------------------
checkout_process    523       200               checkout-service
payment_gateway     1204      200               payment-service
inventory_check     89        200               inventory-service

3 row(s) returned
```

### Verbose Format

```
SQL: SELECT * FROM otel_spans WHERE tenant_id = 0 AND duration > 100 LIMIT 5
Stats: 1523/10000 docs scanned, 12ms

name                duration  http_status_code  service_name
--------------------------------------------------------------
checkout_process    523       200               checkout-service
payment_gateway     1204      200               payment-service

2 row(s) returned
```

### JSON Format

```json
{
  "results": [
    {
      "sql": "SELECT * FROM otel_spans WHERE tenant_id = 0 LIMIT 5",
      "columns": ["trace_id", "span_id", "name", "duration"],
      "rows": [
        ["abc123...", "def456...", "checkout_process", 523],
        ["abc123...", "ghi789...", "payment_gateway", 1204]
      ],
      "stats": {
        "numDocsScanned": 1523,
        "totalDocs": 10000,
        "timeUsedMs": 12
      }
    }
  ]
}
```

## Environment Variables

You can also use environment variables for configuration:

```bash
export OQL_ENDPOINT=http://localhost:8080
export OQL_TENANT_ID=0

# Then use without flags
oql-cli "signal=spans limit 10"
```

Note: Currently environment variables are not implemented. Use flags instead.

## Tips

1. **Multi-line queries**: Use interactive mode (`oql-cli --tenant-id=0`) for better editing of complex queries
2. **Pipes are optional**: Both `signal=spans | limit 10` and `signal=spans limit 10` work
3. **Combine with jq**: Use `--json` flag with `jq` for advanced JSON processing
4. **Save common queries**: Keep frequently-used queries in files and pipe them to `oql-cli`
5. **Verbose mode**: Use `--verbose` to understand the generated SQL and query performance

## Troubleshooting

### Connection Refused

```
Error executing query: failed to execute request: Post "http://localhost:8080/query": dial tcp: connect: connection refused
```

**Solution**: Ensure OTEL-OQL service is running on the specified endpoint.

### Tenant Not Found

```
Query error: tenant-id not found
```

**Solution**: Ensure you're passing `--tenant-id` flag or the service is in test mode.

### Empty Query

```
Error: query cannot be empty
```

**Solution**: Provide a query either as command argument or via stdin.
