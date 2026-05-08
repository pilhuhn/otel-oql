# OTEL-OQL

A multi-tenant OpenTelemetry data ingestion and query service with OQL (Observability Query Language) support, backed by Apache Pinot with Kafka streaming.

## Features

- **OTLP Data Ingestion**: Receive metrics, logs, and traces via gRPC (port 4317) and HTTP (port 4318)
- **Multi-Tenant Isolation**: Enforce strict tenant separation with mandatory tenant-id
- **Multi-Language Query Support**:
  - **OQL** - Native query language for cross-signal correlation and debugging
  - **PromQL** - Prometheus Query Language for metrics (100% parser reuse)
  - **LogQL** - Loki Query Language for logs with native column optimization (10-100x faster)
  - **TraceQL** - Planned (Phase 3) - Tempo Query Language for traces
- **Apache Pinot Storage**: Scalable REALTIME tables backed by Kafka streaming
- **Exemplar Support**: Jump from aggregated metrics to specific traces (the "wormhole")
- **MCP Server**: Model Context Protocol server for AI tool integration (port 8090)
- **CLI Query Tool**: Interactive command-line client for executing queries in any language
- **Self-Observability**: OpenTelemetry instrumentation for traces and metrics
- **Split Deployment**: Run ingestion and query components independently for scalability

## Quick Start

NOTE: The Apache Pinot setup in the compose file is not for production. 
Restarting the Pinot pod will delete the saved data.

### Prerequisites

- Go 1.21 or later
- Podman and podman-compose (or Docker/Docker Compose)
- All dependencies must use Apache 2.0 license

### Automated Setup (Recommended)

Use the setup scripts for quick environment setup:

```bash
# Start infrastructure (Kafka + Pinot)
podman-compose up -d

# Wait for services to be ready, then run automated setup
./scripts/setup-all.sh

# Verify the setup
./scripts/verify-setup.sh
```

This will:
- Create Kafka topics (otel-spans, otel-metrics, otel-logs)
- Initialize Pinot REALTIME tables
- Verify all services are running
- Insert test data

### Manual Setup

```bash
# Build the main service
go build -o otel-oql ./cmd/otel-oql

# Build the CLI query tool
go build -o oql-cli ./cmd/oql-cli

# Setup Pinot schema
./otel-oql setup-schema --pinot-url=http://localhost:9000
```

### Run the Service

```bash
# Production mode (requires tenant-id header)
./otel-oql --pinot-url=http://localhost:9000

# Test mode (defaults to tenant-id=0)
./otel-oql --test-mode --pinot-url=http://localhost:9000
```

### Configuration

Configuration via environment variables or command-line flags:

| Flag               | Environment Variable | Default                 | Description                                    |
|--------------------|----------------------|-------------------------|------------------------------------------------|
| `--mode`           | `OTEL_OQL_MODE`      | `all`                   | Operating mode: `all`, `ingestion`, or `query` |
| `--pinot-url`      | `PINOT_URL`          | `http://localhost:9000` | Pinot broker URL                               |
| `--kafka-brokers`  | `KAFKA_BROKERS`      | `localhost:9092`        | Kafka Broker URL                               | 
| `--otlp-grpc-port` | `OTLP_GRPC_PORT`     | `4317`                  | OTLP gRPC receiver port                        |
| `--otlp-http-port` | `OTLP_HTTP_PORT`     | `4318`                  | OTLP HTTP receiver port                        |
| `--query-api-port` | `QUERY_API_PORT`     | `8080`                  | Query API server port                          |
| `--test-mode`      | `TEST_MODE`          | `false`                 | Enable test mode (tenant-id=0)                 |

## Grafana Integration

OTEL-OQL provides Prometheus and Loki-compatible API endpoints, enabling seamless integration with Grafana without requiring custom plugins.

### Prometheus Datasource (Metrics)

Configure OTEL-OQL as a Prometheus datasource:

1. Add datasource in Grafana → Configuration → Data Sources
2. Select **Prometheus**
3. Set URL: `http://localhost:8080`
4. Add custom HTTP header: `X-Tenant-ID: 0`
5. Save & Test

Use standard PromQL queries in your dashboards:

```promql
rate(http_requests_total{job="api"}[5m])
sum by (service_name) (http_server_duration)
```

### Loki Datasource (Logs)

Configure OTEL-OQL as a Loki datasource:

