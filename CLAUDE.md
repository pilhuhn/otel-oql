# CLAUDE.md

## Project Overview

OTEL-OQL is a multi-tenant OpenTelemetry data ingestion and query service written in Go. It bridges observability signals (metrics, logs, traces) with multiple query languages designed for cross-signal correlation and debugging workflows.

**Current State**: ✅ **Fully Operational** - Complete implementation with all three signal types, multi-language query support, comprehensive testing, and production-ready features.

**Core Functionality**:
- Ingests OpenTelemetry data via OTLP (gRPC port 4317, HTTP port 4318)
- Stores telemetry data in Apache Pinot backend with Kafka streaming
- Supports **4 query languages**: OQL, PromQL, LogQL, TraceQL (Phase 3 - planned)
- Enforces multi-tenant isolation with mandatory `tenant-id` property
- Translates all query languages to Pinot SQL
- Full OpenTelemetry self-instrumentation with traces and metrics
- MCP (Model Context Protocol) server for AI tool integration

## Architecture (Implemented)

```
┌─────────────────────────────────────────────────────────────────┐
│                       OTEL-OQL Service                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │         OTLP Receivers (OpenTelemetry Instrumented)      │  │
│  │  - gRPC (4317)  - HTTP (4318)                           │  │
│  └────────────────────┬─────────────────────────────────────┘  │
│                       │                                         │
│                       ▼                                         │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │    Multi-Tenant Middleware                               │  │
│  │  - gRPC/HTTP tenant-id validation                        │  │
│  │  - Test mode: default tenant-id=0                        │  │
│  └────────────────────┬─────────────────────────────────────┘  │
│                       │                                         │
│         ┌─────────────┴───────────────┐                        │
│         ▼                             ▼                         │
│  ┌─────────────┐             ┌────────────────────────────┐   │
│  │  Ingestion  │             │  Multi-Language Query API  │   │
│  │  Pipeline   │             │  - OQL Parser/Translator   │   │
│  │  - OTLP→Map │             │  - PromQL (Prometheus)     │   │
│  │  - Extract  │             │  - LogQL (Loki)            │   │
│  │  - Kafka    │             │  - TraceQL (Phase 3)       │   │
│  └──────┬──────┘             └───────────┬────────────────┘   │
│         │                                │                     │
│         ▼                                │                     │
│  ┌─────────────┐                        │                     │
│  │   Kafka     │                        │                     │
│  │  - 3 Topics │                        │                     │
│  └──────┬──────┘                        │                     │
│         │                                │                     │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  MCP Server (port 8090)                                 │  │
│  │  - oql_query tool - oql_help tool                       │  │
│  └─────────────────────────────────────┬───────────────────┘  │
│                                         │                      │
└─────────────────────────────────────────┼──────────────────────┘
                                          │
                  ┌───────────────────────┴────────────────────┐
                  ▼                                            ▼
    ┌──────────────────────────┐              ┌─────────────────────┐
    │  Apache Pinot REALTIME   │              │  OpenTelemetry      │
    │  - otel_spans           │              │  Backend            │
    │  - otel_metrics         │              │  (Self-observability)│
    │  - otel_logs            │              └─────────────────────┘
    │  (tenant partitioned)   │
    │  (Kafka streaming)      │
    └──────────────────────────┘
```

### Component Breakdown

**OTLP Receivers**:
- Accept all three signal types: metrics, logs, traces
- gRPC on port 4317, HTTP on port 4318
- Extract and validate `tenant-id` from incoming requests

**Multi-Tenant Request Handler**:
- Enforces tenant isolation by validating `tenant-id` property
- Rejects requests without tenant-id (unless in test mode)
- Test mode: sets default `tenant-id=0` for local development

**Ingestion Pipeline**:
- Transforms OTLP data to Pinot-compatible format
- Partitions data by tenant-id
- Manages schema setup for Pinot tables

**Query Engine**:
- Parses OQL queries
- Plans execution across signal types
- Translates to Pinot SQL
- Handles cross-signal correlation and context switching

## OQL Query Language

OQL enables powerful observability workflows by allowing queries to start from one signal type and correlate or expand into others. **The pipe operator (`|`) is completely optional** - use it for readability or omit it entirely.

### Key Operators

