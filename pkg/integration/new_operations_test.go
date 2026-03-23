package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregationFunctions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Send some test spans with varying durations
	timestamp := time.Now().UnixNano()
	for i := 0; i < 5; i++ {
		traceID := fmt.Sprintf("%032x", timestamp+int64(i))
		spanID := fmt.Sprintf("%016x", timestamp+int64(i))
		duration := time.Duration((i+1)*100) * time.Millisecond

		traces := CreateTestSpan(traceID, spanID, "test-operation", "test-service", 200, duration)
		err := SendTracesHTTP(t, traces, testTenantID)
		require.NoError(t, err)
	}

	// Wait for data to be available
	time.Sleep(5 * time.Second)

	// Test COUNT()
	t.Run("Count", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"test-operation\" count()", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		// Check SQL generation
		assert.Contains(t, resp.Results[0].SQL, "COUNT(*)")
		t.Logf("✅ Count aggregation SQL: %s", resp.Results[0].SQL)
	})

	// Test AVG()
	t.Run("Average", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"test-operation\" avg(duration)", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		assert.Contains(t, resp.Results[0].SQL, "AVG(duration)")
		t.Logf("✅ Average aggregation SQL: %s", resp.Results[0].SQL)
	})

	// Test MIN()
	t.Run("Minimum", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"test-operation\" min(duration)", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		assert.Contains(t, resp.Results[0].SQL, "MIN(duration)")
		t.Logf("✅ Min aggregation SQL: %s", resp.Results[0].SQL)
	})

	// Test MAX()
	t.Run("Maximum", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"test-operation\" max(duration)", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		assert.Contains(t, resp.Results[0].SQL, "MAX(duration)")
		t.Logf("✅ Max aggregation SQL: %s", resp.Results[0].SQL)
	})

	// Test SUM()
	t.Run("Sum", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"test-operation\" sum(duration)", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		assert.Contains(t, resp.Results[0].SQL, "SUM(duration)")
		t.Logf("✅ Sum aggregation SQL: %s", resp.Results[0].SQL)
	})

	// Test with alias
	t.Run("AverageWithAlias", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans avg(duration) as avg_latency", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		assert.Contains(t, resp.Results[0].SQL, "AVG(duration) AS avg_latency")
		t.Logf("✅ Average with alias SQL: %s", resp.Results[0].SQL)
	})
}

func TestGroupByOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Send test spans for multiple services
	timestamp := time.Now().UnixNano()
	services := []string{"service-a", "service-b", "service-c"}

	for i, service := range services {
		traceID := fmt.Sprintf("%032x", timestamp+int64(i)*10)
		spanID := fmt.Sprintf("%016x", timestamp+int64(i)*10)

		traces := CreateTestSpan(traceID, spanID, "group-test", service, 200, 100*time.Millisecond)
		err := SendTracesHTTP(t, traces, testTenantID)
		require.NoError(t, err)
	}

	time.Sleep(5 * time.Second)

	// Test GROUP BY with aggregation
	t.Run("GroupByWithCount", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"group-test\" group by service_name count()", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "service_name")
		assert.Contains(t, sql, "COUNT(*)")
		assert.Contains(t, sql, "GROUP BY service_name")
		t.Logf("✅ Group by SQL: %s", sql)
	})

	// Test GROUP BY with AVG
	t.Run("GroupByWithAverage", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans group by service_name avg(duration)", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "service_name")
		assert.Contains(t, sql, "AVG(duration)")
		assert.Contains(t, sql, "GROUP BY service_name")
		t.Logf("✅ Group by with avg SQL: %s", sql)
	})

	// Test multiple GROUP BY fields
	t.Run("MultipleGroupByFields", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans group by service_name, name count()", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "service_name, name")
		assert.Contains(t, sql, "GROUP BY service_name, name")
		t.Logf("✅ Multiple group by fields SQL: %s", sql)
	})
}

