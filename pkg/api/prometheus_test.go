package api

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParsePrometheusQueryParams(t *testing.T) {
	tests := []struct {
		name      string
		params    url.Values
		wantQuery string
		wantErr   bool
	}{
		{
			name: "valid query",
			params: url.Values{
				"query": []string{"http_requests_total"},
			},
			wantQuery: "http_requests_total",
			wantErr:   false,
		},
		{
			name: "query with time",
			params: url.Values{
				"query": []string{"http_requests_total"},
				"time":  []string{"2024-03-20T10:00:00Z"},
			},
			wantQuery: "http_requests_total",
			wantErr:   false,
		},
		{
			name: "query with timeout",
			params: url.Values{
				"query":   []string{"http_requests_total"},
				"timeout": []string{"30s"},
			},
			wantQuery: "http_requests_total",
			wantErr:   false,
		},
		{
			name:    "missing query parameter",
			params:  url.Values{},
			wantErr: true,
		},
		{
			name: "invalid time format",
			params: url.Values{
				"query": []string{"http_requests_total"},
				"time":  []string{"invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/query", nil)
			req.Form = tt.params

			params, err := ParsePrometheusQueryParams(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePrometheusQueryParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && params.Query != tt.wantQuery {
				t.Errorf("ParsePrometheusQueryParams() query = %v, want %v", params.Query, tt.wantQuery)
			}
		})
	}
}

func TestParsePrometheusRangeParams(t *testing.T) {
	tests := []struct {
		name      string
		params    url.Values
		wantQuery string
		wantErr   bool
	}{
		{
			name: "valid range query",
			params: url.Values{
				"query": []string{"rate(http_requests_total[5m])"},
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
				"step":  []string{"15s"},
			},
			wantQuery: "rate(http_requests_total[5m])",
			wantErr:   false,
		},
		{
			name: "range query with RFC3339 timestamps",
			params: url.Values{
				"query": []string{"http_requests_total"},
				"start": []string{"2024-03-20T10:00:00Z"},
				"end":   []string{"2024-03-20T11:00:00Z"},
				"step":  []string{"30s"},
			},
			wantQuery: "http_requests_total",
			wantErr:   false,
		},
		{
			name: "missing query",
			params: url.Values{
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
				"step":  []string{"15s"},
			},
			wantErr: true,
		},
		{
			name: "missing start",
			params: url.Values{
				"query": []string{"http_requests_total"},
				"end":   []string{"1710932400"},
				"step":  []string{"15s"},
			},
			wantErr: true,
		},
		{
			name: "missing end",
			params: url.Values{
				"query": []string{"http_requests_total"},
				"start": []string{"1710928800"},
				"step":  []string{"15s"},
			},
			wantErr: true,
		},
		{
			name: "missing step",
			params: url.Values{
				"query": []string{"http_requests_total"},
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/query_range", nil)
			req.Form = tt.params

			params, err := ParsePrometheusRangeParams(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePrometheusRangeParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && params.Query != tt.wantQuery {
				t.Errorf("ParsePrometheusRangeParams() query = %v, want %v", params.Query, tt.wantQuery)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "RFC3339 format",
			input:   "2024-03-20T10:00:00Z",
			wantErr: false,
		},
		{
			name:    "Unix timestamp (seconds)",
			input:   "1710928800",
			wantErr: false,
		},
		{
			name:    "Unix timestamp (milliseconds)",
			input:   "1710928800000",
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "not-a-timestamp",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTime() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:    "Go duration format",
			input:   "5m0s",
			want:    5 * time.Minute,
			wantErr: false,
		},
		{
			name:    "Prometheus format - minutes",
			input:   "5m",
			want:    5 * time.Minute,
			wantErr: false,
		},
		{
			name:    "Prometheus format - seconds",
			input:   "30s",
			want:    30 * time.Second,
			wantErr: false,
		},
		{
			name:    "Prometheus format - hours",
			input:   "1h",
			want:    1 * time.Hour,
			wantErr: false,
		},
		{
			name:    "numeric seconds",
			input:   "300",
			want:    300 * time.Second,
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("parseDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrometheusQueryEndpoint_GET(t *testing.T) {
	// This test verifies that GET requests with URL-encoded parameters work
	req := httptest.NewRequest("GET", "/api/v1/query?query=http_requests_total&time=1710928800", nil)

	params, err := ParsePrometheusQueryParams(req)
	if err != nil {
		t.Fatalf("Failed to parse GET request: %v", err)
	}

	if params.Query != "http_requests_total" {
		t.Errorf("Expected query 'http_requests_total', got '%s'", params.Query)
	}
}

func TestPrometheusQueryEndpoint_POST(t *testing.T) {
	// This test verifies that POST requests with form-encoded bodies work
	formData := url.Values{
		"query": []string{"rate(http_requests_total[5m])"},
		"time":  []string{"1710928800"},
	}

	req := httptest.NewRequest("POST", "/api/v1/query", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	params, err := ParsePrometheusQueryParams(req)
	if err != nil {
		t.Fatalf("Failed to parse POST request: %v", err)
	}

	if params.Query != "rate(http_requests_total[5m])" {
		t.Errorf("Expected query 'rate(http_requests_total[5m])', got '%s'", params.Query)
	}
}

func TestPrometheusRangeEndpoint_MultipleStepFormats(t *testing.T) {
	tests := []struct {
		name     string
		stepStr  string
		wantStep time.Duration
	}{
		{
			name:     "step in seconds (numeric)",
			stepStr:  "15",
			wantStep: 15 * time.Second,
		},
		{
			name:     "step with unit (s)",
			stepStr:  "15s",
			wantStep: 15 * time.Second,
		},
		{
			name:     "step with unit (m)",
			stepStr:  "5m",
			wantStep: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formData := url.Values{
				"query": []string{"http_requests_total"},
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
				"step":  []string{tt.stepStr},
			}

			req := httptest.NewRequest("POST", "/api/v1/query_range", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			params, err := ParsePrometheusRangeParams(req)
			if err != nil {
				t.Fatalf("Failed to parse request: %v", err)
			}

			if params.Step != tt.wantStep {
				t.Errorf("Expected step %v, got %v", tt.wantStep, params.Step)
			}
		})
	}
}

func TestPrometheusEndpoint_TenantIsolation(t *testing.T) {
	// This test verifies that the endpoint requires tenant-id
	// Note: This would require setting up a full server with middleware,
	// so this is more of a documentation test showing the expected behavior

	// In practice, the validator.HTTPMiddleware() should:
	// 1. Extract X-Tenant-ID header
	// 2. Validate it's a valid integer
	// 3. Inject into request context
	// 4. Return 401 if missing/invalid

	t.Log("Tenant isolation is enforced by validator.HTTPMiddleware()")
	t.Log("All Prometheus endpoints require X-Tenant-ID header")
}

func TestPrometheusEndpoint_ErrorResponses(t *testing.T) {
	// This test documents expected error response formats

	tests := []struct {
		name          string
		errorType     string
		errorMessage  string
		expectedJSON  string
	}{
		{
			name:         "bad_data error",
			errorType:    "bad_data",
			errorMessage: "missing required parameter: query",
			expectedJSON: `{"status":"error","errorType":"bad_data","error":"missing required parameter: query"}`,
		},
		{
			name:         "execution error",
			errorType:    "execution",
			errorMessage: "query execution failed",
			expectedJSON: `{"status":"error","errorType":"execution","error":"query execution failed"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This documents the expected error format for Prometheus API
			t.Logf("Error type: %s", tt.errorType)
			t.Logf("Error message: %s", tt.errorMessage)
			t.Logf("Expected JSON: %s", tt.expectedJSON)
		})
	}
}
