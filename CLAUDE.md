# CLAUDE.md

## Project Overview

OTEL-OQL is a multi-tenant OpenTelemetry data ingestion and query service written in Go. It bridges observability signals (metrics, logs, traces) with a powerful query language designed for cross-signal correlation and debugging workflows.

**Current State**: This is a greenfield project with only a specification (SPEC.md). No code has been written yet.

**Core Functionality**:
- Ingests OpenTelemetry data via OTLP (gRPC port 4317, HTTP port 4318)
- Stores telemetry data in Apache Pinot backend
- Provides OQL (Observability Query Language) for querying and correlating signals
- Enforces multi-tenant isolation with mandatory `tenant-id` property
- Translates OQL queries to Pinot SQL

## Architecture (Intended)

```
┌─────────────────────────────────────────────────────┐
│                   OTEL-OQL Service                  │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌──────────────────────────────────────────────┐  │
│  │         OTLP Receivers                       │  │
│  │  - gRPC (4317)  - HTTP (4318)               │  │
│  └────────────────┬─────────────────────────────┘  │
│                   │                                 │
│                   ▼                                 │
│  ┌──────────────────────────────────────────────┐  │
│  │    Multi-Tenant Request Handler              │  │
│  │  - Validate tenant-id (reject if missing)    │  │
│  │  - Test mode: default tenant-id=0            │  │
│  └────────────────┬─────────────────────────────┘  │
│                   │                                 │
│         ┌─────────┴─────────┐                      │
│         ▼                   ▼                       │
│  ┌─────────────┐     ┌─────────────────────────┐  │
│  │   Ingestion │     │   Query Engine          │  │
│  │   Pipeline  │     │  - OQL Parser           │  │
│  │             │     │  - Query Planner        │  │
│  │             │     │  - Pinot SQL Translator │  │
│  └──────┬──────┘     └──────────┬──────────────┘  │
│         │                       │                  │
└─────────┼───────────────────────┼──────────────────┘
          │                       │
          ▼                       ▼
    ┌─────────────────────────────────────┐
    │       Apache Pinot Backend          │
    │  - Metrics table (tenant partitioned)│
    │  - Logs table   (tenant partitioned)│
    │  - Traces/Spans (tenant partitioned)│
    └─────────────────────────────────────┘
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

OQL enables powerful observability workflows by allowing queries to start from one signal type and correlate or expand into others. The pipe operator (`|`) is optional but recommended for readability.

### Key Operators

#### `where`
Filter data based on conditions.
```
signal=spans | where name == "checkout_process" and duration > 500ms
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

**Note**: Code does not exist yet. When implementation begins:

### Prerequisites

- Go 1.21+ (or latest stable)
- Apache Pinot instance (running and accessible)
- **License Requirement**: Only use dependencies with Apache 2.0 license

### Build

```bash
go build -o otel-oql ./cmd/otel-oql
```

### Test

```bash
go test ./...
```

### Run Locally

```bash
# With test mode enabled (tenant-id=0 default)
./otel-oql --test-mode --pinot-url=http://localhost:9000

# Production mode (requires tenant-id)
./otel-oql --pinot-url=http://localhost:9000
```

### Environment Variables

- `PINOT_URL`: Apache Pinot broker URL
- `OTLP_GRPC_PORT`: gRPC receiver port (default: 4317)
- `OTLP_HTTP_PORT`: HTTP receiver port (default: 4318)
- `TEST_MODE`: Enable test mode with tenant-id=0 default (default: false)

### Schema Setup

The service should provide a command to initialize Pinot tables:

```bash
./otel-oql setup-schema --pinot-url=http://localhost:9000
```

## Project Structure (Proposed)

```
otel-oql/
├── cmd/
│   └── otel-oql/          # Main entry point
├── pkg/
│   ├── receiver/          # OTLP receivers (gRPC, HTTP)
│   ├── tenant/            # Multi-tenant validation & routing
│   ├── ingestion/         # Data transformation & storage
│   ├── oql/               # OQL parser & query planner
│   ├── translator/        # OQL to Pinot SQL translator
│   ├── pinot/             # Pinot client & schema management
│   └── api/               # Query API server
├── internal/
│   └── config/            # Configuration management
├── SPEC.md                # Project specification
├── CLAUDE.md              # This file
├── README.md              # Public documentation
└── go.mod
```

## Important Notes for Future Development

1. **Tenant Isolation is Critical**: Every query and ingestion path must enforce tenant-id scoping to prevent data leakage
2. **Exemplars are the Key**: The exemplar mechanism is what makes cross-signal correlation powerful - ensure metrics include exemplar trace_ids
3. **Pinot Schema Design**: Carefully design table schemas for efficient tenant-based partitioning and querying
4. **OQL Parser Complexity**: The language supports both fresh queries and result-set refinement - parser must handle both modes
5. **License Compliance**: All dependencies must use Apache 2.0 license
6. **Error Handling**: Reject invalid queries gracefully with helpful error messages
7. **Performance**: Large tenants may generate massive data volumes - consider query limits and pagination

## References

- SPEC.md - Full project specification
- OpenTelemetry Protocol (OTLP) specification
- Apache Pinot documentation
