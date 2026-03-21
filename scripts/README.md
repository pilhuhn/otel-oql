# OTEL-OQL Setup Scripts

Automation scripts to help you get started with OTEL-OQL and Apache Pinot.

## Quick Start

### Option 1: Automated Setup (Recommended)

Run everything in one command:

```bash
./scripts/setup-all.sh
```

This will:
1. Build the OTEL-OQL binary
2. Start Pinot in Podman
3. Create all schemas and tables
4. Verify the setup

### Option 2: Step by Step

```bash
# 1. Start Pinot
./scripts/start-pinot.sh

# 2. Build OTEL-OQL
go build -o otel-oql ./cmd/otel-oql

# 3. Create schemas
./otel-oql setup-schema --pinot-url=http://localhost:9000

# 4. Verify setup
./scripts/verify-setup.sh
```

## Scripts

### `start-pinot.sh`

Starts Apache Pinot in Podman all-in-one mode.

**What it does:**
- Pulls Pinot Podman image
- Starts container with proper port mappings (9000, 8099)
- Waits for Pinot to be healthy
- Handles existing containers gracefully

**Usage:**
```bash
./scripts/start-pinot.sh
```

**Ports:**
- `9000` - Pinot Broker (query endpoint)
- `8099` - Pinot Controller (admin API)

### `verify-setup.sh`

Verifies that everything is set up correctly.

**What it checks:**
- ✅ Podman is running
- ✅ Pinot container exists and is running
- ✅ Pinot is healthy
- ✅ OTEL-OQL binary is built
- ✅ Tables exist (otel_spans, otel_metrics, otel_logs)
- ✅ OTEL-OQL service ports (if running)

**Usage:**
```bash
./scripts/verify-setup.sh
```

**Exit codes:**
- `0` - All checks passed
- `>0` - Number of errors found

### `setup-all.sh`

Complete automated setup.

**What it does:**
1. Builds OTEL-OQL binary
2. Runs `start-pinot.sh`
3. Creates schemas in Pinot
4. Runs `verify-setup.sh`

**Usage:**
```bash
./scripts/setup-all.sh
```

### `insert-test-data.sh`

Inserts sample data for testing.

**What it creates:**
- 3 test spans (trace-001, trace-002)
- 2 test metrics (with exemplar for trace correlation)
- 1 test log entry

**Usage:**
```bash
# Requires Pinot to be running with schemas created
./scripts/insert-test-data.sh
```

**Test queries after insertion:**
```sql
-- Via Pinot SQL
SELECT * FROM otel_spans WHERE tenant_id = 0;
SELECT * FROM otel_metrics WHERE tenant_id = 0;
SELECT * FROM otel_logs WHERE tenant_id = 0;
```

```bash
# Via OQL (requires OTEL-OQL service running)
curl -X POST http://localhost:8080/query \
  -H "X-Tenant-ID: 0" \
  -H "Content-Type: application/json" \
  -d '{"query": "signal=spans | where tenant_id == 0 | limit 10"}'
```

## Common Tasks

### Start Everything

```bash
# Full setup
./scripts/setup-all.sh

# Start OTEL-OQL service
./otel-oql --test-mode --pinot-url=http://localhost:9000
```

### Check Status

```bash
./scripts/verify-setup.sh
```

### View Pinot UI

```bash
open http://localhost:9000
```

### Stop Pinot

```bash
podman stop pinot-quickstart
```

### Restart Pinot

```bash
podman start pinot-quickstart

# Wait for it to be ready
sleep 10
```

### Delete Everything and Start Fresh

```bash
# Stop and remove Pinot container
podman stop pinot-quickstart
podman rm pinot-quickstart

# Re-run setup
./scripts/setup-all.sh
```

## Troubleshooting

### Script Won't Run

Make sure scripts are executable:
```bash
chmod +x scripts/*.sh
```

### Pinot Won't Start

Check Podman:
```bash
podman info
podman ps -a
```

View Pinot logs:
```bash
podman logs pinot-quickstart
```

### Schema Creation Fails

**Check Pinot is ready:**
```bash
curl http://localhost:9000/health
```

**View existing tables:**
```bash
curl http://localhost:9000/tables
```

**Delete a table if needed:**
```bash
curl -X DELETE http://localhost:9000/tables/otel_spans
```

### Port Conflicts

Check what's using the ports:
```bash
lsof -i :9000   # Pinot Broker
lsof -i :8099   # Pinot Controller
lsof -i :4317   # OTLP gRPC
lsof -i :4318   # OTLP HTTP
lsof -i :8080   # Query API
```

## Environment Variables

All scripts respect these environment variables:

```bash
# Pinot URL (default: http://localhost:9000)
export PINOT_URL=http://localhost:9000

# Test mode (default: true for scripts)
export TEST_MODE=true
```

## Dependencies

- **Podman** - For running Pinot
- **curl** - For health checks and API calls
- **Go 1.21+** - For building OTEL-OQL
- **lsof** - For port checking (optional)

## See Also

- [GETTING_STARTED.md](../GETTING_STARTED.md) - Detailed setup guide
- [README.md](../README.md) - Project documentation
- [examples/queries.md](../examples/queries.md) - OQL query examples
