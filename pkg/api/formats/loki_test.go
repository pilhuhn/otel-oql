package formats

import (
	"testing"
)

func TestTransformToLokiStreams_EmptyResults(t *testing.T) {
	// Test that empty result array returns success with empty results
	response := TransformToLokiStreams([]PinotResult{}, 100, "backward")

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Data.ResultType != "streams" {
		t.Errorf("Expected resultType 'streams', got '%s'", response.Data.ResultType)
	}

	if len(response.Data.Result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(response.Data.Result))
	}
}

func TestTransformToLokiStreams_EmptyRows(t *testing.T) {
	// Test that query with 0 rows returns success with empty results (not an error)
	results := []PinotResult{
		{
			SQL:     "SELECT * FROM otel_logs WHERE job = 'varlogs'",
			Columns: []string{"body", "timestamp", "job"},
			Rows:    [][]interface{}{}, // No rows
		},
	}

	response := TransformToLokiStreams(results, 100, "backward")

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Error != "" {
		t.Errorf("Expected no error, got error: %s", response.Error)
	}

	if response.Data.ResultType != "streams" {
		t.Errorf("Expected resultType 'streams', got '%s'", response.Data.ResultType)
	}

	if len(response.Data.Result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(response.Data.Result))
	}
}

func TestTransformToLokiStreams_WithData(t *testing.T) {
	// Test normal case with actual data
	results := []PinotResult{
		{
			SQL:     "SELECT * FROM otel_logs WHERE job = 'varlogs'",
			Columns: []string{"body", "timestamp", "job", "level"},
			Rows: [][]interface{}{
				{"error message 1", int64(1710928800000000000), "varlogs", "error"},
				{"error message 2", int64(1710928801000000000), "varlogs", "error"},
			},
		},
	}

	response := TransformToLokiStreams(results, 100, "backward")

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if len(response.Data.Result) == 0 {
		t.Fatalf("Expected at least 1 stream, got 0")
	}

	// Check that we have log entries
	hasEntries := false
	for _, stream := range response.Data.Result {
		if len(stream.Values) > 0 {
			hasEntries = true
			break
		}
	}

	if !hasEntries {
		t.Errorf("Expected log entries in streams")
	}
}

func TestTransformToLokiMatrix_EmptyResults(t *testing.T) {
	// Test that empty result array returns success with empty results
	response := TransformToLokiMatrix([]PinotResult{})

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.Data.ResultType != "matrix" {
		t.Errorf("Expected resultType 'matrix', got '%s'", response.Data.ResultType)
	}

	if len(response.Data.Result) != 0 {
		t.Errorf("Expected 0 results, got %d", len(response.Data.Result))
	}
}

func TestTransformToLokiMatrix_EmptyRows(t *testing.T) {
	// Test that metric query with 0 rows returns success with empty results
	results := []PinotResult{
		{
			SQL:     "SELECT COUNT(*) FROM otel_logs WHERE job = 'varlogs'",
			Columns: []string{"cnt", "timestamp"},
			Rows:    [][]interface{}{}, // No rows
		},
	}

	response := TransformToLokiMatrix(results)

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

func TestLokiError(t *testing.T) {
	response := LokiError("bad_data", "test error message")

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

func TestToNanoseconds(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int64
	}{
		{
			name:  "nanoseconds",
			input: int64(1710928800000000000),
			want:  int64(1710928800000000000),
		},
		{
			name:  "milliseconds",
			input: int64(1710928800000),
			want:  int64(1710928800000000000),
		},
		{
			name:  "seconds",
			input: int64(1710928800),
			want:  int64(1710928800000000000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toNanoseconds(tt.input)
			if got != tt.want {
				t.Errorf("toNanoseconds() = %v, want %v", got, tt.want)
			}
		})
	}
}
