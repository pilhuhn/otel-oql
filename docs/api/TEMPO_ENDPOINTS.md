# Tempo API Endpoints for TraceQL

This document describes the Grafana Tempo-compatible API endpoints implemented for TraceQL support.

## Overview

OTEL-OQL now provides Tempo API v2 endpoints that enable:
- **Grafana autocomplete**: Tag/label discovery for TraceQL query builder
- **Search functionality**: Execute TraceQL queries and return matching traces
- **Trace retrieval**: Fetch detailed trace data by trace ID
- **Native performance**: 10-100x faster queries using indexed columns

## Available Endpoints

### 1. `/api/echo` - Health Check

Simple health check endpoint used by Grafana to test datasource connectivity.

**Request**:
```bash
GET /api/echo
```

**Response**:
```json
{
  "message": "ok"
}
```

**Example**:
```bash
curl 'http://localhost:8080/api/echo'
```

**Note**: This endpoint does **not** require authentication (no `X-Tenant-ID` header needed).

---

### 2. `/api/search` - Tempo v1 Search & Metadata

Tempo v1 search endpoint used by Grafana to get trace metadata (service names, operation names) and search results.

**Request**:
```bash
GET /api/search?q={}&limit=20&start=1774833777&end=1774855377
```

**Parameters**:
- `q` (optional): TraceQL query string (e.g., `{duration > 100ms}`, or `{}` for metadata)
- `limit` (optional): Maximum number of traces to return (default: 20)
- `start` (optional): Unix timestamp (seconds) for time range start
- `end` (optional): Unix timestamp (seconds) for time range end

**Response when `q={}` (metadata request)**:
```json
{
  "traces": [],
  "metadata": {
    "serviceNames": ["api", "checkout", "payment"],
    "operationNames": ["HTTP GET", "HTTP POST", "checkout_process"]
  }
}
```

**Response when executing a query**:
```json
{
  "traces": [
    {
      "traceID": "abc123...",
      "rootServiceName": "api",
      "rootTraceName": "HTTP GET",
      "startTimeUnixNano": "1774832350000000000",
      "durationMs": 125
    }
  ]
}
```

**Examples**:
```bash
# Get metadata (service names and operation names)
curl 'http://localhost:8080/api/search?q=%7B%7D&start=1774833777&end=1774855377' \
  -H 'X-Tenant-ID: 0'

# Search for slow traces
curl 'http://localhost:8080/api/search?q={duration > 1s}&limit=20' \
  -H 'X-Tenant-ID: 0'

# Search for HTTP 500 errors
curl 'http://localhost:8080/api/search?q={span.http.status_code = 500}' \
  -H 'X-Tenant-ID: 0'
```

**Note**: This is the **v1 API** endpoint. Grafana uses this for the search UI and to populate service/operation dropdowns. For v2 API, use `/api/v2/search`.

---

### 3. `/api/v2/search` - Execute TraceQL Queries

Main search endpoint for executing TraceQL queries (v2 API).

**Request**:
```bash
GET /api/v2/search?q={duration > 100ms}&start=1774832350&end=1774853950
```

**Parameters**:
- `q` (required): TraceQL query string (e.g., `{duration > 100ms}`)
- `start` (optional): Unix timestamp (seconds) for time range start
- `end` (optional): Unix timestamp (seconds) for time range end

**Response**:
```json
{
  "traces": [
    {
      "traceID": "abc123...",
      "rootServiceName": "api",
      "rootTraceName": "HTTP GET",
      "startTimeUnixNano": 1774832350000000000,
      "durationMs": 125
    }
  ]
}
```

**Example**:
```bash
# Find slow traces
curl 'http://localhost:8080/api/v2/search?q={duration > 1s}' \
  -H 'X-Tenant-ID: 0'

# Find HTTP 500 errors
curl 'http://localhost:8080/api/v2/search?q={span.http.status_code = 500}' \
  -H 'X-Tenant-ID: 0'

# Find traces for specific service
curl 'http://localhost:8080/api/v2/search?q={resource.service.name = "checkout"}' \
  -H 'X-Tenant-ID: 0'
```

