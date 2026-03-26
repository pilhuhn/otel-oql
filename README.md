# OTEL-OQL

A multi-tenant OpenTelemetry data ingestion and query service with OQL (Observability Query Language) support, backed by Apache Pinot with Kafka streaming.

## Features

- **OTLP Data Ingestion**: Receive metrics, logs, and traces via gRPC (port 4317) and HTTP (port 4318)
- **Multi-Tenant Isolation**: Enforce strict tenant separation with mandatory tenant-id
- **OQL Query Language**: Powerful query language for cross-signal correlation and debugging
- **Apache Pinot Storage**: Scalable REALTIME tables backed by Kafka streaming
- **Exemplar Support**: Jump from aggregated metrics to specific traces (the "wormhole")
- **MCP Server**: Model Context Protocol server for AI tool integration (port 8090)
- **CLI Query Tool**: Interactive command-line client for executing OQL queries
- **Self-Observability**: OpenTelemetry instrumentation for traces and metrics

## Quick Start

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

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--pinot-url` | `PINOT_URL` | `http://localhost:9000` | Pinot broker URL |
| `--otlp-grpc-port` | `OTLP_GRPC_PORT` | `4317` | OTLP gRPC receiver port |
| `--otlp-http-port` | `OTLP_HTTP_PORT` | `4318` | OTLP HTTP receiver port |
| `--query-api-port` | `QUERY_API_PORT` | `8080` | Query API server port |
| `--test-mode` | `TEST_MODE` | `false` | Enable test mode (tenant-id=0) |

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

## Multi-Tenancy

All requests must include a tenant-id:

- **gRPC**: Set `tenant-id` metadata
- **HTTP**: Set `X-Tenant-ID` header

In test mode, if no tenant-id is provided, it defaults to 0.

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
