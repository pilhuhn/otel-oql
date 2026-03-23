# Testing Guide for OTEL-OQL

This document describes how to run tests for the OTEL-OQL project.

## Test Structure

The project has three types of tests:

1. **Unit Tests** - Fast, no external dependencies
   - OQL parser tests (`pkg/oql/parser_test.go`)
   - SQL translator tests (`pkg/translator/translator_test.go`)

2. **Integration Tests** - Require running Pinot instance
   - End-to-end tests (`pkg/integration/e2e_test.go`)
   - Test helpers (`pkg/integration/helpers_test.go`)
   - TestMain infrastructure (`pkg/integration/integration_test.go`)

## Running Unit Tests

Unit tests run quickly and don't require any external services.

```bash
# Run all unit tests
go test ./pkg/oql ./pkg/translator -v

# Run with short flag (skips integration tests)
go test -short ./... -v

# Run specific package
go test ./pkg/oql -v

# Run with coverage
go test ./pkg/oql ./pkg/translator -cover
```

**Expected output:**
- Parser tests: ~25 test cases covering all OQL operators
- Translator tests: ~20 test cases covering SQL generation

All unit tests should pass ✅

## Running Integration Tests

Integration tests require:
1. Apache Pinot running on `localhost:9000`
2. Schemas created (`otel_spans`, `otel_metrics`, `otel_logs`)
3. OTEL-OQL service running on `localhost:4317/4318/8080` (optional for some tests)

### Setup

```bash
# 1. Start Pinot
./scripts/start-pinot.sh

# 2. Create schemas
./otel-oql setup-schema --pinot-url=http://localhost:9000

# 3. (Optional) Start OTEL-OQL service for full E2E tests
./otel-oql --test-mode --pinot-url=http://localhost:9000
```

### Run Integration Tests

```bash
# Run all integration tests
go test ./pkg/integration -v

# Run specific test
go test ./pkg/integration -run TestSpanIngestionAndQuery -v

# Run with timeout (integration tests can take 30+ seconds)
go test ./pkg/integration -v -timeout 5m
```

### Integration Test Coverage

The integration tests verify:

- **TestSpanIngestionAndQuery**
  - ✅ Send span via OTLP HTTP
  - ✅ Verify data in Pinot SQL
  - ✅ Query via OQL
  - ✅ Results match between Pinot and OQL

- **TestMetricWithExemplarIngestion**
  - ✅ Send metric with exemplar
  - ✅ Verify exemplar trace_id captured
  - ✅ Test the "wormhole" from metrics to traces

- **TestLogIngestionAndCorrelation**
  - ✅ Send log with trace context
  - ✅ Verify log storage in Pinot
  - ✅ Verify trace_id correlation

- **TestAttributeExtraction**
  - ✅ Verify native column extraction (http.status_code)
  - ✅ Verify custom attributes in JSON column
  - ✅ No duplication between native and JSON

- **TestMultiTenantIsolation**
  - ✅ Send data to tenant 100 and 200
  - ✅ Verify tenant 100 only sees its data
  - ✅ Verify tenant 200 only sees its data

- **TestOQLExpandOperation**
  - ✅ Create trace with multiple spans
  - ✅ Query with expand operation
  - ✅ Verify all spans in trace returned

- **TestOQLGetExemplars**
  - ✅ Create metric with exemplar
  - ✅ Use get_exemplars() operation
  - ✅ Verify exemplar data extracted

- **TestEndToEndQueryFlow**
  - ✅ Complete flow: OTLP → Pinot → OQL
  - ✅ Verify consistency across the stack

## Test Modes

### Short Mode (Unit Tests Only)

```bash
go test -short ./...
```

Skips all integration tests. Use this for:
- Quick validation during development
- CI/CD pre-commit checks
- When Pinot is not available

### Full Mode (Unit + Integration)

```bash
# Requires Pinot running
./scripts/start-pinot.sh
./otel-oql setup-schema --pinot-url=http://localhost:9000
./otel-oql --test-mode --pinot-url=http://localhost:9000 &

# Run all tests
go test ./...
```

Use this for:
- Pre-release validation
- Full stack verification
- Performance testing

## Troubleshooting

### Integration Tests Skip/Fail

**Problem:** `Pinot is not running or not accessible`

**Solution:**
```bash
# Check Pinot is running
curl http://localhost:9000/health

# Start if needed
./scripts/start-pinot.sh

# Verify schemas exist
curl http://localhost:9000/tables
```

**Problem:** `OTEL-OQL service not running`

**Solution:**
Some tests will skip if the service isn't running. To run all tests:
```bash
./otel-oql --test-mode --pinot-url=http://localhost:9000
```

### Tests Timeout

**Problem:** Integration tests timeout after 2 minutes

**Solution:**
```bash
# Increase timeout
go test ./pkg/integration -v -timeout 10m
```

### Stale Test Data

**Problem:** Tests fail due to data from previous runs

**Solution:**
Integration tests use unique IDs, but if needed:
```bash
# Stop and remove Pinot container (clears all data)
podman stop pinot-quickstart
podman rm pinot-quickstart

# Restart fresh
./scripts/setup-all.sh
```

## Test Data

Integration tests use:
- **Tenant ID:** 0 (test mode default)
- **Trace IDs:** Prefixed with test names (e.g., `test-trace-123`, `expand-trace-001`)
- **Service Names:** Usually `test-service` or `payment-service`

Test data is left in Pinot for inspection. You can view it in the Pinot UI:
```bash
open http://localhost:9000/#/query
```

Example queries:
```sql
SELECT * FROM otel_spans WHERE tenant_id = 0;
SELECT * FROM otel_metrics WHERE tenant_id = 0;
SELECT * FROM otel_logs WHERE tenant_id = 0;
```

## CI/CD Integration

For CI/CD pipelines:

```bash
# Stage 1: Unit tests (fast, no dependencies)
go test -short ./... -v

# Stage 2: Build
go build -o otel-oql ./cmd/otel-oql

# Stage 3: Integration tests (requires Pinot)
# Start Pinot in CI environment
podman run -d --name pinot-test -p 9000:9000 \
  docker.io/apachepinot/pinot:latest QuickStart -type batch

# Wait for Pinot
sleep 30

# Create schemas
./otel-oql setup-schema --pinot-url=http://localhost:9000

# Run integration tests
go test ./pkg/integration -v -timeout 10m

# Cleanup
podman stop pinot-test
podman rm pinot-test
```

## Coverage Goals

- **Parser:** >90% (critical path, no external deps)
- **Translator:** >85% (complex logic, important for correctness)
- **Integration:** 100% of E2E flows tested

Check coverage:
```bash
go test ./pkg/oql ./pkg/translator -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Maintenance

When adding new features:

1. **Add parser tests** if new OQL syntax
2. **Add translator tests** if new SQL generation logic
3. **Add integration tests** if new data flow or operation

Keep tests:
- **Independent** - Each test can run standalone
- **Deterministic** - Same input → same output
- **Fast** - Unit tests < 1s, integration tests < 30s per test
- **Clear** - Good test names and failure messages