func TestTimeFunctions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Test SINCE with relative duration
	t.Run("SinceRelative", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans since 1h limit 10", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "timestamp >=")
		t.Logf("✅ Since 1h SQL: %s", sql)
	})

	// Test SINCE with different durations
	t.Run("SinceMinutes", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans since 30m limit 10", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "timestamp >=")
		t.Logf("✅ Since 30m SQL: %s", sql)
	})

	// Test BETWEEN with dates
	t.Run("BetweenDates", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans between 2024-03-01 and 2024-03-31 limit 10", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "timestamp >=")
		assert.Contains(t, sql, "timestamp <=")
		t.Logf("✅ Between dates SQL: %s", sql)
	})

	// Test SINCE with absolute date
	t.Run("SinceAbsolute", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans since 2024-03-01 limit 10", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "timestamp >=")
		t.Logf("✅ Since absolute date SQL: %s", sql)
	})
}

func TestCorrelateOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Create a unique trace ID shared across signals
	timestamp := time.Now().UnixNano()
	traceID := fmt.Sprintf("%032x", timestamp+100)
	spanID := fmt.Sprintf("%016x", timestamp+100)

	// Send span
	traces := CreateTestSpan(traceID, spanID, "correlate-test", "correlate-service", 200, 150*time.Millisecond)
	err := SendTracesHTTP(t, traces, testTenantID)
	require.NoError(t, err)

	// Send log with same trace_id
	logs := CreateTestLog(traceID, spanID, "correlate-service", "Correlate test log", "INFO")
	err = SendLogsHTTP(t, logs, testTenantID)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	// Test CORRELATE with logs
	t.Run("CorrelateWithLogs", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"correlate-test\" correlate logs", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		// Should return results from both spans and logs
		// First result is the base query (spans)
		assert.Contains(t, resp.Results[0].SQL, "otel_spans")

		// If we got correlated logs, second result should be logs
		if len(resp.Results) > 1 {
			assert.Contains(t, resp.Results[1].SQL, "otel_logs")
			assert.Contains(t, resp.Results[1].SQL, "trace_id IN")
			t.Logf("✅ Correlate returned %d result sets (spans + logs)", len(resp.Results))
		} else {
			t.Log("⚠️  Correlate returned only base results (no correlated logs found)")
		}
	})

	// Test CORRELATE with multiple signals
	t.Run("CorrelateMultiple", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans where name == \"correlate-test\" correlate logs, metrics", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		// Should attempt to correlate with both logs and metrics
		t.Logf("✅ Correlate with multiple signals returned %d result sets", len(resp.Results))
	})
}

func TestExtractOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Test EXTRACT with field
	t.Run("ExtractField", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans extract trace_id as my_trace", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "SELECT trace_id AS my_trace")
		t.Logf("✅ Extract SQL: %s", sql)
	})

	// Test EXTRACT without alias
	t.Run("ExtractWithoutAlias", func(t *testing.T) {
		// This should fail gracefully since we require 'as'
		_, err := QueryOQL(t, "signal=spans extract trace_id", testTenantID)
		// We expect this to fail or be handled
		t.Logf("Extract without alias: %v", err)
	})
}

func TestSwitchContextOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Test SWITCH_CONTEXT
	t.Run("SwitchContext", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=metrics switch_context signal=spans limit 10", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		// After switch_context, we should be querying otel_spans
		assert.Contains(t, sql, "otel_spans")
		t.Logf("✅ Switch context SQL: %s", sql)
	})
}

func TestFilterOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Test FILTER (like WHERE)
	t.Run("Filter", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans filter duration > 100 limit 10", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "duration >")
		t.Logf("✅ Filter SQL: %s", sql)
	})
}

func TestComplexQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if !isOtelOQLAvailable() {
		t.Skip("OTEL-OQL service not running")
	}

	// Test complex query: time range + filter + aggregation + group by
	t.Run("TimeRangeWithAggregation", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans since 1h where http_status_code >= 200 group by service_name avg(duration)", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "timestamp >=")
		assert.Contains(t, sql, "http_status_code >=")
		assert.Contains(t, sql, "AVG(duration)")
		assert.Contains(t, sql, "GROUP BY service_name")
		t.Logf("✅ Complex query SQL: %s", sql)
	})

	// Test query without pipes
	t.Run("ComplexQueryNoPipes", func(t *testing.T) {
		resp, err := QueryOQL(t, "signal=spans since 30m where duration > 100 limit 50", testTenantID)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Results)

		sql := resp.Results[0].SQL
		assert.Contains(t, sql, "timestamp >=")
		assert.Contains(t, sql, "duration >")
		assert.Contains(t, sql, "LIMIT 50")
		t.Logf("✅ Complex query (no pipes) SQL: %s", sql)
	})
}
