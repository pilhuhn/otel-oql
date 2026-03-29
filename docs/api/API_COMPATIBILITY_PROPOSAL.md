# API Endpoint Compatibility Proposal

## Summary

OTEL-OQL currently has a custom API (`POST /query` with JSON body) that is **incompatible** with standard Prometheus and Grafana Loki datasources. This proposal adds Prometheus and Loki-compatible endpoints alongside the existing API to enable seamless Grafana integration.

## Current State

**Endpoint**: `POST /query`

**Request Format**:
```json
{
  "query": "http_requests_total{job=\"api\"}",
  "language": "promql"
}
```

**Response Format**:
```json
{
  "results": [
    {
      "sql": "SELECT...",
      "columns": [...],
      "rows": [...],
      "stats": {...}
    }
  ]
}
```

**Problems**:
- ❌ Not compatible with Grafana Prometheus datasource
- ❌ Not compatible with Grafana Loki datasource
- ❌ Requires custom Grafana plugin for integration
- ❌ Doesn't support time ranges (`start`, `end`, `step`)
- ❌ Doesn't distinguish instant vs range queries
- ❌ Uses JSON instead of form-encoded parameters

## Prometheus API Standard

### Instant Query: `/api/v1/query`

**Method**: GET or POST (form-encoded)

**Parameters**:
- `query` - PromQL expression (required)
- `time` - Evaluation timestamp (optional, defaults to now)
- `timeout` - Evaluation timeout (optional)

**Example Request**:
```bash
curl 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=http_requests_total{job="api"}' \
  --data-urlencode 'time=2024-03-20T10:00:00Z'
```

**Response Format**:
```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {"job": "api", "__name__": "http_requests_total"},
        "value": [1710928800, "1234"]
      }
    ]
  }
}
```

### Range Query: `/api/v1/query_range`

**Method**: GET or POST (form-encoded)

**Parameters**:
- `query` - PromQL expression (required)
- `start` - Start timestamp (required)
- `end` - End timestamp (required)
- `step` - Query resolution step width (required)
- `timeout` - Evaluation timeout (optional)

**Example Request**:
```bash
curl 'http://localhost:9090/api/v1/query_range' \
  --data-urlencode 'query=rate(http_requests_total[5m])' \
  --data-urlencode 'start=2024-03-20T10:00:00Z' \
  --data-urlencode 'end=2024-03-20T11:00:00Z' \
  --data-urlencode 'step=15s'
```

**Response Format**:
```json
{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": {"job": "api"},
        "values": [
          [1710928800, "10.5"],
          [1710928815, "12.3"],
          [1710928830, "11.8"]
        ]
      }
    ]
  }
}
```

## Grafana Loki API Standard

### Instant Query: `/loki/api/v1/query`

**Method**: GET or POST (form-encoded)

**Parameters**:
- `query` - LogQL expression (required)
- `time` - Evaluation timestamp (optional, defaults to now)
- `limit` - Max number of entries (optional, default: 100)
- `direction` - `forward` or `backward` (optional, default: backward)

**Example Request**:
```bash
curl 'http://localhost:3100/loki/api/v1/query' \
  --data-urlencode 'query={job="varlogs"} |= "error"' \
  --data-urlencode 'limit=1000'
```

**Response Format**:
```json
{
  "status": "success",
  "data": {
    "resultType": "streams",
    "result": [
      {
        "stream": {"job": "varlogs", "level": "error"},
        "values": [
          ["1710928800000000000", "database connection timeout"],
          ["1710928801000000000", "authentication failed"]
        ]
      }
    ]
  }
}
```

### Range Query: `/loki/api/v1/query_range`

**Method**: GET or POST (form-encoded)

**Parameters**:
- `query` - LogQL expression (required)
- `start` - Start timestamp (required)
- `end` - End timestamp (required)
- `limit` - Max number of entries (optional)
- `step` - Query resolution (optional, for metric queries)
- `interval` - Only for metric queries (optional)
- `direction` - `forward` or `backward` (optional)

**Example Request**:
```bash
curl 'http://localhost:3100/loki/api/v1/query_range' \
  --data-urlencode 'query=sum by (level) (count_over_time({job="varlogs"}[5m]))' \
  --data-urlencode 'start=1710928800' \
  --data-urlencode 'end=1710932400' \
  --data-urlencode 'step=300'
```

**Response Format** (for metric queries):
```json
{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": {"level": "error"},
        "values": [
          [1710928800, "45"],
          [1710929100, "52"]
        ]
      }
    ]
  }
}
```

## Proposed Changes

### 1. Add Prometheus-Compatible Endpoints

Add two new endpoints that follow Prometheus API standard:

```
GET|POST /api/v1/query          # Instant queries
GET|POST /api/v1/query_range    # Range queries
```

**Handler Logic**:
1. Parse form-encoded parameters (instead of JSON)
2. Detect language from endpoint (PromQL assumed)
3. Translate PromQL → Pinot SQL (reuse existing translator)
4. Execute query against Pinot
5. Transform results to Prometheus response format
6. Include tenant-id from HTTP header (existing middleware)

