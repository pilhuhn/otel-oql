package formats

import (
	"testing"
	"time"
)

func TestTransformToPrometheusInstant_EmptyResults(t *testing.T) {
	// Test that empty result array returns success with empty results
	response := TransformToPrometheusInstant([]PinotResult{}, time.Now())

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Data.ResultType != "vector" {
		t.Errorf("Expected resultType 'vector', got '%s'", response.Data.ResultType)
	}

	if len(response.Data.Result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(response.Data.Result))
	}
}

func TestTransformToPrometheusInstant_EmptyRows(t *testing.T) {
	// Test that query with 0 rows returns success with empty results (not an error)
	results := []PinotResult{
		{
			SQL:     "SELECT * FROM otel_metrics WHERE metric_name = 'jvm_memory_used'",
			Columns: []string{"metric_name", "value", "timestamp"},
			Rows:    [][]interface{}{}, // No rows
		},
	}

	response := TransformToPrometheusInstant(results, time.Now())

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Error != "" {
		t.Errorf("Expected no error, got error: %s", response.Error)
	}

	if response.Data.ResultType != "vector" {
		t.Errorf("Expected resultType 'vector', got '%s'", response.Data.ResultType)
	}

	if len(response.Data.Result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(response.Data.Result))
	}
}

func TestTransformToPrometheusInstant_WithData(t *testing.T) {
	// Test normal case with actual data
	results := []PinotResult{
		{
			SQL:     "SELECT * FROM otel_metrics WHERE metric_name = 'http_requests_total'",
			Columns: []string{"metric_name", "value", "timestamp", "job"},
			Rows: [][]interface{}{
				{"http_requests_total", 1234.0, int64(1710928800000), "api"},
			},
		},
	}

	response := TransformToPrometheusInstant(results, time.Unix(1710928800, 0))

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if len(response.Data.Result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(response.Data.Result))
	}

	result := response.Data.Result[0]
	if result.Metric["__name__"] != "http_requests_total" {
		t.Errorf("Expected metric name 'http_requests_total', got '%s'", result.Metric["__name__"])
	}

	if result.Metric["job"] != "api" {
		t.Errorf("Expected job 'api', got '%s'", result.Metric["job"])
	}
}

func TestTransformToPrometheusRange_EmptyRows(t *testing.T) {
	// Test that range query with 0 rows returns success with empty results
	results := []PinotResult{
		{
			SQL:     "SELECT * FROM otel_metrics WHERE metric_name = 'jvm_memory_used'",
			Columns: []string{"metric_name", "value", "timestamp"},
			Rows:    [][]interface{}{}, // No rows
		},
	}

	response := TransformToPrometheusRange(results)

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Error != "" {
		t.Errorf("Expected no error, got error: %s", response.Error)
	}

	if response.Data.ResultType != "matrix" {
		t.Errorf("Expected resultType 'matrix', got '%s'", response.Data.ResultType)
	}

	if len(response.Data.Result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(response.Data.Result))
	}
}

func TestPrometheusError(t *testing.T) {
	response := PrometheusError("bad_data", "test error message")

	if response.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", response.Status)
	}

	if response.ErrorType != "bad_data" {
		t.Errorf("Expected errorType 'bad_data', got '%s'", response.ErrorType)
	}

	if response.Error != "test error message" {
		t.Errorf("Expected error 'test error message', got '%s'", response.Error)
	}
}
