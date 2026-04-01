# Perses Datasource Templates

This directory contains template files for Perses global datasources that connect to OTEL-OQL.

## Templates

- `otel-oql-metrics.json.template` - Prometheus datasource for metrics queries
- `otel-oql-logs.json.template` - Tempo datasource for trace queries  
- `oql-otel-trace.json.template` - Loki datasource for log queries

## Usage

Run the setup script to configure datasources:

```bash
./scripts/setup-perses.sh
```

The script will:
1. Prompt for the OTEL-OQL endpoint
2. Default to port 8080 if no port is specified
3. Replace `{{OTEL_OQL_ENDPOINT}}` placeholder with the actual endpoint
4. Create datasource files in `data/perses/globaldatasources/`

## Template Placeholder

All templates use the placeholder `{{OTEL_OQL_ENDPOINT}}` which gets replaced with the configured endpoint (e.g., `http://localhost:8080`).

## Datasource Mappings

| Datasource | Type | Query Language | Purpose |
|------------|------|----------------|---------|
| otel-oql-metrics | Prometheus | PromQL | Query metrics via OTEL-OQL's PromQL support |
| otel-oql-logs | Tempo | TraceQL | Query traces via OTEL-OQL (future TraceQL support) |
| oql-otel-trace | Loki | LogQL | Query logs via OTEL-OQL's LogQL support |

## Configuration

All datasources are configured with:
- `X-tenant-id: 0` header (for multi-tenant support)
- Appropriate API endpoints for each query language
- HTTP proxy configuration

## Integration with Setup

The `scripts/setup-all.sh` script can optionally call `setup-perses.sh` to configure datasources during initial setup.
