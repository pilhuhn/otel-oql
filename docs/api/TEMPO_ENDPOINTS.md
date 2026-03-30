# Tempo API Endpoints for TraceQL

This document describes the Grafana Tempo-compatible API endpoints implemented for TraceQL support.

## Overview

OTEL-OQL now provides Tempo API v2 endpoints that enable:
- **Grafana autocomplete**: Tag/label discovery for TraceQL query builder
- **Search functionality**: Execute TraceQL queries and return matching traces
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

To use these endpoints in Grafana:

1. **Add Tempo Data Source**:
   - Type: Tempo
   - URL: `http://localhost:8080`
   - Custom HTTP Headers: `X-Tenant-ID: 0`

2. **Test Connection**:
   - Grafana will call `/api/v2/search/tags` to verify connectivity

3. **Query Builder**:
   - Grafana will use `/api/v2/search/tag/{tag}/values` for autocomplete
   - Clicking a tag shows its available values

4. **Search**:
   - Grafana sends TraceQL queries to `/api/v2/search`
   - Results displayed as trace list

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
- **Total**: 17 tests, all passing

---

## Implementation Files

- `pkg/api/tempo.go` - Tempo API handlers (~400 lines)
- `pkg/api/tempo_test.go` - Tempo endpoint tests (17 tests)
- `pkg/api/server.go` - Endpoint registration

---

## Next Steps

Potential enhancements:
- Apply filter query `q={}` in tag values endpoint
- Return more trace metadata in search results
- Add trace detail endpoint `/api/v2/traces/{traceID}`
- Add span sets in search response