#### `where`
Filter data based on conditions.
```
# With pipes (readable)
signal=spans | where name == "checkout_process" and duration > 500ms

# Without pipes (also valid)
signal=spans where name == "checkout_process" and duration > 500ms
```

#### `expand trace`
Magic operator that fetches all spans sharing the same `trace_id`.
```
signal=spans
| where name == "checkout_process" and duration > 500ms
| limit 1
| expand trace  // Reconstructs full trace waterfall
```

#### `correlate`
Find matching logs and/or metrics for the current signal.
```
signal=spans
| where name == "payment_gateway" and attributes.error == "true"
| correlate logs, metrics
```

#### `get_exemplars()`
Extracts exemplars (trace_ids attached to aggregated metrics) - the "wormhole" from aggregation space to event space.
```
signal=metrics
| where name == "http.server.duration" and value > 2s
| get_exemplars()  // Returns trace_ids of slow requests
| expand trace
| correlate logs
```

#### `switch_context`
Explicitly jump from one signal type to another, using extracted identifiers.
```
signal=metrics
| where metric_name == "http.server.duration" and value > 5000ms
| extract exemplar.trace_id as bad_trace
| switch_context signal=spans
| where trace_id == bad_trace
| expand trace
```

#### `filter`
Refine an existing result set without starting a new query.
```
// First query
signal=traces | where attribute.duration > 5s

// Refine results
filter attribute.error = true
```

### Query Patterns

**Pattern 1: Trace Expansion**
```
signal=spans
| where name == "checkout_process" and duration > 500ms
| limit 1
| expand trace
```

**Pattern 2: Error Investigation**
```
signal=spans
| where name == "payment_gateway" and attributes.error == "true"
| correlate logs, metrics
```

**Pattern 3: Latency Spike Debugging (The Wormhole)**
```
// 1. Find the latency spike in aggregated metrics
signal=metrics
| where metric_name == "http.server.duration" and value > 5000ms

// 2. Extract the exemplar (the wormhole key)
| extract exemplar.trace_id as bad_trace

// 3. Jump from Aggregation Space to Event Space
| switch_context signal=spans
| where trace_id == bad_trace
| expand trace  // Rebuild the full waterfall

// 4. Pull correlated logs
| correlate logs
| where attributes.error == "true"
```

**Pattern 4: Progressive Refinement**
```
// Initial broad query
signal=traces | where attribute.duration > 5s

// Then refine
filter attribute.error = true

// Or expand context
find baseline for bad_trace.service
```

## PromQL Support (Prometheus Query Language)

**Status**: ✅ Fully Implemented (Phase 1 - March 2026)

OTEL-OQL supports PromQL as an alternative query language for metrics, enabling seamless integration with existing Prometheus tooling and Grafana dashboards.

### Implementation Approach

- **Parser**: Reuses official `github.com/prometheus/prometheus/promql/parser` (Apache 2.0)
- **Code Reuse**: 100% parser reuse - parse PromQL AST, translate to Pinot SQL
- **Testing**: 171 comprehensive tests covering all supported features
- **Translation**: Direct AST-to-SQL translation with tenant isolation

### Supported Features

- ✅ Instant and range vector selectors: `http_requests_total`, `http_requests_total[5m]`
- ✅ Label matchers: `=`, `!=`, `=~`, `!~`
- ✅ Aggregations: `sum`, `avg`, `min`, `max`, `count` with `by (label)` grouping
- ✅ Rate functions: `rate()`, `irate()`
- ✅ Value comparisons: `>`, `<`, `>=`, `<=`, `==`, `!=`
- ✅ Multi-tenant isolation (automatic tenant_id injection)

### Not Supported

- ❌ Binary operations between metrics (`metric1 + metric2`)
- ❌ Subqueries
- ❌ Advanced functions (`histogram_quantile`, `predict_linear`, etc.)
- ❌ Offset modifier
- ❌ @ modifier

### Example Usage

```bash
# Via Query API
curl -X POST http://localhost:8080/query \
  -H 'X-Tenant-ID: 0' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "sum by (service_name) (rate(http_requests_total[5m]))",
    "language": "promql"
  }'

# Via CLI
./oql-cli --tenant-id=0 --language=promql \
  "sum by (service_name) (rate(http_requests_total[5m]))"
```

### Translation Example

