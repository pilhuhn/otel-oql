# OTEL-OQL Quickstart with Podman

Get up and running with OTEL-OQL and Apache Pinot in 3 commands.

## Prerequisites

- **Podman** installed and running
  - Linux: `sudo dnf install podman` or `sudo apt install podman`
  - macOS: `brew install podman && podman machine init && podman machine start`
  - Windows: Install Podman Desktop
- **Go 1.21+** installed
- **curl** for testing

## Three-Command Setup

```bash
# 1. One-command setup (builds, starts Pinot, creates schemas)
./scripts/setup-all.sh

# 2. Start OTEL-OQL service
./otel-oql --test-mode --pinot-url=http://localhost:9000

# 3. Verify everything works
./scripts/verify-setup.sh
```

That's it! 🎉

## What Just Happened?

The `setup-all.sh` script:
1. ✅ Built the `otel-oql` binary
2. ✅ Started Apache Pinot in Podman (ports 9000, 8099)
3. ✅ Created schemas for `otel_spans`, `otel_metrics`, `otel_logs`
4. ✅ Verified everything is working

The service is now:
- 📊 Accepting OTLP data on ports **4317** (gRPC) and **4318** (HTTP)
- 🔍 Ready for OQL queries on port **8080**
- 🗄️ Storing data in Pinot on port **9000**

## Quick Test

### Option 1: Insert Test Data

```bash
# Insert sample spans, metrics, and logs
./scripts/insert-test-data.sh

# Query via Pinot UI
open http://localhost:9000/#/query
```

Run in Pinot UI:
```sql
SELECT * FROM otel_spans WHERE tenant_id = 0;
```

### Option 2: Query via OQL

```bash
curl -X POST http://localhost:8080/query \
  -H "X-Tenant-ID: 0" \
  -H "Content-Type: application/json" \
  -d '{"query": "signal=spans | where tenant_id == 0 | limit 10"}'
```

### Option 3: Send Real OTLP Data

Point your OpenTelemetry SDK or Collector to:
- gRPC: `localhost:4317`
- HTTP: `localhost:4318`

In test mode, no tenant-id header is required (defaults to 0).

## Explore

**Pinot UI:**
```bash
open http://localhost:9000
```

**Check status:**
```bash
./scripts/verify-setup.sh
```

**View logs:**
```bash
podman logs pinot-quickstart
```

**Stop everything:**
```bash
# Stop service: Ctrl+C
podman stop pinot-quickstart
```

## Next Steps

- 📖 Read [GETTING_STARTED.md](GETTING_STARTED.md) for detailed setup
- 🔍 Try [examples/queries.md](examples/queries.md) for OQL query examples
- 🏗️ See [SCHEMA_CHANGES.md](SCHEMA_CHANGES.md) for architecture details
- 📝 Check [README.md](README.md) for full documentation

## Troubleshooting

### Podman not running

```bash
# macOS/Windows
podman machine start

# Check status
podman info
```

### Port already in use

```bash
# Check what's using the ports
lsof -i :9000   # Pinot
lsof -i :4317   # OTLP gRPC
lsof -i :4318   # OTLP HTTP
lsof -i :8080   # Query API
```

### Schema creation failed

```bash
# Check Pinot is healthy
curl http://localhost:9000/health

# View Pinot logs
podman logs pinot-quickstart

# Retry schema creation
./otel-oql setup-schema --pinot-url=http://localhost:9000
```

### Start fresh

```bash
# Remove everything and start over
podman stop pinot-quickstart
podman rm pinot-quickstart
./scripts/setup-all.sh
```

## Manual Setup (Step by Step)

If you prefer to run each step manually:

```bash
# 1. Build
go build -o otel-oql ./cmd/otel-oql

# 2. Start Pinot
./scripts/start-pinot.sh

# 3. Create schemas
./otel-oql setup-schema --pinot-url=http://localhost:9000

# 4. Verify
./scripts/verify-setup.sh

# 5. Start service
./otel-oql --test-mode --pinot-url=http://localhost:9000
```

## Configuration

### Production Mode (Requires Tenant-ID)

```bash
# Start without test mode
./otel-oql --pinot-url=http://localhost:9000

# Now queries must include X-Tenant-ID header
curl -X POST http://localhost:8080/query \
  -H "X-Tenant-ID: 1234" \
  -H "Content-Type: application/json" \
  -d '{"query": "signal=spans | limit 10"}'
```

### Custom Ports

```bash
./otel-oql \
  --pinot-url=http://localhost:9000 \
  --otlp-grpc-port=14317 \
  --otlp-http-port=14318 \
  --query-api-port=18080 \
  --test-mode
```

### Environment Variables

```bash
export PINOT_URL=http://localhost:9000
export TEST_MODE=true
export OTLP_GRPC_PORT=4317
export OTLP_HTTP_PORT=4318
export QUERY_API_PORT=8080

./otel-oql
```

## Architecture

```
┌─────────────────────────────────────────────┐
│   Your App (OpenTelemetry SDK)             │
└────────────┬────────────────────────────────┘
             │ OTLP (gRPC:4317 / HTTP:4318)
             ▼
┌─────────────────────────────────────────────┐
│         OTEL-OQL Service                    │
│  ┌──────────────┐    ┌─────────────────┐   │
│  │ OTLP Receiver│    │  Query API      │   │
│  │ (Multi-tenant)│   │  (OQL → SQL)   │   │
│  └──────┬───────┘    └────────┬────────┘   │
│         │                      │             │
│         ▼                      ▼             │
└─────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────┐
│        Apache Pinot (Podman)                │
│  • otel_spans  (traces)                     │
│  • otel_metrics (metrics + exemplars)       │
│  • otel_logs   (logs)                       │
└─────────────────────────────────────────────┘
```

## Ports Reference

| Port | Service | Purpose |
|------|---------|---------|
| 4317 | OTEL-OQL | OTLP gRPC receiver |
| 4318 | OTEL-OQL | OTLP HTTP receiver |
| 8080 | OTEL-OQL | OQL Query API |
| 9000 | Pinot | Broker (queries + UI) |
| 8099 | Pinot | Controller API |

## Resources

- 📚 [Full Documentation](README.md)
- 🚀 [Detailed Setup Guide](GETTING_STARTED.md)
- 💡 [OQL Query Examples](examples/queries.md)
- 🔧 [Schema Details](SCHEMA_CHANGES.md)
- 📝 [Script Documentation](scripts/README.md)
