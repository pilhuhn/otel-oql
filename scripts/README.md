# OTEL-OQL Setup Scripts

Automation scripts to help you get started with OTEL-OQL, Apache Kafka, and Apache Pinot.

## Quick Start

### Option 1: Automated Setup (Recommended)

Run everything in one command:

```bash
./scripts/setup-all.sh
```

This will:
1. Build the OTEL-OQL and CLI binaries
2. Start Kafka and Pinot using `podman compose`
3. Create all schemas and tables
4. Verify the setup

### Option 2: Step by Step

```bash
# 1. Start infrastructure (Kafka + Pinot)
podman compose up -d

# 2. Build OTEL-OQL and CLI
go build -o otel-oql ./cmd/otel-oql
go build -o oql-cli ./cmd/oql-cli

# 3. Create schemas
./otel-oql setup-schema --pinot-url=http://localhost:9000

# 4. Verify setup
./scripts/verify-setup.sh
```

## Infrastructure

OTEL-OQL uses `podman compose` to manage all infrastructure services.

**Services:**
- **Kafka** - Message broker for streaming OTLP data
- **Pinot** - Analytics database for storing and querying telemetry

**Start infrastructure:**
```bash
podman compose up -d
```

**Stop infrastructure:**
```bash
podman compose down
```

**View logs:**
```bash
podman compose logs kafka
podman compose logs pinot
```

**Ports:**
- `9092` - Kafka broker
- `9000` - Pinot Broker (query endpoint)
- `8099` - Pinot Controller (admin API)

## Scripts

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

**Prerequisites:**
- Infrastructure running: `podman compose up -d`
- OTEL-OQL service running: `./otel-oql --test-mode`

**Usage:**
```bash
./scripts/insert-test-data.sh
```

**Test queries after insertion:**
```bash
# Via OQL CLI (recommended)
./oql-cli --tenant-id=0 "signal=spans limit 10"
./oql-cli --tenant-id=0 "signal=metrics limit 10"
./oql-cli --tenant-id=0 "signal=logs limit 10"
```

```sql
-- Via Pinot SQL (direct query)
SELECT * FROM otel_spans WHERE tenant_id = 0;
SELECT * FROM otel_metrics WHERE tenant_id = 0;
SELECT * FROM otel_logs WHERE tenant_id = 0;
```

## Common Tasks

### Start Everything

```bash
# Full setup (recommended for first time)
./scripts/setup-all.sh

# Or start just infrastructure
podman compose up -d

# Start OTEL-OQL service
./otel-oql --test-mode
```

### Check Status

```bash
./scripts/verify-setup.sh
```

### View UIs

```bash
# Pinot Query Console
open http://localhost:9000

# Pinot Controller
open http://localhost:8099
```

### Stop Infrastructure

```bash
podman compose down
```

### Restart Infrastructure

```bash
podman compose restart
```

### View Logs

```bash
# All services
podman compose logs -f

# Specific service
podman compose logs -f kafka
podman compose logs -f pinot
```

### Delete Everything and Start Fresh

```bash
# Stop and remove all containers and volumes
podman compose down -v

# Re-run setup
./scripts/setup-all.sh
```

## Troubleshooting

### Script Won't Run

Make sure scripts are executable:
```bash
chmod +x scripts/*.sh
```

### Podman Compose Not Found

Install podman-compose:
```bash
# With pip
pip3 install podman-compose

# With Homebrew (macOS)
brew install podman-compose
```

### Infrastructure Won't Start

Check Podman is running:
```bash
podman info
```

View all containers:
```bash
podman compose ps
```

View service logs:
```bash
podman compose logs kafka
podman compose logs pinot
```

Restart everything:
```bash
podman compose down
podman compose up -d
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
lsof -i :9092   # Kafka
lsof -i :9000   # Pinot Broker
lsof -i :8099   # Pinot Controller
lsof -i :4317   # OTLP gRPC
lsof -i :4318   # OTLP HTTP
lsof -i :8080   # Query API
```

### Kafka Connection Issues

Check Kafka is healthy:
```bash
podman compose logs kafka
nc -z localhost 9092
```

Restart Kafka:
```bash
podman compose restart kafka
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

- **Podman** - Container runtime
- **podman-compose** - For managing multi-container setup
  ```bash
  # Install with pip
  pip3 install podman-compose

  # Or with Homebrew (macOS)
  brew install podman-compose
  ```
- **curl** - For health checks and API calls
- **Go 1.21+** - For building OTEL-OQL
- **nc** (netcat) - For port checking
- **lsof** - For service verification (optional)

## See Also

- [GETTING_STARTED.md](../GETTING_STARTED.md) - Detailed setup guide
- [README.md](../README.md) - Project documentation
- [examples/queries.md](../examples/queries.md) - OQL query examples
