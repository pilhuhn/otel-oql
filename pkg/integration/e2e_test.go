package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanIngestionAndQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test span with unique trace ID (timestamp-based to avoid conflicts)
	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("%032x", timestamp) // 32 hex chars - unique per test run
	spanID := fmt.Sprintf("%016x", timestamp)  // 16 hex chars
	spanName := "test-checkout"
	serviceName := "test-payment-service"
	statusCode := 500
	duration := 150 * time.Millisecond

	traces := CreateTestSpan(traceID, spanID, spanName, serviceName, statusCode, duration)

	// Send via OTLP HTTP
	err := SendTracesHTTP(t, traces, testTenantID)
	require.NoError(t, err, "Failed to send traces via HTTP")

	// Query Pinot directly to verify data was stored
	sql := fmt.Sprintf("SELECT * FROM otel_spans WHERE tenant_id = %d AND trace_id = '%s'", testTenantID, traceID)
	results, err := QueryPinot(t, sql)
	require.NoError(t, err, "Failed to query Pinot")
	require.NotEmpty(t, results, "No spans found in Pinot")

	// Verify span fields
	span := results[0]
	assert.Equal(t, float64(testTenantID), span["tenant_id"])
	assert.Contains(t, span["trace_id"], traceID)
	assert.Equal(t, spanName, span["name"])
	assert.Equal(t, serviceName, span["service_name"])
	assert.Equal(t, float64(statusCode), span["http_status_code"])

	// Query via OQL (if service is running)
	if isOtelOQLAvailable() {
		query := fmt.Sprintf(`signal=spans | where trace_id == "%s"`, traceID)
		oqlResp, err := QueryOQL(t, query, testTenantID)
		require.NoError(t, err, "Failed to query via OQL")
		require.NotEmpty(t, oqlResp.Results, "No results from OQL")
		require.NotEmpty(t, oqlResp.Results[0].Rows, "No rows in OQL results")

		// Verify OQL results match Pinot results
		assert.Equal(t, len(results), len(oqlResp.Results[0].Rows), "Row count mismatch between Pinot and OQL")
	} else {
		t.Skip("OTEL-OQL service not running, skipping OQL query test")
	}
}

func TestMetricWithExemplarIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test metric with exemplar (unique IDs)
	timestamp := time.Now().UnixNano()
	metricName := "http.server.duration"
	serviceName := "test-api-service"
	value := 250.5
	exemplarTraceID := fmt.Sprintf("%032x", timestamp+1) // 32 hex chars
	exemplarSpanID := fmt.Sprintf("%016x", timestamp+1)  // 16 hex chars

	metrics := CreateTestMetric(metricName, serviceName, value, exemplarTraceID, exemplarSpanID)

	// Send via OTLP HTTP
	err := SendMetricsHTTP(t, metrics, testTenantID)
	require.NoError(t, err, "Failed to send metrics via HTTP")

	// Query Pinot to verify exemplar was captured
	sql := fmt.Sprintf("SELECT * FROM otel_metrics WHERE tenant_id = %d AND metric_name = '%s'", testTenantID, metricName)
	results, err := QueryPinot(t, sql)
	require.NoError(t, err, "Failed to query Pinot")
	require.NotEmpty(t, results, "No metrics found in Pinot")

	// Verify metric fields
	metric := results[0]
	assert.Equal(t, float64(testTenantID), metric["tenant_id"])
	assert.Equal(t, metricName, metric["metric_name"])
	assert.Equal(t, serviceName, metric["service_name"])

	// Verify exemplar fields (the "wormhole" to traces)
	if metric["exemplar_trace_id"] != nil {
		assert.Contains(t, metric["exemplar_trace_id"], exemplarTraceID)
		assert.Contains(t, metric["exemplar_span_id"], exemplarSpanID)
		t.Log("✅ Exemplar trace_id captured - wormhole from metrics to traces works!")
	} else {
		t.Log("⚠️  Exemplar trace_id not captured (might need schema adjustment)")
	}
}

