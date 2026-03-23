package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	pinotBrokerURL     = "http://localhost:8000" // Broker for queries
	pinotControllerURL = "http://localhost:9000" // Controller for schema/table management
	otlpGRPCAddr       = "localhost:4317"
	otlpHTTPURL        = "http://localhost:4318"
	queryAPIURL        = "http://localhost:8080"
	testTenantID       = 0
)

// IsPinotRunning checks if Pinot is running and healthy
func IsPinotRunning(t *testing.T) bool {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(pinotBrokerURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// CleanupTestData deletes test data from Pinot tables
func CleanupTestData(t *testing.T, tenantID int) {
	t.Helper()

	client := pinot.NewClient(pinotBrokerURL)
	ctx := context.Background()

	tables := []string{"otel_spans", "otel_metrics", "otel_logs"}
	for _, table := range tables {
		sql := fmt.Sprintf("DELETE FROM %s WHERE tenant_id = %d", table, tenantID)
		// Note: Pinot doesn't support DELETE in all modes, so this might not work
		// In real tests, we might need to accept stale data or use unique tenant IDs
		_, _ = client.Query(ctx, sql)
	}
}

// QueryPinot executes a SQL query against Pinot
func QueryPinot(t *testing.T, sql string) ([]map[string]interface{}, error) {
	t.Helper()

	client := pinot.NewClient(pinotBrokerURL)
	ctx := context.Background()

	resp, err := client.Query(ctx, sql)
	if err != nil {
		return nil, err
	}

	// Convert rows to maps for easier assertions
	results := make([]map[string]interface{}, 0)
	if len(resp.ResultTable.DataSchema.ColumnNames) == 0 {
		return results, nil
	}

	for _, row := range resp.ResultTable.Rows {
		record := make(map[string]interface{})
		for i, colName := range resp.ResultTable.DataSchema.ColumnNames {
			if i < len(row) {
				record[colName] = row[i]
			}
		}
		results = append(results, record)
	}

	return results, nil
}

// QueryOQL executes an OQL query against the query API
func QueryOQL(t *testing.T, query string, tenantID int) (*OQLResponse, error) {
	t.Helper()

	reqBody := map[string]string{"query": query}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", queryAPIURL+"/query", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Tenant-ID", strconv.Itoa(tenantID))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query failed with status %d", resp.StatusCode)
	}

	var result OQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// OQLResponse represents the response from the OQL API
type OQLResponse struct {
	Results []OQLResult `json:"results"`
}

// OQLResult represents a single query result
type OQLResult struct {
	SQL     string                   `json:"sql"`
	Columns []string                 `json:"columns"`
	Rows    [][]interface{}          `json:"rows"`
	Stats   map[string]interface{}   `json:"stats"`
}

// CreateTestSpan creates a test span with the given parameters
// traceID and spanID should be hex strings (e.g., "0123456789abcdef0123456789abcdef")
func CreateTestSpan(traceID, spanID, name, serviceName string, statusCode int, duration time.Duration) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()

	// Set resource attributes
	rs.Resource().Attributes().PutStr("service.name", serviceName)

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	// Parse trace ID from hex string
	traceIDHex := traceID
	if len(traceIDHex) < 32 {
		// Pad to 32 hex chars
		for len(traceIDHex) < 32 {
			traceIDHex += "0"
		}
	}
	traceIDBytes, _ := hex.DecodeString(traceIDHex)
	var tid [16]byte
	copy(tid[:], traceIDBytes)
	span.SetTraceID(pcommon.TraceID(tid))

	// Parse span ID from hex string
	spanIDHex := spanID
	if len(spanIDHex) < 16 {
		// Pad to 16 hex chars
		for len(spanIDHex) < 16 {
			spanIDHex += "0"
		}
	}
	spanIDBytes, _ := hex.DecodeString(spanIDHex)
	var sid [8]byte
	copy(sid[:], spanIDBytes)
	span.SetSpanID(pcommon.SpanID(sid))

	span.SetName(name)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(duration)))

	// Set attributes
	if statusCode > 0 {
		span.Attributes().PutInt("http.status_code", int64(statusCode))
		span.Attributes().PutStr("http.method", "GET")
	}

	return traces
}