---

### 4. `/api/v2/search/tags` - List Available Tags

Returns list of all available TraceQL tags/fields for autocomplete.

**Request**:
```bash
GET /api/v2/search/tags
```

**Response**:
```json
{
  "tagNames": [
    "name",
    "duration",
    "status",
    "kind",
    "span.http.method",
    "span.http.status_code",
    "span.db.system",
    "resource.service.name"
  ]
}
```

**Example**:
```bash
curl 'http://localhost:8080/api/v2/search/tags' \
  -H 'X-Tenant-ID: 0'
```

**Available Tags**:

**Intrinsic fields** (built-in span fields):
- `name` - Span name
- `duration` - Span duration (nanoseconds)
- `status` - Span status (unset, ok, error)
- `kind` - Span kind (client, server, internal, etc.)

**Common span attributes** (OTel semantic conventions):
- `span.http.method` - HTTP method (GET, POST, etc.)
- `span.http.status_code` - HTTP status code (200, 500, etc.)
- `span.http.route` - HTTP route pattern
- `span.http.target` - HTTP target URL
- `span.db.system` - Database system (postgresql, mysql, etc.)
- `span.db.statement` - Database query statement
- `span.messaging.system` - Messaging system (kafka, rabbitmq, etc.)
- `span.messaging.destination` - Message queue/topic name
- `span.rpc.service` - RPC service name
- `span.rpc.method` - RPC method name
- `span.error` - Error flag (true/false)

**Resource attributes**:
- `resource.service.name` - Service name

---

### 5. `/api/v2/search/tag/{tagName}/values` - Get Tag Values

Returns distinct values for a specific tag, used for autocomplete dropdowns.

**Request**:
```bash
GET /api/v2/search/tag/{tagName}/values?q={}&start=1774832350&end=1774853950
```

**Parameters**:
- `{tagName}`: Tag name (e.g., `name`, `span.http.method`, `resource.service.name`)
- `q` (optional): TraceQL filter query (currently not applied, reserved for future)
- `start` (optional): Unix timestamp (seconds) for time range start
- `end` (optional): Unix timestamp (seconds) for time range end

**Response**:
```json
{
  "tagValues": [
    "HTTP GET",
    "HTTP POST",
    "checkout_process",
    "payment_gateway"
  ]
}
```

**Examples**:
```bash
# Get all span names
curl 'http://localhost:8080/api/v2/search/tag/name/values' \
  -H 'X-Tenant-ID: 0'

# Get all HTTP methods used
curl 'http://localhost:8080/api/v2/search/tag/span.http.method/values' \
  -H 'X-Tenant-ID: 0'

# Get all HTTP status codes
curl 'http://localhost:8080/api/v2/search/tag/span.http.status_code/values' \
  -H 'X-Tenant-ID: 0'

# Get all service names
curl 'http://localhost:8080/api/v2/search/tag/resource.service.name/values' \
  -H 'X-Tenant-ID: 0'

# Get all database systems used
curl 'http://localhost:8080/api/v2/search/tag/span.db.system/values' \
  -H 'X-Tenant-ID: 0'
```

---

### 6. `/api/traces/{traceID}` and `/api/v2/traces/{traceID}` - Get Trace by ID

Returns detailed trace data for a specific trace ID, including all spans in the trace.

**Content Negotiation**: Both endpoints support dual format based on the `Accept` header:
- **Protobuf**: `Accept: application/protobuf` - Returns OTLP binary protobuf (used by Grafana)
- **JSON**: No Accept header or `Accept: application/json` - Returns JSON (used by Perses, curl, etc.)

**Request**:
```bash
# v1 endpoint (returns batches format)
GET /api/traces/{traceID}

# v2 endpoint (returns resourceSpans format)
GET /api/v2/traces/{traceID}
```

