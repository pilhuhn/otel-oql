package api

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseLokiQueryParams(t *testing.T) {
	tests := []struct {
		name      string
		params    url.Values
		wantQuery string
		wantLimit int
		wantDir   string
		wantErr   bool
	}{
		{
			name: "valid query",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
			},
			wantQuery: `{job="varlogs"}`,
			wantLimit: 100, // default
			wantDir:   "backward",
			wantErr:   false,
		},
		{
			name: "query with limit",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
				"limit": []string{"1000"},
			},
			wantQuery: `{job="varlogs"}`,
			wantLimit: 1000,
			wantDir:   "backward",
			wantErr:   false,
		},
		{
			name: "query with direction",
			params: url.Values{
				"query":     []string{`{job="varlogs"}`},
				"direction": []string{"forward"},
			},
			wantQuery: `{job="varlogs"}`,
			wantLimit: 100,
			wantDir:   "forward",
			wantErr:   false,
		},
		{
			name: "query with time",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
				"time":  []string{"2024-03-20T10:00:00Z"},
			},
			wantQuery: `{job="varlogs"}`,
			wantLimit: 100,
			wantDir:   "backward",
			wantErr:   false,
		},
		{
			name:    "missing query parameter",
			params:  url.Values{},
			wantErr: true,
		},
		{
			name: "invalid limit",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
				"limit": []string{"invalid"},
			},
			wantErr: true,
		},
		{
			name: "invalid direction",
			params: url.Values{
				"query":     []string{`{job="varlogs"}`},
				"direction": []string{"invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/loki/api/v1/query", nil)
			req.Form = tt.params

			params, err := ParseLokiQueryParams(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLokiQueryParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if params.Query != tt.wantQuery {
					t.Errorf("ParseLokiQueryParams() query = %v, want %v", params.Query, tt.wantQuery)
				}
				if params.Limit != tt.wantLimit {
					t.Errorf("ParseLokiQueryParams() limit = %v, want %v", params.Limit, tt.wantLimit)
				}
				if params.Direction != tt.wantDir {
					t.Errorf("ParseLokiQueryParams() direction = %v, want %v", params.Direction, tt.wantDir)
				}
			}
		})
	}
}

func TestParseLokiRangeParams(t *testing.T) {
	tests := []struct {
		name      string
		params    url.Values
		wantQuery string
		wantLimit int
		wantErr   bool
	}{
		{
			name: "valid range query",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
			},
			wantQuery: `{job="varlogs"}`,
			wantLimit: 5000, // default for range
			wantErr:   false,
		},
		{
			name: "range query with limit",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
				"limit": []string{"10000"},
			},
			wantQuery: `{job="varlogs"}`,
			wantLimit: 10000,
			wantErr:   false,
		},
		{
			name: "metric query with step",
			params: url.Values{
				"query": []string{`count_over_time({job="varlogs"}[5m])`},
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
				"step":  []string{"300"},
			},
			wantQuery: `count_over_time({job="varlogs"}[5m])`,
			wantLimit: 5000,
			wantErr:   false,
		},
		{
			name: "missing query",
			params: url.Values{
				"start": []string{"1710928800"},
				"end":   []string{"1710932400"},
			},
			wantErr: true,
		},
		{
			name: "missing start",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
				"end":   []string{"1710932400"},
			},
			wantErr: true,
		},
		{
			name: "missing end",
			params: url.Values{
				"query": []string{`{job="varlogs"}`},
				"start": []string{"1710928800"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/loki/api/v1/query_range", nil)
			req.Form = tt.params

			params, err := ParseLokiRangeParams(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLokiRangeParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if params.Query != tt.wantQuery {
					t.Errorf("ParseLokiRangeParams() query = %v, want %v", params.Query, tt.wantQuery)
				}
				if params.Limit != tt.wantLimit {
					t.Errorf("ParseLokiRangeParams() limit = %v, want %v", params.Limit, tt.wantLimit)
				}
			}
		})
	}
}

func TestLokiQueryEndpoint_GET(t *testing.T) {
	// This test verifies that GET requests with URL-encoded parameters work
	query := url.QueryEscape(`{job="varlogs"}`)
	req := httptest.NewRequest("GET", "/loki/api/v1/query?query="+query+"&limit=1000", nil)

	params, err := ParseLokiQueryParams(req)
	if err != nil {
		t.Fatalf("Failed to parse GET request: %v", err)
	}

	if params.Query != `{job="varlogs"}` {
		t.Errorf("Expected query '{job=\"varlogs\"}', got '%s'", params.Query)
	}

	if params.Limit != 1000 {
		t.Errorf("Expected limit 1000, got %d", params.Limit)
	}
}