// CreateTestMetric creates a test metric with exemplar
func CreateTestMetric(metricName, serviceName string, value float64, exemplarTraceID, exemplarSpanID string) pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()

	// Set resource attributes
	rm.Resource().Attributes().PutStr("service.name", serviceName)

	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()

	metric.SetName(metricName)
	metric.SetEmptyGauge()

	dp := metric.Gauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	dp.SetDoubleValue(value)

	// Add exemplar if trace ID provided
	if exemplarTraceID != "" {
		exemplar := dp.Exemplars().AppendEmpty()
		exemplar.SetDoubleValue(value)
		exemplar.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

		// Parse trace ID from hex string
		traceIDHex := exemplarTraceID
		if len(traceIDHex) < 32 {
			// Pad to 32 hex chars
			for len(traceIDHex) < 32 {
				traceIDHex += "0"
			}
		}
		traceIDBytes, _ := hex.DecodeString(traceIDHex)
		var tid [16]byte
		copy(tid[:], traceIDBytes)
		exemplar.SetTraceID(pcommon.TraceID(tid))

		if exemplarSpanID != "" {
			spanIDHex := exemplarSpanID
			if len(spanIDHex) < 16 {
				// Pad to 16 hex chars
				for len(spanIDHex) < 16 {
					spanIDHex += "0"
				}
			}
			spanIDBytes, _ := hex.DecodeString(spanIDHex)
			var sid [8]byte
			copy(sid[:], spanIDBytes)
			exemplar.SetSpanID(pcommon.SpanID(sid))
		}
	}

	return metrics
}

// CreateTestLog creates a test log entry
func CreateTestLog(traceID, spanID, body, severity, serviceName string) plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()

	// Set resource attributes
	rl.Resource().Attributes().PutStr("service.name", serviceName)

	sl := rl.ScopeLogs().AppendEmpty()
	logRecord := sl.LogRecords().AppendEmpty()

	logRecord.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	logRecord.Body().SetStr(body)
	logRecord.SetSeverityText(severity)

	// Set trace context
	if traceID != "" {
		traceIDHex := traceID
		if len(traceIDHex) < 32 {
			// Pad to 32 hex chars
			for len(traceIDHex) < 32 {
				traceIDHex += "0"
			}
		}
		traceIDBytes, _ := hex.DecodeString(traceIDHex)
		var tid [16]byte
		copy(tid[:], traceIDBytes)
		logRecord.SetTraceID(pcommon.TraceID(tid))
	}

	if spanID != "" {
		spanIDHex := spanID
		if len(spanIDHex) < 16 {
			// Pad to 16 hex chars
			for len(spanIDHex) < 16 {
				spanIDHex += "0"
			}
		}
		spanIDBytes, _ := hex.DecodeString(spanIDHex)
		var sid [8]byte
		copy(sid[:], spanIDBytes)
		logRecord.SetSpanID(pcommon.SpanID(sid))
	}

	// Set log level attribute
	logRecord.Attributes().PutStr("log.level", severity)

	return logs
}

// SendTracesHTTP sends traces via OTLP HTTP
func SendTracesHTTP(t *testing.T, traces ptrace.Traces, tenantID int) error {
	t.Helper()

	// Marshal traces to JSON (OTLP HTTP uses protobuf, but for testing we might use JSON)
	// This is simplified - in real implementation, we'd use protobuf encoding
	marshaler := ptrace.ProtoMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return fmt.Errorf("failed to marshal traces: %w", err)
	}

	req, err := http.NewRequest("POST", otlpHTTPURL+"/v1/traces", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("tenant-id", strconv.Itoa(tenantID))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send traces: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("send failed with status %d", resp.StatusCode)
	}

	// Give Pinot time to ingest from Kafka (REALTIME tables need more time)
	time.Sleep(5 * time.Second)

	return nil
}

// SendMetricsHTTP sends metrics via OTLP HTTP
func SendMetricsHTTP(t *testing.T, metrics pmetric.Metrics, tenantID int) error {
	t.Helper()

	marshaler := pmetric.ProtoMarshaler{}
	data, err := marshaler.MarshalMetrics(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	req, err := http.NewRequest("POST", otlpHTTPURL+"/v1/metrics", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("tenant-id", strconv.Itoa(tenantID))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("send failed with status %d", resp.StatusCode)
	}

	// Give Pinot time to ingest from Kafka (REALTIME tables need more time)
	time.Sleep(5 * time.Second)

	return nil
}

// SendLogsHTTP sends logs via OTLP HTTP
func SendLogsHTTP(t *testing.T, logs plog.Logs, tenantID int) error {
	t.Helper()

	marshaler := plog.ProtoMarshaler{}
	data, err := marshaler.MarshalLogs(logs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	req, err := http.NewRequest("POST", otlpHTTPURL+"/v1/logs", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("tenant-id", strconv.Itoa(tenantID))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("send failed with status %d", resp.StatusCode)
	}

	// Give Pinot time to ingest from Kafka (REALTIME tables need more time)
	time.Sleep(5 * time.Second)

	return nil
}

// WaitForPinot waits for Pinot to be ready
func WaitForPinot(t *testing.T, timeout time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(pinotBrokerURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("pinot not ready after %v", timeout)
}

// WaitForOtelOQL waits for the OTEL-OQL service to be ready
func WaitForOtelOQL(t *testing.T, timeout time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(queryAPIURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("otel-oql service not ready after %v", timeout)
}