### 2. Add Loki-Compatible Endpoints

Add two new endpoints that follow Loki API standard:

```
GET|POST /loki/api/v1/query        # Instant queries
GET|POST /loki/api/v1/query_range  # Range queries
```

**Handler Logic**:
1. Parse form-encoded parameters
2. Detect language from endpoint (LogQL assumed)
3. Translate LogQL → Pinot SQL (reuse existing translator)
4. Execute query against Pinot
5. Transform results to Loki response format
6. Include tenant-id from HTTP header

### 3. Keep Existing `/query` Endpoint

**No changes** to existing endpoint - maintains backward compatibility for:
- OQL queries
- Custom integrations
- CLI tool

### 4. Response Format Transformers

Create utility functions to transform Pinot results:

```go
// pkg/api/formats/prometheus.go
func TransformToPrometheusInstant(pinotResults []PinotResult) PrometheusResponse
func TransformToPrometheusRange(pinotResults []PinotResult) PrometheusResponse

// pkg/api/formats/loki.go
func TransformToLokiStreams(pinotResults []PinotResult) LokiResponse
func TransformToLokiMatrix(pinotResults []PinotResult) LokiResponse
```

### 5. Time Range Handling

Add support for time range parameters:

```go
// Prometheus: start, end, step
// Loki: start, end, step (for metrics), limit, direction

// Translators need to inject time range filters into SQL:
WHERE timestamp >= ? AND timestamp <= ?
```

## Benefits

### Grafana Integration

With compatible endpoints, Grafana datasource configuration becomes trivial:

**Prometheus Datasource** (for metrics):
```yaml
datasources:
  - name: OTEL-OQL-Metrics
    type: prometheus
    url: http://localhost:8080
    jsonData:
      httpHeaderName1: X-Tenant-ID
    secureJsonData:
      httpHeaderValue1: "0"
```

**Loki Datasource** (for logs):
```yaml
datasources:
  - name: OTEL-OQL-Logs
    type: loki
    url: http://localhost:8080
    jsonData:
      httpHeaderName1: X-Tenant-ID
    secureJsonData:
      httpHeaderValue1: "0"
```

### Dashboard Compatibility

- ✅ Existing Prometheus dashboards work immediately
- ✅ Existing Loki dashboards work immediately
- ✅ No custom Grafana plugin required
- ✅ Standard Explore UI works
- ✅ Alerting rules work (if implemented)

### Migration Path

Users can migrate from Prometheus/Loki to OTEL-OQL by:
1. Changing datasource URL
2. Adding tenant-id header
3. No query changes needed

## Implementation Plan

### Phase 1: Core API Handlers (Week 1)

**Files to Create**:
- `pkg/api/prometheus.go` - Prometheus endpoint handlers
- `pkg/api/loki.go` - Loki endpoint handlers
- `pkg/api/formats/prometheus.go` - Response transformers
- `pkg/api/formats/loki.go` - Response transformers
- `pkg/api/params.go` - Form parameter parsing

**Files to Modify**:
- `pkg/api/server.go` - Register new endpoints

**Key Functions**:
```go
func (s *Server) handlePrometheusQuery(w http.ResponseWriter, r *http.Request)
func (s *Server) handlePrometheusQueryRange(w http.ResponseWriter, r *http.Request)
func (s *Server) handleLokiQuery(w http.ResponseWriter, r *http.Request)
func (s *Server) handleLokiQueryRange(w http.ResponseWriter, r *http.Request)
```

### Phase 2: Time Range Support (Week 1)

**Files to Modify**:
- `pkg/promql/translator.go` - Add time range injection
- `pkg/logql/translator.go` - Add time range injection

**Logic**:
```go
// Parse time parameters
start, end, step := parseTimeParams(r)

// Inject into WHERE clause
WHERE timestamp >= ? AND timestamp <= ?

// For range queries, adjust query resolution based on step
```

### Phase 3: Response Transformers (Week 1)

**Transform Pinot Results**:

```go
// Prometheus instant query
{columns: ["metric_name", "value", "timestamp", "job"], rows: [...]}
  ↓
{status: "success", data: {resultType: "vector", result: [...]}}

// Prometheus range query
{columns: ["metric_name", "value", "timestamp", "job"], rows: [...]}
  ↓
{status: "success", data: {resultType: "matrix", result: [...]}}

// Loki streams
{columns: ["timestamp", "body", "job", "level"], rows: [...]}
  ↓
{status: "success", data: {resultType: "streams", result: [...]}}
```

### Phase 4: Testing (Week 1-2)

**Test Categories**:
1. Unit tests for parameter parsing
2. Unit tests for response transformers
3. Integration tests with real Pinot queries
4. Grafana datasource testing (manual)
5. Dashboard import testing (manual)

**Test Files**:
- `pkg/api/prometheus_test.go`
- `pkg/api/loki_test.go`
- `pkg/api/formats/prometheus_test.go`
- `pkg/api/formats/loki_test.go`
- `pkg/integration/grafana_test.go`