**Parameters**:
- `{traceID}`: Trace ID (e.g., `d246e356447cd0508dc66c42103ec0ed`)
- **Header** `Accept`: Optional - `application/protobuf` for binary protobuf, `application/json` or omitted for JSON

**Response for v1** (`/api/traces/{traceID}` - Tempo format):
```json
{
  "batches": [
    {
      "resource": {
        "attributes": [
          {
            "key": "service.name",
            "value": {
              "stringValue": "api"
            }
          }
        ]
      },
      "scopeSpans": [
        {
          "scope": {},
          "spans": [...]
        }
      ]
    }
  ]
}
```

**Response for v2** (`/api/v2/traces/{traceID}` - OTLP format):
```json
{
  "resourceSpans": [
    {
      "resource": {
        "attributes": [
          {
            "key": "service.name",
            "value": {
              "stringValue": "api"
            }
          }
        ]
      },
      "scopeSpans": [
        {
          "scope": {},
          "spans": [
            {
              "traceID": "b06e5ee8f5b5a34c87808e09834cbff3",
              "spanID": "abc123",
              "parentSpanID": "",
              "name": "HTTP GET /checkout",
              "startTimeUnixNano": "1774832350000000000",
              "durationNanos": "125000000",
              "attributes": [
                {
                  "key": "http.method",
                  "value": {
                    "stringValue": "GET"
                  }
                },
                {
                  "key": "http.status_code",
                  "value": {
                    "intValue": "200"
                  }
                }
              ],
              "status": {
                "code": "STATUS_CODE_OK"
              }
            }
          ]
        }
      ]
    }
  ]
}
```

**Examples**:
```bash
# Get trace by ID (v1 endpoint - JSON format)
curl 'http://localhost:8080/api/traces/d246e356447cd0508dc66c42103ec0ed' \
  -H 'X-Tenant-ID: 0'
# Returns: {"batches": [...]} (JSON)

# Get trace by ID (v2 endpoint - JSON format)
curl 'http://localhost:8080/api/v2/traces/d246e356447cd0508dc66c42103ec0ed' \
  -H 'X-Tenant-ID: 0'
# Returns: {"resourceSpans": [...]} (JSON)

# Get trace by ID (protobuf format - for Grafana)
curl 'http://localhost:8080/api/v2/traces/d246e356447cd0508dc66c42103ec0ed' \
  -H 'X-Tenant-ID: 0' \
  -H 'Accept: application/protobuf'
# Returns: OTLP binary protobuf (Content-Type: application/protobuf)
```

**Response Format Differences**:

Both endpoints return OTLP-compatible JSON with attribute values wrapped in type-specific objects:
- String values: `{"stringValue": "..."}`
- Integer values: `{"intValue": 123}`
- Boolean values: `{"boolValue": true}`
- Double values: `{"doubleValue": 1.23}`

Both use the same nested structure, just with different top-level field names:

**v1** (`/api/traces/{id}`):
- Top-level field: `batches` (Tempo v1 format)
- Each batch contains: `resource`, `scopeSpans[]`
- Each scopeSpan contains: `scope`, `spans`

**v2** (`/api/v2/traces/{id}`):
- Top-level field: `resourceSpans` (OTLP format)
- Each resourceSpan contains: `resource`, `scopeSpans[]`
- Each scopeSpan contains: `scope`, `spans`

The internal structure is identical - only the top-level wrapper differs.

**Native Column Attributes**:

The endpoint automatically includes attributes from native indexed columns:
- `http.method` → `http_method` column (stringValue)
- `http.status_code` → `http_status_code` column (intValue)
- `db.system` → `db_system` column (stringValue)
- `db.statement` → `db_statement` column (stringValue)
- `messaging.system` → `messaging_system` column (stringValue)
- `messaging.destination` → `messaging_destination` column (stringValue)
- `rpc.service` → `rpc_service` column (stringValue)
- `rpc.method` → `rpc_method` column (stringValue)

Custom attributes stored in the JSON `attributes` column are also included.

---

## Performance Optimizations

### Native Column Mapping