```promql
sum by (service_name) (
  rate(http_requests_total{job="api", status_code="200"}[5m])
)
```

Translates to:

```sql
SELECT service_name, SUM(value) / 300000
FROM otel_metrics
WHERE tenant_id = 0
  AND metric_name = 'http_requests_total'
  AND job = 'api'
  AND JSON_EXTRACT_SCALAR(attributes, '$.status_code', 'STRING') = '200'
  AND timestamp >= (now() - 300000)
GROUP BY service_name
```

## LogQL Support (Loki Query Language)

**Status**: ✅ Fully Implemented (Phase 2 - March 2026)

OTEL-OQL supports LogQL for querying logs, enabling Grafana Loki-compatible log queries with native trace correlation.

### Implementation Approach

- **Hybrid Parser**: Reuses Prometheus parser for stream selectors (60-70% code reuse)
- **Custom Extensions**: Custom parser for LogQL-specific operators (`|=`, `|~`, `| json`, etc.)
- **Testing**: 201 tests total (171 LogQL + 30 API integration)
- **Performance**: Native indexed columns for common labels (10-100x faster)

### Supported Features

**Log Range Queries**:
- ✅ Stream selectors: `{job="varlogs", level="error"}`
- ✅ Line filters: `|= "error"`, `!= "debug"`, `|~ "pattern"`, `!~ "exclude"`
- ✅ Label parsers: `| json`, `| logfmt`, `| pattern`, `| regexp`
- ✅ Time ranges: `[5m]`, `[1h]`

**Metric Queries**:
- ✅ `count_over_time({job="varlogs"}[5m])`
- ✅ `rate({job="varlogs"}[5m])`
- ✅ `bytes_over_time({job="varlogs"}[5m])`
- ✅ `bytes_rate({job="varlogs"}[5m])`

**Aggregations**:
- ✅ `sum`, `avg`, `min`, `max`, `count` with `by (label)` grouping
- ✅ `sum by (level) (count_over_time({job="varlogs"}[5m]))`

### Native Column Optimization

For maximum performance, common log labels are stored as native indexed columns instead of JSON:

```go
// Native Columns (10-100x faster!)
- job, instance, environment  // Prometheus/Loki common labels
- trace_id, span_id           // Trace correlation
- severity_text, log_level    // Severity filtering
- service_name, host_name     // Service/host filtering
- log_source                  // Source file/stream
```

**Performance Impact**:

```logql
# BEFORE (JSON extraction): ~100ms
{job="varlogs"}  →  WHERE JSON_EXTRACT_SCALAR(attributes, '$.job') = 'varlogs'

# AFTER (native column): ~10ms
{job="varlogs"}  →  WHERE job = 'varlogs'  -- 10x faster!
```

### Log-to-Trace Correlation

Native `trace_id` and `span_id` columns enable instant log-to-trace correlation:

```logql
# Find all logs for a specific trace
{trace_id="abc123"}

# Find logs for error spans
{trace_id="abc123", level="error"}
```

### Example Usage

```bash
# Via Query API
curl -X POST http://localhost:8080/query \
  -H 'X-Tenant-ID: 0' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "{job=\"varlogs\", level=\"error\"} |= \"timeout\"",
    "language": "logql"
  }'

# Metric query
curl -X POST http://localhost:8080/query \
  -H 'X-Tenant-ID: 0' \
  -d '{
    "query": "sum by (level) (count_over_time({job=\"varlogs\"}[5m]))",
    "language": "logql"
  }'
```

### Translation Example

```logql
sum by (level) (
  count_over_time({job="varlogs", level="error"} |= "timeout" [5m])
)
```

Translates to:

```sql
SELECT log_level, SUM(cnt) FROM (
  SELECT log_level, COUNT(*) as cnt
  FROM otel_logs
  WHERE tenant_id = 0
    AND job = 'varlogs'
    AND log_level = 'error'
    AND body LIKE '%timeout%'
    AND timestamp >= (now() - 300000)
  GROUP BY log_level
)
GROUP BY log_level
```

## Key Concepts

### Multi-Tenancy

All data and queries are isolated by `tenant-id`:
- Incoming OTLP data MUST include a `tenant-id` property
- Requests without `tenant-id` are rejected (unless in test mode)
- Pinot tables are partitioned by `tenant-id`
- Queries automatically scope to the authenticated tenant

