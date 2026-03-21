# Getting Started with OTEL-OQL and Pinot

This guide walks through setting up Apache Pinot and initializing the OTEL-OQL schemas.

## Prerequisites

- Podman installed and running
- Go 1.21+ installed
- This repository cloned

## Step 1: Start Apache Pinot (Podman All-in-One)

### Start Pinot Container

```bash
# Pull the Pinot image
podman pull docker.io/apachepinot/pinot:latest

# Start Pinot in all-in-one mode
podman run \
  --name pinot-quickstart \
  -p 9000:9000 \
  -p 8099:8099 \
  -d \
  docker.io/apachepinot/pinot:latest QuickStart -type batch
```

**What this does:**
- Port 9000: Pinot Broker (query endpoint) - this is what OTEL-OQL connects to
- Port 8099: Pinot Controller (admin UI)
- `-type batch`: Runs Pinot in batch mode (simpler for testing)

### Verify Pinot is Running

```bash
# Check container is running
podman ps | grep pinot-quickstart

# Check Pinot is responding
curl http://localhost:9000/health
# Should return: {"status":"OK"}

# Open Pinot UI in browser
open http://localhost:9000
```

**Pinot UI** should show:
- Dashboard with cluster status
- Query Console for running SQL
- Schema/Table management

## Step 2: Build OTEL-OQL

```bash
cd /Users/hrupp/src/otel-oql

# Build the binary
go build -o otel-oql ./cmd/otel-oql

# Verify build
./otel-oql --help
```

## Step 3: Initialize Schemas

### Run Schema Setup

```bash
# This creates the schemas and tables in Pinot
./otel-oql setup-schema --pinot-url=http://localhost:9000
```

**What this does:**
1. Creates schema definitions for:
   - `otel_spans` - Trace/span data
   - `otel_metrics` - Metrics data
   - `otel_logs` - Log data

2. Creates table configurations with:
   - Tenant-based partitioning
   - Inverted indexes on key columns
   - JSON indexes for flexible attributes

### Verify Schemas Were Created

**Option 1: Using Pinot UI**
```bash
open http://localhost:9000
```
- Go to "Tables" tab
- You should see: `otel_spans`, `otel_metrics`, `otel_logs`

**Option 2: Using curl**
```bash
# List all tables
curl http://localhost:9000/tables

# Get schema for spans table
curl http://localhost:9000/schemas/otel_spans

# Get table config for spans
curl http://localhost:9000/tables/otel_spans
```

**Expected Output:**
```json
{
  "tables": [
    "otel_spans",
    "otel_metrics",
    "otel_logs"
  ]
}
```

## Step 4: Start OTEL-OQL Service

### Start in Test Mode

```bash
# Test mode allows ingestion without tenant-id header (uses tenant-id=0)
./otel-oql --test-mode --pinot-url=http://localhost:9000
```

**You should see:**
```
Starting OTEL-OQL service...
Pinot URL: http://localhost:9000
OTLP gRPC Port: 4317
OTLP HTTP Port: 4318
Query API Port: 8080
Test Mode: true
OTLP gRPC receiver listening on port 4317
OTLP HTTP receiver listening on port 4318
Query API server listening on port 8080
OTEL-OQL service started successfully
```

### Verify Services Are Running

```bash
# In another terminal, check all ports
lsof -i :4317  # OTLP gRPC
lsof -i :4318  # OTLP HTTP
lsof -i :8080  # Query API
```

## Step 5: Test with Sample Data

### Option A: Manual SQL Insert (Quick Test)

Use Pinot UI to insert test data:

```bash
open http://localhost:9000/#/query
```

**Insert a test span:**
```sql
INSERT INTO otel_spans (
  tenant_id, trace_id, span_id, name,
  service_name, http_status_code, timestamp, duration
) VALUES (
  0, 'test-trace-123', 'span-001', 'checkout',
  'payment-service', 500, 1710000000000, 150000000
);
```

**Query it:**
```sql
SELECT * FROM otel_spans
WHERE tenant_id = 0
LIMIT 10;
```

### Option B: Send OTLP Data (Real Test)

**Install OpenTelemetry Collector (optional):**
```bash
# Using Podman
podman run -d --name otel-collector \
  -p 4317:4317 \
  -p 4318:4318 \
  --network host \
  docker.io/otel/opentelemetry-collector-contrib:latest
```

**Or use a simple Go test:**

Create `test_ingest.go`:
```go
package main

import (
    "context"
    "log"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
    ctx := context.Background()

    // Create OTLP exporter
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint("localhost:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Create tracer provider
    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
    )
    otel.SetTracerProvider(tp)

    // Create a span
    tracer := tp.Tracer("test")
    _, span := tracer.Start(ctx, "test-span")
    span.End()

    // Flush
    tp.Shutdown(ctx)

    log.Println("Sent test span!")
}
```

Run it:
```bash
go run test_ingest.go
```

### Option C: Test Query API