For maximum performance, common tags are mapped to native indexed Pinot columns:

| Tag | Pinot Column | Performance Boost |
|-----|--------------|-------------------|
| `name` | `name` | 10-100x faster |
| `duration` | `duration` | 10-100x faster |
| `status` | `status_code` | 10-100x faster |
| `span.http.method` | `http_method` | 10-100x faster |
| `span.http.status_code` | `http_status_code` | 10-100x faster |
| `span.db.system` | `db_system` | 10-100x faster |
| `resource.service.name` | `service_name` | 10-100x faster |

Custom tags not in the above list use JSON extraction, which is slower but still functional:
```
span.custom.field → JSON_EXTRACT_SCALAR(attributes, '$.custom.field', 'STRING')
```

---

## Debug Logging

Enable debug logging to see query translation and execution:

```bash
# Via command-line flags
./otel-oql --debug-query --debug-translation

# Or via global debug flag (enables all debug)
./otel-oql --debug

# Or via environment variables
DEBUG_QUERY=true DEBUG_TRANSLATION=true ./otel-oql
```

**Debug output example**:
```
[DEBUG QUERY] Tempo search (tenant_id=0, q={duration > 100ms}, start=2026-03-30..., end=2026-03-30...)
[DEBUG TRANSLATION] TraceQL query translated to 1 SQL statements:
[DEBUG TRANSLATION]   [1] SELECT * FROM otel_spans WHERE tenant_id = 0 AND duration > 100000000 ORDER BY "timestamp" DESC
```

---

## Grafana Integration

**Status**: ✅ Search works | ⚠️ Trace detail has issues

To use these endpoints in Grafana:

1. **Add Tempo Data Source**:
   - Type: Tempo
   - URL: `http://localhost:8080`
   - Custom HTTP Headers: `X-Tenant-ID: 0`

2. **Test Connection**:
   - Grafana will call `/api/v2/search/tags` to verify connectivity
   - ✅ **Working**

3. **Query Builder**:
   - Grafana will use `/api/v2/search/tag/{tag}/values` for autocomplete
   - Clicking a tag shows its available values
   - ✅ **Working**

4. **Search**:
   - Grafana sends TraceQL queries to `/api/v2/search`
   - Results displayed as trace list with correct timestamps and durations
   - ✅ **Working**

5. **Trace Detail**:
   - Clicking a trace fetches `/api/v2/traces/{traceID}` with `Accept: application/protobuf`
   - ⚠️ **Currently fails with "unexpected EOF"** - Grafana expects Tempo's proprietary protobuf format
   - **Workaround**: Use Perses or Jaeger UI for trace visualization

## Perses Integration

**Status**: ✅ Fully working