### Phase 5: Documentation (Week 2)

**Files to Create/Update**:
- `GRAFANA_INTEGRATION.md` - Grafana datasource setup guide
- `PROMETHEUS_API.md` - Prometheus API documentation
- `LOKI_API.md` - Loki API documentation
- Update `README.md` with Grafana quickstart

## Compatibility Notes

### Tenant Isolation

All endpoints require `X-Tenant-ID` header (or test mode):

```bash
# Prometheus query with tenant
curl 'http://localhost:8080/api/v1/query' \
  -H 'X-Tenant-ID: 123' \
  --data-urlencode 'query=http_requests_total'

# Loki query with tenant
curl 'http://localhost:8080/loki/api/v1/query' \
  -H 'X-Tenant-ID: 123' \
  --data-urlencode 'query={job="varlogs"}'
```

### Unsupported Features

Some Prometheus/Loki features won't be supported initially:

**Prometheus**:
- ❌ `/api/v1/labels` - List all labels
- ❌ `/api/v1/label/<name>/values` - List label values
- ❌ `/api/v1/series` - Find series by label matchers
- ❌ `/api/v1/metadata` - Metric metadata
- ❌ `/api/v1/rules` - Recording/alerting rules
- ❌ `/api/v1/alerts` - Active alerts

**Loki**:
- ❌ `/loki/api/v1/labels` - List all labels
- ❌ `/loki/api/v1/label/<name>/values` - List label values
- ❌ `/loki/api/v1/series` - Find series
- ❌ `/loki/api/v1/tail` - Streaming logs
- ❌ `/loki/api/v1/push` - Log ingestion (already handled via OTLP)

These can be added in future phases if needed.

### Error Response Format

Standard error format for both APIs:

```json
{
  "status": "error",
  "errorType": "bad_data",
  "error": "parse error at char 15: invalid syntax"
}
```

**Error Types**:
- `bad_data` - Invalid query syntax
- `timeout` - Query timeout
- `execution` - Query execution error
- `internal` - Internal server error

## Endpoint Summary

After implementation, OTEL-OQL will expose:

```
# OQL (existing)
POST /query                          # Multi-language query (OQL/PromQL/LogQL)

# Prometheus-compatible
GET|POST /api/v1/query              # PromQL instant query
GET|POST /api/v1/query_range        # PromQL range query

# Loki-compatible
GET|POST /loki/api/v1/query         # LogQL instant query
GET|POST /loki/api/v1/query_range   # LogQL range query

# Metadata (future)
GET /api/v1/labels                  # List all labels (Prometheus)
GET /api/v1/label/<name>/values     # List label values (Prometheus)
GET /loki/api/v1/labels             # List all labels (Loki)
GET /loki/api/v1/label/<name>/values # List label values (Loki)
```

## Alternatives Considered

### Alternative 1: Keep Custom API Only

**Pros**:
- Less code to maintain
- Simpler implementation

**Cons**:
- ❌ Requires custom Grafana plugin
- ❌ Existing dashboards won't work
- ❌ Steeper learning curve for users

**Decision**: Rejected - Grafana integration is too valuable

### Alternative 2: Replace Custom API with Standard APIs

**Pros**:
- Only one set of endpoints to maintain
- Forces standards compliance

**Cons**:
- ❌ Breaks backward compatibility with OQL
- ❌ No clean way to support OQL queries
- ❌ Requires major version bump

**Decision**: Rejected - Keep both for flexibility

### Alternative 3: Auto-Detect Query Language

Automatically detect PromQL/LogQL/OQL from query syntax:

**Pros**:
- Single endpoint for all languages
- User doesn't specify language

**Cons**:
- ❌ Ambiguous queries (syntax overlap)
- ❌ Doesn't match Prometheus/Loki API contracts
- ❌ Still need standard endpoints for Grafana

**Decision**: Rejected - Standard endpoints are better for Grafana

## Recommendation

**Proceed with implementation** of Prometheus and Loki-compatible endpoints:

1. ✅ **High Value**: Enables seamless Grafana integration
2. ✅ **Backward Compatible**: Existing `/query` endpoint unchanged
3. ✅ **Code Reuse**: Leverages existing PromQL/LogQL translators
4. ✅ **Standard Compliance**: Matches industry standards
5. ✅ **Migration Path**: Easy migration from Prometheus/Loki

**Estimated Effort**: 1-2 weeks

**Risk**: Low - Core translation logic already exists, mainly adding HTTP handlers and response transformers

## Next Steps

**Awaiting approval** before implementation. Once approved:

1. Create feature branch: `git checkout -b feature/prometheus-loki-api-compatibility`
2. Implement Phase 1 (Core API handlers)
3. Implement Phase 2 (Time range support)
4. Implement Phase 3 (Response transformers)
5. Implement Phase 4 (Testing)
6. Implement Phase 5 (Documentation)
7. Manual Grafana integration testing
8. Create PR for review