**Query using OQL:**
```bash
curl -X POST http://localhost:8080/query \
  -H "X-Tenant-ID: 0" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "signal=spans | where tenant_id == 0 | limit 10"
  }'
```

**Expected response:**
```json
{
  "results": [
    {
      "sql": "SELECT * FROM otel_spans WHERE tenant_id = 0 AND tenant_id = 0 LIMIT 10",
      "columns": ["tenant_id", "trace_id", "span_id", "name", ...],
      "rows": [
        [0, "test-trace-123", "span-001", "checkout", ...]
      ],
      "stats": {
        "numDocsScanned": 1,
        "totalDocs": 1,
        "timeUsedMs": 5
      }
    }
  ]
}
```

## Troubleshooting

### Schema Setup Fails

**Issue:** `./otel-oql setup-schema` returns errors

**Check:**
```bash
# Verify Pinot is responding
curl http://localhost:9000/health

# Check Pinot controller logs
podman logs pinot-quickstart

# Try to list existing tables
curl http://localhost:9000/tables
```

**Common fixes:**
- Make sure Pinot container is running: `podman ps`
- Wait 30 seconds after starting Pinot (initialization takes time)
- Check firewall isn't blocking port 9000
- Try using `http://host.containers.internal:9000` if running on Mac

### Data Not Showing Up

**Issue:** Send data but queries return empty results

**Debug:**
```bash
# Check data is in Pinot directly
curl -X POST http://localhost:9000/query/sql \
  -H "Content-Type: application/json" \
  -d '{"sql": "SELECT COUNT(*) FROM otel_spans"}'

# Check Pinot query console
open http://localhost:9000/#/query
# Run: SELECT * FROM otel_spans LIMIT 10
```

**Common causes:**
- Data sent to wrong port (check 4317 for gRPC, 4318 for HTTP)
- Tenant-id mismatch (use tenant-id=0 in test mode)
- Data still in buffer (Pinot batches writes)

### OTLP Receiver Not Working

**Issue:** Can't send OTLP data to ports 4317/4318

**Check:**
```bash
# Verify OTEL-OQL is running
lsof -i :4317
lsof -i :4318

# Check for port conflicts
netstat -an | grep 4317
netstat -an | grep 4318
```

## Advanced Configuration

### Production Mode (Require Tenant-ID)

```bash
# Remove --test-mode flag
./otel-oql --pinot-url=http://localhost:9000

# Now all requests MUST include tenant-id
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

## Next Steps

1. **Explore Pinot UI**: http://localhost:9000
   - Run SQL queries
   - View table schemas
   - Monitor cluster health

2. **Send Real OTLP Data**:
   - Configure your app to send to `localhost:4317` (gRPC)
   - Or use OpenTelemetry Collector

3. **Try OQL Queries**:
   - See `examples/queries.md` for query examples
   - Test cross-signal correlation
   - Try the "wormhole" with exemplars

4. **Monitor Performance**:
   - Watch Pinot query times in UI
   - Compare native column queries vs JSON queries
   - Check segment size and count

## Stopping and Cleaning Up

### Stop Services

```bash
# Stop OTEL-OQL (Ctrl+C in terminal)

# Stop Pinot
podman stop pinot-quickstart

# Remove Pinot container
podman rm pinot-quickstart
```

### Clean Data

```bash
# Remove Pinot data volumes
podman volume ls | grep pinot
podman volume rm <volume-name>
```

### Remove Schemas

If you need to recreate schemas:

```bash
# Delete tables via Pinot UI
open http://localhost:9000/#/tables
# Click "Delete" for each table

# Or via API
curl -X DELETE http://localhost:9000/tables/otel_spans
curl -X DELETE http://localhost:9000/tables/otel_metrics
curl -X DELETE http://localhost:9000/tables/otel_logs

# Then re-run setup
./otel-oql setup-schema --pinot-url=http://localhost:9000
```

## Known Issues

### Schema Creation Might Need Adjustment

⚠️ **Important**: The schema implementation has been fixed but not yet tested against actual Pinot. You may encounter:

1. **JSON type not supported**: Pinot might not support `JSON` datatype in all versions
   - **Workaround**: Change `DataType: "JSON"` to `DataType: "STRING"` in `pkg/pinot/schema.go`
   - This will store JSON as string (still queryable with JSON functions)

2. **Schema API format mismatch**: Pinot's schema API might expect different field names
   - Check Pinot version: `curl http://localhost:9000/version`
   - Refer to Pinot docs for your version's schema format

3. **Table creation endpoint differences**: Separate schema/table creation might not match your Pinot version
   - May need to combine into single request
   - Check `pkg/pinot/client.go` CreateSchema/CreateTable methods

If you encounter issues, please report them so we can fix the schema format!

## Resources

- [Pinot Documentation](https://docs.pinot.apache.org/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [OTEL-OQL Examples](./examples/queries.md)
- [Schema Implementation Details](./SCHEMA_CHANGES.md)