### Test Mode

For local development:
- Sets default `tenant-id=0` when no tenant-id is provided
- Allows ingestion without explicit tenant headers
- Should NOT be enabled in production

### Signal Types

Three OpenTelemetry signal types are supported:
- **Metrics**: Aggregated measurements (counters, gauges, histograms)
- **Logs**: Discrete log events
- **Traces/Spans**: Distributed trace data

### Aggregation Space vs Event Space

A critical concept for understanding OQL:

- **Aggregation Space**: Metrics summarize behavior (e.g., "average latency was 2s")
- **Event Space**: Individual occurrences (specific traces, logs)

**The Wormhole**: Exemplars attached to metrics provide `trace_id` pointers that let you jump from aggregated metrics back to the specific traces that contributed to them. This is how you debug "which exact request caused this spike?"

### Apache Pinot Backend

- Assumed to be running and accessible
- No pre-existing schema required - this service sets up tables
- Tables for metrics, logs, and spans/traces
- Each table partitioned by `tenant-id`
- Service translates OQL to Pinot SQL dialect

## Development Setup

### Prerequisites

- Go 1.21+ (or latest stable)
- Apache Pinot instance (running and accessible)
- Apache Kafka (for streaming ingestion)
- **License Requirement**: Only use dependencies with Apache 2.0 license
- Use `podman` and `podman-compose` (not docker)

### Build

```bash
go build -o otel-oql ./cmd/otel-oql
go build -o oql-cli ./cmd/oql-cli
```

### Test

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package tests
go test ./pkg/promql -v
go test ./pkg/logql -v
go test ./pkg/integration -v

# IMPORTANT: Write unit tests, not /tmp scripts!
```

### Run Locally

```bash
# Start infrastructure
podman-compose up -d

# Setup Pinot schemas
./otel-oql setup-schema --pinot-url=http://localhost:9000

# Run service with test mode
./otel-oql --test-mode

# Run with self-observability
./otel-oql --observability-enabled

# Query via CLI
./oql-cli "signal=spans | limit 10"
./oql-cli --language=promql "http_requests_total"
./oql-cli --language=logql '{job="varlogs"}'
```

### Environment Variables

- `PINOT_URL`: Apache Pinot broker URL (default: http://localhost:8000)
- `KAFKA_BROKERS`: Kafka broker addresses (default: localhost:9092)
- `OTLP_GRPC_PORT`: gRPC receiver port (default: 4317)
- `OTLP_HTTP_PORT`: HTTP receiver port (default: 4318)
- `QUERY_API_PORT`: Query API port (default: 8080)
- `MCP_PORT`: MCP server port (default: 8090)
- `TEST_MODE`: Enable test mode with tenant-id=0 default (default: false)
- `OBSERVABILITY_ENABLED`: Enable self-observability (default: false)
- `OBSERVABILITY_ENDPOINT`: OTLP endpoint (default: localhost:4317)
- `OBSERVABILITY_TENANT_ID`: Tenant ID for self-observability (default: "0")

### Schema Setup

```bash
# Initialize all Pinot tables
./otel-oql setup-schema --pinot-url=http://localhost:9000