Perses (https://perses.dev/) works perfectly with the JSON API:

1. **Configure Tempo Datasource**:
   - URL: `http://localhost:8080`
   - Headers: `X-Tenant-ID: 0`

2. **All features working**:
   - ✅ Search traces
   - ✅ View trace details (waterfall)
   - ✅ Correct timestamps and durations
   - ✅ Span attributes display properly

---

## Multi-Tenancy

All endpoints enforce tenant isolation:
- **gRPC**: Set `tenant-id` metadata
- **HTTP**: Set `X-Tenant-ID` header
- In test mode (`--test-mode`), defaults to `tenant-id=0`

**Example with tenant ID**:
```bash
curl 'http://localhost:8080/api/v2/search?q={duration > 100ms}' \
  -H 'X-Tenant-ID: 42'
```

---

## Testing

Run the Tempo endpoint tests:
```bash
go test ./pkg/api -run TestTempo -v
```

**Test coverage**:
- Tag-to-column mapping: 11 tests
- SQL generation: 3 tests
- TraceQL query execution: 3 tests
- V1 metadata: 1 test
- Trace by ID: 1 test
- Echo endpoint: 1 test
- **Total**: 20 tests, all passing

---

## Implementation Files

- `pkg/api/tempo.go` - Tempo API handlers (~1200+ lines)
- `pkg/api/tempo_test.go` - Tempo endpoint tests (20 tests)
- `pkg/api/server.go` - Endpoint registration

---

## Content Negotiation (JSON vs Protobuf)

The trace detail endpoints (`/api/traces/{id}` and `/api/v2/traces/{id}`) support **dual-format responses** based on the `Accept` header:

⚠️ **Known Limitation**: Grafana's Tempo datasource currently encounters "unexpected EOF" errors with our OTLP protobuf format. Grafana likely expects Tempo's proprietary protobuf format (AGPL-licensed), which we cannot implement directly. **Use Perses or Jaeger UI for trace visualization**, or stick with JSON-based tools.

### Protobuf Format (Grafana)

**When**: Client sends `Accept: application/protobuf` header

**Response**: OTLP binary protobuf (`Content-Type: application/protobuf`)

**Structure**: OpenTelemetry Protocol (OTLP) TracesData protobuf message
- TraceID and SpanID as bytes (hex-decoded)
- Timestamps as uint64 nanoseconds
- Attributes as KeyValue with AnyValue protobuf messages

**Example**:
```bash
curl 'http://localhost:8080/api/v2/traces/{traceID}' \
  -H 'Accept: application/protobuf' \
  -H 'X-Tenant-ID: 0'
# Returns: binary protobuf data
```

**Used by**: Grafana Tempo datasource

### JSON Format (Perses, curl, browsers)

**When**: No Accept header or `Accept: application/json`

**Response**: OTLP-compatible JSON (`Content-Type: application/json`)

**Structure**: Same as protobuf but serialized as JSON
- TraceID and SpanID as hex strings
- Timestamps as string nanoseconds (v1) or uint64 (v2)
- Attributes with type wrappers (stringValue, intValue, etc.)

**Example**:
```bash
curl 'http://localhost:8080/api/v2/traces/{traceID}' \
  -H 'X-Tenant-ID: 0'
# Returns: {"resourceSpans": [...]}
```

**Used by**: Perses, curl, web browsers, custom clients

### Implementation

The server detects the `Accept` header and routes accordingly:

```go
acceptHeader := r.Header.Get("Accept")
if strings.Contains(acceptHeader, "application/protobuf") {
    // Return OTLP protobuf binary
    tracesData := s.transformToOTLPProtobuf(results[0])
    data, _ := proto.Marshal(tracesData)
    w.Header().Set("Content-Type", "application/protobuf")
    w.Write(data)
} else {
    // Return JSON
    trace := s.transformToTempoTraceV2(results[0])
    writeJSON(w, http.StatusOK, trace)
}
```

This allows the same endpoint to serve both Grafana (protobuf) and Perses (JSON) clients.

---

## Pinot Type Handling

**Important**: Pinot returns numeric columns as `float64` values in Go, even for columns defined as `BIGINT` or `INT`.

This affects timestamp and duration extraction in all Tempo endpoints. The implementation uses comprehensive type handling:

```go
var tsMillis int64
if tsVal := span["timestamp"]; tsVal != nil {
    switch v := tsVal.(type) {
    case int64:
        tsMillis = v
    case int:
        tsMillis = int64(v)
    case float64:
        tsMillis = int64(v)  // Pinot returns float64!
    case string:
        if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
            tsMillis = parsed
        }
    }
}
```

**Why This Matters**:
- Naive `span["timestamp"].(int64)` type assertions fail with Pinot data
- Results in zero timestamps ("epoch" in UIs)
- Must handle all numeric types for robust extraction

**Affected Endpoints**:
- `/api/search` - Search results timestamps and durations
- `/api/v2/search` - Search results timestamps and durations
- `/api/traces/{id}` - Span timestamps and durations
- `/api/v2/traces/{id}` - Span timestamps and durations

---

## Next Steps

Potential enhancements:
- Apply filter query `q={}` in tag values endpoint
- Return more trace metadata in search results
- Add span sets in search response
- Add trace comparison endpoint
- Implement trace streaming for large traces