1. Add datasource in Grafana → Configuration → Data Sources
2. Select **Loki**
3. Set URL: `http://localhost:8080`
4. Add custom HTTP header: `X-Tenant-ID: 0`
5. Save & Test

Use standard LogQL queries in your dashboards:

```logql
{job="varlogs", level="error"} |= "timeout"
sum by (level) (count_over_time({job="varlogs"}[5m]))
```

### API Endpoints

OTEL-OQL exposes 4 Grafana-compatible endpoints:

**Prometheus Endpoints** (for metrics):
- `GET|POST /api/v1/query` - Instant queries
- `GET|POST /api/v1/query_range` - Range queries

**Loki Endpoints** (for logs):
- `GET|POST /loki/api/v1/query` - Instant log queries
- `GET|POST /loki/api/v1/query_range` - Range log queries

For complete Grafana integration guide, dashboard import instructions, and troubleshooting, see [GRAFANA_INTEGRATION.md](./docs/api/GRAFANA_INTEGRATION.md).

## OTLP Data Ingestion

### Send Traces (gRPC)

```bash
# With tenant-id in metadata
grpcurl -d @ \
  -H "tenant-id: 1" \
  localhost:4317 \
  opentelemetry.proto.collector.trace.v1.TraceService/Export < trace.json
```

### Send Metrics (HTTP)

```bash
# With tenant-id in header
curl -X POST http://localhost:4318/v1/metrics \
  -H "X-Tenant-ID: 1" \
  -H "Content-Type: application/x-protobuf" \
  --data-binary @metrics.pb
```

## OQL Query Language

### Query via CLI Tool

The easiest way to query OTEL-OQL is using the `oql-cli` command-line tool:

```bash
# Build the CLI
go build -o oql-cli ./cmd/oql-cli

# Execute a query
oql-cli --tenant-id=0 "signal=spans limit 10"

# Interactive mode (multi-line input)
oql-cli --tenant-id=0

# Verbose output with SQL and stats
oql-cli --tenant-id=0 --verbose "signal=spans where duration > 100"

# Pipe query from stdin
echo "signal=spans since 1h" | oql-cli --tenant-id=0

# JSON output for scripting
oql-cli --tenant-id=0 --json "signal=spans limit 5" | jq .
```

See [cmd/oql-cli/README.md](cmd/oql-cli/README.md) for complete CLI documentation.

### Query via HTTP API

```bash
POST http://localhost:8080/query
Headers:
  X-Tenant-ID: 1
  Content-Type: application/json

Body:
{
  "query": "signal=spans | where name == \"checkout\" | limit 10"
}
```

Example with curl:

```bash
curl -X POST http://localhost:8080/query \
  -H 'X-Tenant-ID: 0' \
  -H 'Content-Type: application/json' \
  -d '{"query": "signal=spans limit 10"}'
```

### OQL Examples

#### Basic Trace Query

```oql
signal=spans
| where name == "checkout_process" and duration > 500ms
| limit 1
| expand trace
```

#### Error Investigation

```oql
signal=spans
| where name == "payment_gateway" and attributes.error == "true"
| correlate logs, metrics
```

#### Latency Spike Debugging (The Wormhole)

```oql
signal=metrics
| where metric_name == "http.server.duration" and value > 5000ms
| extract exemplar.trace_id as bad_trace
| switch_context signal=spans
| where trace_id == bad_trace
| expand trace
| correlate logs
```

#### Progressive Refinement

```oql
# First query
signal=traces | where attribute.duration > 5s

# Then refine (in a separate request)
filter attribute.error = true
```

### OQL Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `signal=<type>` | Start with a signal type | `signal=spans` |
| `where` | Filter data | `where name == "checkout"` |
| `expand trace` | Get all spans with same trace_id | `expand trace` |
| `correlate` | Find matching signals | `correlate logs, metrics` |
| `get_exemplars()` | Extract trace_ids from metrics | `get_exemplars()` |
| `switch_context` | Jump to another signal type | `switch_context signal=spans` |
| `extract` | Select specific fields | `extract trace_id as id` |
| `filter` | Refine existing results | `filter duration > 1s` |
| `limit` | Limit results | `limit 100` |

### Supported Signal Types

- `metrics` - Aggregated measurements (counters, gauges, histograms)
- `logs` - Discrete log events
- `spans` - Individual trace spans
- `traces` - Alias for spans

## PromQL Support

OTEL-OQL now supports PromQL (Prometheus Query Language) for querying metrics! Use your existing Prometheus queries with OTEL-OQL.