# Verify setup
curl http://localhost:9000/tables
```

## Project Structure (Implemented)

```
otel-oql/
├── cmd/
│   ├── otel-oql/              # Main service entry point
│   │   ├── main.go
│   │   └── setup_schema.go
│   ├── oql-cli/               # CLI query tool
│   │   ├── main.go
│   │   └── README.md
│   └── send-test-data/        # Test data generator
│       └── main.go
├── pkg/
│   ├── api/                   # Multi-language query API
│   │   ├── server.go          # Routing for OQL/PromQL/LogQL/TraceQL
│   │   ├── query_routing_test.go
│   │   ├── logql_integration_test.go
│   │   └── logql_trace_correlation_test.go
│   ├── receiver/              # OTLP receivers
│   │   ├── grpc.go            # gRPC receiver (port 4317)
│   │   └── http.go            # HTTP receiver (port 4318)
│   ├── tenant/                # Multi-tenant middleware
│   │   ├── grpc.go
│   │   ├── http.go
│   │   └── tenant.go
│   ├── ingestion/             # Data transformation pipeline
│   │   ├── ingester.go        # Kafka producer
│   │   └── attributes.go
│   ├── oql/                   # OQL parser & translator
│   │   ├── ast.go
│   │   ├── parser.go
│   │   └── parser_test.go
│   ├── promql/                # PromQL support (Phase 1)
│   │   ├── translator.go      # PromQL AST → Pinot SQL
│   │   ├── translator_test.go # 171 tests
│   │   ├── integration_test.go
│   │   └── parser_behavior_test.go
│   ├── logql/                 # LogQL support (Phase 2)
│   │   ├── parser.go          # Hybrid parser
│   │   ├── translator.go      # LogQL → Pinot SQL
│   │   ├── stream.go          # Stream selector parsing
│   │   ├── pipeline.go        # Pipeline operator parsing
│   │   ├── translator_test.go # 171 tests
│   │   ├── stream_test.go
│   │   ├── pipeline_test.go
│   │   └── parser_test.go
│   ├── querylangs/            # Shared query language components
│   │   ├── common/
│   │   │   ├── matcher.go     # Label matcher translation
│   │   │   ├── timerange.go   # Time range handling
│   │   │   └── aggregation.go # Aggregation functions
│   │   ├── analysis_test.go
│   │   └── reuse_opportunities_test.go
│   ├── translator/            # OQL to SQL translator
│   │   ├── translator.go
│   │   └── translator_test.go
│   ├── pinot/                 # Pinot client & schema
│   │   ├── client.go
│   │   └── schema.go          # REALTIME table schemas
│   ├── observability/         # Self-instrumentation
│   │   └── observability.go   # OpenTelemetry setup
│   ├── mcp/                   # Model Context Protocol
│   │   ├── server.go          # MCP HTTP server
│   │   └── server_test.go
│   └── integration/           # Integration tests
│       ├── integration_test.go
│       ├── e2e_test.go
│       └── new_operations_test.go
├── internal/
│   └── config/                # Configuration management
│       └── config.go
├── scripts/                   # Setup automation
│   ├── setup-all.sh
│   ├── verify-setup.sh
│   └── insert-test-data.sh
├── examples/
│   └── promql-examples.sh     # PromQL query examples
├── SPEC.md                    # Original specification
├── CLAUDE.md                  # This file
├── CHECKPOINT.md              # Implementation progress
├── README.md                  # User documentation
├── CONFIG.md                  # Configuration guide
├── TESTING.md                 # Testing strategy
├── OQL_REFERENCE.md           # OQL language reference
├── LOGQL_SUPPORT.md           # LogQL documentation
├── MIGRATION_GUIDE.md         # Schema migration guide
├── SCHEMA.md                  # Pinot schema documentation
├── PROMQL_TESTING.md          # PromQL testing documentation
├── QUERY_LANGUAGE_ANALYSIS.md # Language comparison analysis
├── otel-oql.yaml              # Example configuration
├── compose.yml                # Podman compose setup
└── go.mod
```

## Important Notes for Future Development

### Security & Isolation

1. **Tenant Isolation is Critical**: Every query and ingestion path must enforce tenant-id scoping to prevent data leakage
   - ✅ All SQL queries include `WHERE tenant_id = ?`
   - ✅ tenant-id validated as integer (not user-controlled string)
   - ✅ Integration tests verify isolation across all languages

2. **Input Validation**: Parser-based validation is your first line of defense
   - ✅ PromQL/LogQL parsers validate label names before SQL generation
   - ✅ Only `[a-zA-Z0-9_]` allowed in label names (prevents SQL injection)
   - ✅ Security review confirmed: No SQL injection vulnerabilities
   - ⚠️ Always escape label values with `sqlutil.StringLiteral()`

### Query Language Implementation

3. **Code Reuse Strategy**: Reuse battle-tested parsers when possible
   - ✅ PromQL: 100% parser reuse from Prometheus (Apache 2.0)
   - ✅ LogQL: 60-70% reuse via hybrid approach
   - ✅ Shared components in `pkg/querylangs/common/`
   - 📝 Analysis in QUERY_LANGUAGE_ANALYSIS.md shows reuse opportunities

4. **Parser Selection**: Don't reinvent the wheel
   - ✅ Use official parsers when available (Prometheus parser)
   - ✅ Hybrid approach for partial compatibility (LogQL)
   - ❌ Avoid AGPL-licensed parsers (Grafana Loki, Tempo)
   - ⚠️ Custom parsers only as last resort (TraceQL)

5. **Translation Patterns**: AST → SQL translation is straightforward
   - ✅ Parse user query to AST using upstream parser
   - ✅ Walk AST and generate SQL fragments
   - ✅ Always inject `tenant_id` in WHERE clause
   - ✅ Use native columns for performance, JSON for flexibility

### Performance Optimization

6. **Native Columns vs JSON**: 10-100x performance difference
   - ✅ Map common labels to native indexed columns
   - ✅ Use JSON for uncommon/custom attributes
   - ✅ LogQL schema: job, instance, environment, trace_id, span_id all native
   - 📝 See SCHEMA.md for column selection rationale

7. **Trace Correlation**: Native columns enable instant correlation
   - ✅ trace_id and span_id as indexed columns
   - ✅ `{trace_id="abc123"}` uses index (not JSON extraction)
   - ✅ Critical for OQL `correlate` operation performance

### Testing Best Practices

8. **Comprehensive Testing**: Learned from 371+ tests across PromQL/LogQL
   - ✅ **Unit tests** for translator (171 tests each for PromQL/LogQL)
   - ✅ **Integration tests** at API level (30+ tests)
   - ✅ **Parser behavior tests** to document upstream behavior
   - ✅ **Trace correlation tests** to verify native columns
   - ❌ **NEVER** write tests in `/tmp` - use proper test files!

9. **Test Organization**:
   ```
   pkg/promql/
   ├── translator_test.go       # SQL generation tests (171)
   ├── integration_test.go      # Complex queries
   └── parser_behavior_test.go  # Document Prometheus parser

   pkg/logql/
   ├── translator_test.go       # SQL generation tests (171)
   ├── parser_test.go           # Hybrid parser tests
   ├── stream_test.go           # Stream selector tests
   └── pipeline_test.go         # Pipeline operator tests

   pkg/api/
   ├── query_routing_test.go    # Multi-language routing
   ├── logql_integration_test.go        # API-level tests
   └── logql_trace_correlation_test.go  # Native column verification
   ```

### Architecture Principles

10. **Exemplars are the Key**: The "wormhole" from metrics to traces
    - ✅ Gauge and sum metrics include exemplar_trace_id
    - ✅ Enables debugging "which request caused this spike?"
    - ✅ Critical for OQL `get_exemplars()` operation

11. **Pinot Schema Design**: Hybrid approach balances performance and flexibility
    - ✅ Native columns for common OTel semantic conventions
    - ✅ JSON columns for custom/uncommon attributes
    - ✅ REALTIME tables with Kafka streaming
    - ✅ Tenant-based partitioning

12. **Error Handling**: Clear, actionable error messages
    - ✅ Parse errors include position and context
    - ✅ Translation errors explain what's unsupported
    - ✅ Early validation prevents cryptic Pinot errors
    - Example: "offset modifier not supported (offset 5m0s)"

### Compliance & Standards

13. **License Compliance**: All dependencies must use Apache 2.0
    - ✅ Prometheus parser: Apache 2.0
    - ✅ OpenTelemetry SDK: Apache 2.0
    - ✅ Kafka client (Sarama): Apache 2.0
    - ❌ Avoid GPL/AGPL: Loki, Tempo parsers

14. **Use Podman, Not Docker**: Project standard
    - ✅ All scripts use `podman` and `podman-compose`
    - ✅ Documentation references podman
    - ❌ Don't write "docker" in docs/examples

### Development Workflow

15. **Write Tests First**: TDD approach works well
    - ✅ Write translator tests before implementing translation
    - ✅ Document expected SQL output
    - ✅ Run tests frequently during development
    - ✅ 100% test pass rate before committing

16. **Documentation Hygiene**: Keep docs in sync with code
    - ✅ Update CLAUDE.md when architecture changes
    - ✅ Update CHECKPOINT.md after major features
    - ✅ Create focused docs (LOGQL_SUPPORT.md, MIGRATION_GUIDE.md)
    - ✅ Include examples in all documentation

## Multi-Language Query Strategy

The project supports 4 query languages, each serving different use cases:

| Language | Status | Use Case | Parser Strategy | Code Reuse |
|----------|--------|----------|----------------|------------|
| **OQL** | ✅ Complete | Cross-signal correlation, debugging workflows | Custom | N/A |
| **PromQL** | ✅ Complete | Metrics queries, Grafana dashboards | Prometheus parser | 100% |
| **LogQL** | ✅ Complete | Log queries, Loki compatibility | Hybrid (Prometheus + custom) | 60-70% |
| **TraceQL** | 🚧 Planned | Trace queries, Tempo compatibility | Custom (AGPL avoidance) | 30-40% |

### Implementation Phases

**Phase 1: PromQL** (March 2026)
- Parser: github.com/prometheus/prometheus/promql/parser
- Translation: AST → Pinot SQL for otel_metrics table
- Tests: 171 unit + 5 integration = 176 tests
- Result: 100% parser reuse, straightforward translation

**Phase 2: LogQL** (March 2026)
- Parser: Hybrid - Prometheus for stream selectors, custom for pipelines
- Translation: AST → Pinot SQL for otel_logs table
- Schema: Added native columns (job, instance, environment, trace_id, span_id)
- Tests: 171 unit + 30 integration = 201 tests
- Result: 60-70% code reuse, 10-100x performance via native columns

**Phase 3: TraceQL** (Planned - Not Started)
- Parser: Custom implementation required (Tempo parser is AGPL)
- Translation: AST → Pinot SQL for otel_spans table
- Estimated: 30-40% concept reuse from PromQL/LogQL
- Challenge: Span selection syntax differs significantly
- Estimated Effort: 2-3 weeks (330+ tests planned)
- **See [TRACEQL_PHASE3.md](./TRACEQL_PHASE3.md) for detailed implementation plan**
- **Note**: OQL already provides comprehensive trace querying; TraceQL adds Tempo compatibility

### Query Routing

The API server (`pkg/api/server.go`) routes queries based on `language` parameter:

```go
func (s *Server) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
    switch req.Language {
    case "promql":
        return s.executePromQLQuery(ctx, req.Query, req.TenantID)
    case "logql":
        return s.executeLogQLQuery(ctx, req.Query, req.TenantID)
    case "traceql":
        return s.executeTraceQLQuery(ctx, req.Query, req.TenantID)
    default: // "oql" or empty
        return s.executeOQLQuery(ctx, req.Query, req.TenantID)
    }
}
```

All languages translate to Pinot SQL with automatic tenant_id injection.

## Testing Strategy Summary

### Test Pyramid (Total: 400+ tests)

**Unit Tests**: 350+ tests
- OQL parser: 30 tests
- OQL translator: 25 tests
- PromQL translator: 171 tests
- LogQL translator: 171 tests
- Query language analysis: 44 tests

**Integration Tests**: 60+ tests
- E2E data flow: 8 tests
- OQL operations: 15 tests
- MCP server: 9 tests
- API routing: 14 tests
- LogQL integration: 30 tests

**Key Testing Principles**:
1. Write unit tests in the codebase, never in /tmp
2. Test SQL output, not just parsing
3. Verify tenant isolation in all tests
4. Document expected behavior with test names
5. Include edge cases and error scenarios

## References

- **SPEC.md** - Original project specification
- **CHECKPOINT.md** - Implementation progress and status
- **README.md** - User-facing documentation
- **CONFIG.md** - Configuration guide
- **TESTING.md** - Testing strategy and examples
- **OQL_REFERENCE.md** - Complete OQL language reference
- **LOGQL_SUPPORT.md** - LogQL documentation with examples
- **PROMQL_TESTING.md** - PromQL testing documentation
- **QUERY_LANGUAGE_ANALYSIS.md** - Parser reuse analysis
- **MIGRATION_GUIDE.md** - Schema migration guide
- **SCHEMA.md** - Pinot schema documentation
- **PINOT_LIMITATIONS.md** - Pinot constraints and workarounds

**External References**:
- [OpenTelemetry Protocol (OTLP)](https://opentelemetry.io/docs/specs/otlp/)
- [Apache Pinot Documentation](https://docs.pinot.apache.org/)
- [Prometheus Query Language](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [LogQL Documentation](https://grafana.com/docs/loki/latest/logql/)
- [TraceQL Documentation](https://grafana.com/docs/tempo/latest/traceql/)
- [OTel Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