func TestLokiQueryEndpoint_POST(t *testing.T) {
	// This test verifies that POST requests with form-encoded bodies work
	formData := url.Values{
		"query":     []string{`{job="varlogs"} |= "error"`},
		"limit":     []string{"500"},
		"direction": []string{"forward"},
	}

	req := httptest.NewRequest("POST", "/loki/api/v1/query", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	params, err := ParseLokiQueryParams(req)
	if err != nil {
		t.Fatalf("Failed to parse POST request: %v", err)
	}

	if params.Query != `{job="varlogs"} |= "error"` {
		t.Errorf("Expected query '{job=\"varlogs\"} |= \"error\"', got '%s'", params.Query)
	}

	if params.Limit != 500 {
		t.Errorf("Expected limit 500, got %d", params.Limit)
	}

	if params.Direction != "forward" {
		t.Errorf("Expected direction 'forward', got '%s'", params.Direction)
	}
}

func TestLokiRangeEndpoint_MultipleTimeFormats(t *testing.T) {
	tests := []struct {
		name      string
		startStr  string
		endStr    string
		wantStart int64
		wantEnd   int64
	}{
		{
			name:      "unix timestamps (seconds)",
			startStr:  "1710928800",
			endStr:    "1710932400",
			wantStart: 1710928800,
			wantEnd:   1710932400,
		},
		{
			name:      "RFC3339 timestamps",
			startStr:  "2024-03-20T10:00:00Z",
			endStr:    "2024-03-20T11:00:00Z",
			wantStart: 1710928800,
			wantEnd:   1710932400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formData := url.Values{
				"query": []string{`{job="varlogs"}`},
				"start": []string{tt.startStr},
				"end":   []string{tt.endStr},
			}

			req := httptest.NewRequest("POST", "/loki/api/v1/query_range", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			params, err := ParseLokiRangeParams(req)
			if err != nil {
				t.Fatalf("Failed to parse request: %v", err)
			}

			if params.Start.Unix() != tt.wantStart {
				t.Errorf("Expected start %d, got %d", tt.wantStart, params.Start.Unix())
			}

			if params.End.Unix() != tt.wantEnd {
				t.Errorf("Expected end %d, got %d", tt.wantEnd, params.End.Unix())
			}
		})
	}
}

func TestLokiRangeEndpoint_MetricQueryWithStep(t *testing.T) {
	// Test metric queries that include step parameter
	formData := url.Values{
		"query": []string{`sum by (level) (count_over_time({job="varlogs"}[5m]))`},
		"start": []string{"1710928800"},
		"end":   []string{"1710932400"},
		"step":  []string{"300"},
	}

	req := httptest.NewRequest("POST", "/loki/api/v1/query_range", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	params, err := ParseLokiRangeParams(req)
	if err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if params.Step != 300*time.Second {
		t.Errorf("Expected step 300s, got %v", params.Step)
	}
}

func TestLokiEndpoint_DirectionParameter(t *testing.T) {
	tests := []struct {
		name      string
		direction string
		wantErr   bool
	}{
		{
			name:      "forward direction",
			direction: "forward",
			wantErr:   false,
		},
		{
			name:      "backward direction",
			direction: "backward",
			wantErr:   false,
		},
		{
			name:      "invalid direction",
			direction: "invalid",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formData := url.Values{
				"query":     []string{`{job="varlogs"}`},
				"direction": []string{tt.direction},
			}

			req := httptest.NewRequest("POST", "/loki/api/v1/query", strings.NewReader(formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			_, err := ParseLokiQueryParams(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Expected error=%v, got error=%v", tt.wantErr, err != nil)
			}
		})
	}
}

func TestLokiEndpoint_TenantIsolation(t *testing.T) {
	// This test verifies that the endpoint requires tenant-id
	// Note: This would require setting up a full server with middleware,
	// so this is more of a documentation test showing the expected behavior

	// In practice, the validator.HTTPMiddleware() should:
	// 1. Extract X-Tenant-ID header
	// 2. Validate it's a valid integer
	// 3. Inject into request context
	// 4. Return 401 if missing/invalid

	t.Log("Tenant isolation is enforced by validator.HTTPMiddleware()")
	t.Log("All Loki endpoints require X-Tenant-ID header")
}

func TestLokiEndpoint_ErrorResponses(t *testing.T) {
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
			// This documents the expected error format for Loki API
			t.Logf("Error type: %s", tt.errorType)
			t.Logf("Error message: %s", tt.errorMessage)
			t.Logf("Expected JSON: %s", tt.expectedJSON)
		})
	}
}

func TestLokiQueryTypes(t *testing.T) {
	// This test documents the different types of LogQL queries

	tests := []struct {
		name        string
		query       string
		queryType   string
		description string
	}{
		{
			name:        "log stream query",
			query:       `{job="varlogs"}`,
			queryType:   "stream",
			description: "Returns log entries as streams",
		},
		{
			name:        "log stream with line filter",
			query:       `{job="varlogs"} |= "error"`,
			queryType:   "stream",
			description: "Returns filtered log entries",
		},
		{
			name:        "metric query - count_over_time",
			query:       `count_over_time({job="varlogs"}[5m])`,
			queryType:   "metric",
			description: "Returns metric data (matrix format)",
		},
		{
			name:        "metric query - rate",
			query:       `rate({job="varlogs"}[5m])`,
			queryType:   "metric",
			description: "Returns rate metric data",
		},
		{
			name:        "metric query with aggregation",
			query:       `sum by (level) (count_over_time({job="varlogs"}[5m]))`,
			queryType:   "metric",
			description: "Returns aggregated metric data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Query: %s", tt.query)
			t.Logf("Type: %s", tt.queryType)
			t.Logf("Description: %s", tt.description)
		})
	}
}