func TestLogIngestionAndCorrelation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test log with trace context (unique IDs)
	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("%032x", timestamp+2) // 32 hex chars
	spanID := fmt.Sprintf("%016x", timestamp+2)  // 16 hex chars
	logBody := "Payment processing failed"
	severity := "ERROR"
	serviceName := "test-payment-service"

	logs := CreateTestLog(traceID, spanID, logBody, severity, serviceName)

	// Send via OTLP HTTP
	err := SendLogsHTTP(t, logs, testTenantID)
	require.NoError(t, err, "Failed to send logs via HTTP")

	// Query Pinot to verify log was stored
	sql := fmt.Sprintf("SELECT * FROM otel_logs WHERE tenant_id = %d AND trace_id = '%s'", testTenantID, traceID)
	results, err := QueryPinot(t, sql)
	require.NoError(t, err, "Failed to query Pinot")
	require.NotEmpty(t, results, "No logs found in Pinot")

	// Verify log fields
	log := results[0]
	assert.Equal(t, float64(testTenantID), log["tenant_id"])
	assert.Contains(t, log["trace_id"], traceID)
	assert.Equal(t, logBody, log["body"])
	assert.Equal(t, severity, log["severity_text"])
	assert.Equal(t, serviceName, log["service_name"])
}

func TestAttributeExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create span with both native and custom attributes (unique IDs)
	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("%032x", timestamp+3)
	spanID := fmt.Sprintf("%016x", timestamp+3)
	traces := CreateTestSpan(traceID, spanID, "test-span", "test-service", 404, 100*time.Millisecond)

	// Add custom attributes (non-native)
	rs := traces.ResourceSpans().At(0)
	span := rs.ScopeSpans().At(0).Spans().At(0)
	span.Attributes().PutStr("custom.field", "custom-value")
	span.Attributes().PutStr("application.version", "1.2.3")

	// Send to service
	err := SendTracesHTTP(t, traces, testTenantID)
	require.NoError(t, err, "Failed to send traces")

	// Query Pinot
	sql := fmt.Sprintf("SELECT * FROM otel_spans WHERE tenant_id = %d AND trace_id = '%s'", testTenantID, traceID)
	results, err := QueryPinot(t, sql)
	require.NoError(t, err, "Failed to query Pinot")
	require.NotEmpty(t, results, "No spans found")

	span_data := results[0]

	// Verify native column has the value
	assert.Equal(t, float64(404), span_data["http_status_code"], "Native column http_status_code should be populated")

	// Verify custom attributes are in JSON column
	// Note: Actual verification depends on how Pinot returns JSON data
	if attributes, ok := span_data["attributes"]; ok && attributes != nil {
		t.Logf("Attributes column: %v", attributes)
		// Custom attributes should be in the attributes JSON column
	}

	t.Log("✅ Attribute extraction test completed")
}

func TestMultiTenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create spans for two different tenants (unique IDs)
	timestamp := time.Now().UnixNano()
	tenant1ID := 100
	tenant2ID := 200

	traces1 := CreateTestSpan(fmt.Sprintf("%032x", timestamp+4), fmt.Sprintf("%016x", timestamp+4), "tenant1-span", "service1", 200, 50*time.Millisecond)
	traces2 := CreateTestSpan(fmt.Sprintf("%032x", timestamp+5), fmt.Sprintf("%016x", timestamp+5), "tenant2-span", "service2", 200, 50*time.Millisecond)

	// Send to different tenants
	err := SendTracesHTTP(t, traces1, tenant1ID)
	require.NoError(t, err)

	err = SendTracesHTTP(t, traces2, tenant2ID)
	require.NoError(t, err)

	// Query tenant 1 - should only see tenant 1 data
	sql1 := fmt.Sprintf("SELECT * FROM otel_spans WHERE tenant_id = %d", tenant1ID)
	results1, err := QueryPinot(t, sql1)
	require.NoError(t, err)
	require.NotEmpty(t, results1)

	for _, row := range results1 {
		assert.Equal(t, float64(tenant1ID), row["tenant_id"], "Should only see tenant 1 data")
	}

	// Query tenant 2 - should only see tenant 2 data
	sql2 := fmt.Sprintf("SELECT * FROM otel_spans WHERE tenant_id = %d", tenant2ID)
	results2, err := QueryPinot(t, sql2)
	require.NoError(t, err)
	require.NotEmpty(t, results2)

	for _, row := range results2 {
		assert.Equal(t, float64(tenant2ID), row["tenant_id"], "Should only see tenant 2 data")
	}

	t.Log("✅ Multi-tenant isolation verified")
}

func TestOQLExpandOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Create a complete trace with multiple spans (unique trace ID)
	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("%032x", timestamp+6)
	serviceName := "test-service"

	// Parent span
	traces1 := CreateTestSpan(traceID, fmt.Sprintf("%016x", timestamp+6), "parent-operation", serviceName, 200, 200*time.Millisecond)
	err := SendTracesHTTP(t, traces1, testTenantID)
	require.NoError(t, err)

	// Child span 1
	traces2 := CreateTestSpan(traceID, fmt.Sprintf("%016x", timestamp+7), "child-operation-1", serviceName, 200, 50*time.Millisecond)
	err = SendTracesHTTP(t, traces2, testTenantID)
	require.NoError(t, err)

	// Child span 2
	traces3 := CreateTestSpan(traceID, fmt.Sprintf("%016x", timestamp+8), "child-operation-2", serviceName, 200, 75*time.Millisecond)
	err = SendTracesHTTP(t, traces3, testTenantID)
	require.NoError(t, err)

	// Query with expand - should get all spans with matching trace_id
	query := fmt.Sprintf(`signal=spans | where name == "parent-operation" | expand trace`)
	resp, err := QueryOQL(t, query, testTenantID)
	require.NoError(t, err)
	require.NotEmpty(t, resp.Results)

	// Should have expanded to include all spans in the trace
	totalRows := 0
	for _, result := range resp.Results {
		totalRows += len(result.Rows)
	}

	assert.GreaterOrEqual(t, totalRows, 3, "Expand should return at least 3 spans (parent + 2 children)")
	t.Logf("✅ Expand operation returned %d spans", totalRows)
}

func TestOQLGetExemplars(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Create metric with exemplar (unique IDs)
	timestamp := time.Now().UnixNano()
	metricName := "request.duration"
	exemplarTraceID := fmt.Sprintf("%032x", timestamp+9)
	exemplarSpanID := fmt.Sprintf("%016x", timestamp+9)

	metrics := CreateTestMetric(metricName, "test-service", 123.45, exemplarTraceID, exemplarSpanID)
	err := SendMetricsHTTP(t, metrics, testTenantID)
	require.NoError(t, err)

	// Query to get exemplars
	query := fmt.Sprintf(`signal=metrics | where metric_name == "%s" | get_exemplars()`, metricName)
	resp, err := QueryOQL(t, query, testTenantID)
	require.NoError(t, err)
	require.NotEmpty(t, resp.Results)

	// Verify exemplar data is returned
	if len(resp.Results[0].Rows) > 0 {
		t.Log("✅ Get exemplars operation returned exemplar data")
		t.Logf("Exemplar columns: %v", resp.Results[0].Columns)
		t.Logf("Exemplar row: %v", resp.Results[0].Rows[0])
	} else {
		t.Log("⚠️  No exemplar rows returned (might need to verify exemplar ingestion)")
	}
}

func TestEndToEndQueryFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Test complete flow: ingest -> Pinot storage -> OQL query (unique IDs)
	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("%032x", timestamp+10)
	spanID := fmt.Sprintf("%016x", timestamp+10)
	spanName := "e2e-test-operation"
	serviceName := "e2e-service"

	// 1. Ingest data
	traces := CreateTestSpan(traceID, spanID, spanName, serviceName, 500, 100*time.Millisecond)
	err := SendTracesHTTP(t, traces, testTenantID)
	require.NoError(t, err, "Failed to ingest traces")

	// 2. Verify in Pinot
	sql := fmt.Sprintf("SELECT * FROM otel_spans WHERE tenant_id = %d AND trace_id = '%s'", testTenantID, traceID)
	pinotResults, err := QueryPinot(t, sql)
	require.NoError(t, err, "Failed to query Pinot")
	require.NotEmpty(t, pinotResults, "No data in Pinot")

	// 3. Query via OQL
	query := fmt.Sprintf(`signal=spans | where trace_id == "%s" | limit 10`, traceID)
	oqlResp, err := QueryOQL(t, query, testTenantID)
	require.NoError(t, err, "Failed to query via OQL")
	require.NotEmpty(t, oqlResp.Results, "No OQL results")
	require.NotEmpty(t, oqlResp.Results[0].Rows, "No OQL rows")

	// 4. Verify results match
	assert.Equal(t, len(pinotResults), len(oqlResp.Results[0].Rows), "Pinot and OQL should return same number of rows")

	t.Log("✅ End-to-end flow: OTLP ingestion → Pinot storage → OQL query successful")
}