### Query via HTTP API

```bash
curl -X POST http://localhost:8080/query \
  -H 'X-Tenant-ID: 0' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "http_requests_total{job=\"api\", status=\"200\"}",
    "language": "promql"
  }'
```

### PromQL Examples

#### Instant Vector Selectors

```promql
# Query metric with labels
http_requests_total{job="api", status="200"}

# Regex label matching
http_requests_total{job=~"api.*"}

# Negative matching
http_requests_total{status!="500"}
```

#### Range Vector Selectors

```promql
# Last 5 minutes of data
http_requests_total{job="api"}[5m]

# Last 1 hour of data
cpu_usage{service="backend"}[1h]
```

#### Aggregations

```promql
# Sum all requests
sum(http_requests_total)

# Sum by label (GROUP BY)
sum by (job) (http_requests_total)

# Sum by multiple labels
sum by (job, status) (http_requests_total)

# Other aggregations
avg(cpu_usage)
min(response_time)
max(response_time)
count(http_requests_total)
```

#### Rate Functions

```promql
# Requests per second over 5 minutes
rate(http_requests_total[5m])

# Instantaneous rate
irate(cpu_usage[1m])
```

#### Value Comparisons

```promql
# Filter by value
cpu_usage > 80
memory_usage < 50
disk_usage >= 90

# Equal/not equal
status_code == 200
status_code != 500
```

### Supported PromQL Features

✅ **Supported**:
- Instant vector selectors with label matchers
- Range vector selectors with time ranges
- Label matcher operators: `=`, `!=`, `=~`, `!~`
- Aggregation functions: `sum()`, `avg()`, `min()`, `max()`, `count()`
- Grouping: `by (label1, label2)`
- Rate functions: `rate()`, `irate()`
- Comparison operators: `>`, `<`, `>=`, `<=`, `==`, `!=`

❌ **Not Yet Supported**:
- Binary operations between metrics (`metric1 / metric2`)
- Subqueries
- Advanced functions (`histogram_quantile`, `predict_linear`)
- Recording rules and alerts

### When to Use PromQL vs OQL

**Use PromQL when**:
- Querying metrics only
- Existing Grafana dashboards use PromQL
- Team is familiar with Prometheus

**Use OQL when**:
- Cross-signal queries (correlate metrics with traces/logs)
- Using `expand trace`, `get_exemplars()`, or `correlate`
- Need to jump from metrics to traces via exemplars
- Debugging complex issues across multiple signal types

## LogQL Support

OTEL-OQL now supports LogQL (Loki Query Language) for querying logs! Use your existing Loki queries with OTEL-OQL.

### Query via HTTP API

```bash
curl -X POST http://localhost:8080/query \
  -H 'X-Tenant-ID: 0' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "{job=\"varlogs\"} |= \"error\"",
    "language": "logql"
  }'
```

### LogQL Examples

#### Stream Selectors

```logql
# Query logs by label
{job="varlogs"}

# Multiple label matchers
{job="varlogs", level="error"}

# Regex label matching
{job=~"var.*"}

# Negative matching
{job="varlogs", level!="debug"}

# Negative regex
{job="varlogs", level!~"debug.*"}
```

#### Line Filters

```logql
# Contains text
{job="varlogs"} |= "error"

# Does not contain
{job="varlogs"} != "debug"

# Regex match
{job="varlogs"} |~ "error|fail"

# Regex not match
{job="varlogs"} !~ "debug|trace"

# Multiple filters
{job="varlogs"} |= "error" != "timeout"
```

#### Label Parsers

```logql
# Parse JSON logs
{job="varlogs"} | json

# Parse logfmt logs
{job="varlogs"} | logfmt

# Parse and filter
{job="varlogs"} | json |= "error"
```

#### Metric Queries

```logql
# Count log lines over time
count_over_time({job="varlogs"}[5m])

# Count with filters
count_over_time({job="varlogs"} |= "error"[5m])

# Rate of log lines per second
rate({job="varlogs"}[5m])

# Total bytes over time
bytes_over_time({job="varlogs"}[5m])

# Bytes per second
bytes_rate({job="varlogs"}[10m])
```

#### Aggregations

```logql
# Sum all counts
sum(count_over_time({job="varlogs"}[5m]))

# Sum by label (GROUP BY)
sum by (level) (count_over_time({job="varlogs"}[5m]))

# Sum by multiple labels
sum by (level, service) (count_over_time({job="varlogs"}[5m]))

# Other aggregations
avg(count_over_time({job="varlogs"}[5m]))
min by (service) (count_over_time({job="varlogs"}[5m]))
max by (level) (count_over_time({job="varlogs"}[5m]))
count(count_over_time({job="varlogs"}[5m]))
```

#### Complete Examples

```logql
# Find error logs in the last 5 minutes
{job="varlogs", level="error"}[5m]

# Count errors per service
sum by (service) (count_over_time({level="error"}[1h]))

# Find database errors (case-insensitive pattern)
{job="varlogs"} |~ "(?i)database.*error"

# Count rate of application logs excluding debug
rate({job="app"} != "debug"[5m])

# Total bytes of error logs by level
sum by (level) (bytes_over_time({job="varlogs"} |= "error"[1h]))
```

### Supported LogQL Features

✅ **Supported**:
- Stream selectors with label matchers
- Label matcher operators: `=`, `!=`, `=~`, `!~`
- Line filter operators: `|=`, `!=`, `|~`, `!~`
- Label parsers: `| json`, `| logfmt`, `| pattern`, `| regexp`
- Metric functions: `count_over_time()`, `rate()`, `bytes_over_time()`, `bytes_rate()`
- Aggregation functions: `sum()`, `avg()`, `min()`, `max()`, `count()`
- Grouping: `by (label1, label2)`, `without (label1)`
- Time ranges: `[5m]`, `[1h]`, `[24h]`
- Native column mapping (job, level, service, environment, etc.)

❌ **Not Yet Supported**:
- Label filters after parsing (`| label="value"`)
- Format expressions (`| line_format`, `| label_format`)
- Unwrap expressions for extracting numeric values
- Advanced metric functions (`quantile_over_time`, `stddev_over_time`)

### When to Use LogQL vs OQL

**Use LogQL when**:
- Querying logs only
- Existing Grafana Loki dashboards use LogQL
- Team is familiar with Loki
- Need log-specific features like line filters and parsers

**Use OQL when**:
- Cross-signal queries (correlate logs with traces/metrics)
- Using `expand trace` to see full request flow
- Need to correlate log entries with specific traces
- Debugging complex issues across multiple signal types

## Multi-Tenancy

All requests must include a tenant-id:

- **gRPC**: Set `tenant-id` metadata
- **HTTP**: Set `X-Tenant-ID` header

In test mode, if no tenant-id is provided, it defaults to 0.

## Split Deployment

OTEL-OQL supports three operating modes for flexible deployment:

### Operating Modes

1. **All-in-One** (default): Single process runs ingestion + query
2. **Ingestion**: Only OTLP receivers and Kafka ingestion
3. **Query**: Only query API and MCP server

### Usage

```bash
# Ingestion-only mode (scale for high OTLP load)
./otel-oql --mode=ingestion --kafka-brokers=kafka:9092

# Query-only mode (scale for high query load)
./otel-oql --mode=query --pinot-url=http://pinot:9000

# All-in-one mode (default, good for dev/test)
./otel-oql  # or --mode=all
```

### Benefits

- **Independent scaling**: Scale ingestion and query separately
- **Resource isolation**: CPU-bound ingestion vs memory-bound queries
- **Security boundaries**: Public ingestion vs private query API
- **Fault isolation**: Component failures don't cascade

See [docs/SPLIT_DEPLOYMENT.md](docs/SPLIT_DEPLOYMENT.md) for complete deployment guide with Kubernetes and Docker Compose examples.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   OTEL-OQL Service                  │
├─────────────────────────────────────────────────────┤
│  OTLP Receivers → Multi-Tenant Handler             │
│         ↓                        ↓                   │
│  Ingestion Pipeline      Query Engine (OQL)         │
│         ↓                        ↓                   │
│    Apache Pinot (metrics, logs, spans tables)       │
└─────────────────────────────────────────────────────┘
```

## Development

### Run Tests

```bash
go test ./...
```

### Format Code

```bash
go fmt ./...
```

### Dependencies

All dependencies use Apache 2.0 license as required by the project.

## License Compliance

This project requires all dependencies to be licensed under Apache 2.0. When adding new dependencies, verify their licenses:

```bash
go-licenses check ./cmd/otel-oql
```

## Documentation

For detailed architecture and development information, see [CLAUDE.md](CLAUDE.md).

For the complete specification, see [SPEC.md](SPEC.md).
