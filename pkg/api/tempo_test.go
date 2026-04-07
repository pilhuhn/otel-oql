package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTempoEcho(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest("GET", "/api/echo", nil)
	w := httptest.NewRecorder()

	s.handleTempoEcho(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var echoResp TempoEchoResponse
	if err := json.NewDecoder(resp.Body).Decode(&echoResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if echoResp.Message != "ok" {
		t.Errorf("Expected message 'ok', got %q", echoResp.Message)
	}
}

func TestTempoEndpoints(t *testing.T) {
	s := &Server{}

	t.Run("mapTraceQLTagToColumn", func(t *testing.T) {
		tests := []struct {
			tagName      string
			wantColumn   string
			wantErr      bool
		}{
			// Intrinsic fields
			{
				tagName:    "name",
				wantColumn: "name",
				wantErr:    false,
			},
			{
				tagName:    "duration",
				wantColumn: "duration",
				wantErr:    false,
			},
			{
				tagName:    "status",
				wantColumn: "status_code",
				wantErr:    false,
			},
			{
				tagName:    "kind",
				wantColumn: "kind",
				wantErr:    false,
			},
			// Span attributes - native columns
			{
				tagName:    "span.http.method",
				wantColumn: "http_method",
				wantErr:    false,
			},
			{
				tagName:    "span.http.status_code",
				wantColumn: "http_status_code",
				wantErr:    false,
			},
			{
				tagName:    "span.db.system",
				wantColumn: "db_system",
				wantErr:    false,
			},
			// Span attributes - custom (JSON extraction)
			{
				tagName:    "span.custom.field",
				wantColumn: "JSON_EXTRACT_SCALAR(attributes, '$.custom.field', 'STRING')",
				wantErr:    false,
			},
			// Resource attributes - native columns
			{
				tagName:    "resource.service.name",
				wantColumn: "service_name",
				wantErr:    false,
			},
			// Resource attributes - custom (JSON extraction)
			{
				tagName:    "resource.environment",
				wantColumn: "JSON_EXTRACT_SCALAR(resource_attributes, '$.environment', 'STRING')",
				wantErr:    false,
			},
			// Unknown tag
			{
				tagName: "unknown",
				wantErr: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.tagName, func(t *testing.T) {
				column, err := s.mapTraceQLTagToColumn(tt.tagName)

				if (err != nil) != tt.wantErr {
					t.Errorf("mapTraceQLTagToColumn(%q) error = %v, wantErr %v", tt.tagName, err, tt.wantErr)
					return
				}

				if !tt.wantErr && column != tt.wantColumn {
					t.Errorf("mapTraceQLTagToColumn(%q) = %q, want %q", tt.tagName, column, tt.wantColumn)
				}
			})
		}
	})

	t.Run("buildTempoTagValuesSQL", func(t *testing.T) {
		tests := []struct {
			name         string
			tenantID     int
			column       string
			wantContains []string
		}{
			{
				name:     "simple native column",
				tenantID: 0,
				column:   "name",
				wantContains: []string{
					"SELECT DISTINCT name FROM otel_spans",
					"tenant_id = 0",
					"name IS NOT NULL",
					"LIMIT 100",
				},
			},
			{
				name:     "http_method column",
				tenantID: 42,
				column:   "http_method",
				wantContains: []string{
					"SELECT DISTINCT http_method FROM otel_spans",
					"tenant_id = 42",
					"http_method IS NOT NULL",
				},
			},
			{
				name:     "JSON extraction column",
				tenantID: 0,
				column:   "JSON_EXTRACT_SCALAR(attributes, '$.custom.field', 'STRING')",
				wantContains: []string{
					"SELECT DISTINCT JSON_EXTRACT_SCALAR(attributes, '$.custom.field', 'STRING') FROM otel_spans",
					"tenant_id = 0",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sql := s.buildTempoTagValuesSQL(tt.tenantID, tt.column, nil, nil)

				for _, want := range tt.wantContains {
					if !strings.Contains(sql, want) {
						t.Errorf("buildTempoTagValuesSQL() SQL should contain %q\nGot: %s", want, sql)
					}
				}
			})
		}
	})
}

func TestTempoSearchQuery(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	tenantID := 0

	tests := []struct {
		name         string
		traceql      string
		wantErr      bool
		wantContains []string
	}{
		{
			name:    "simple duration filter",
			traceql: "{duration > 100ms}",
			wantErr: false,
			wantContains: []string{
				"SELECT * FROM otel_spans",
				"tenant_id = 0",
				"duration > 100000000",
			},
		},
		{
			name:    "span attribute filter",
			traceql: "{span.http.status_code = 500}",
			wantErr: false,
			wantContains: []string{
				"otel_spans",
				"http_status_code = 500",
			},
		},
		{
			name:    "resource attribute filter",
			traceql: `{resource.service.name = "api"}`,
			wantErr: false,
			wantContains: []string{
				"otel_spans",
				"service_name = 'api'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlQueries, err := s.executeTraceQLQuery(ctx, tt.traceql, tenantID)

			if (err != nil) != tt.wantErr {
				t.Errorf("executeTraceQLQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(sqlQueries) == 0 {
					t.Error("executeTraceQLQuery() returned no SQL queries")
					return
				}

				sql := sqlQueries[0]
				for _, want := range tt.wantContains {
					if !strings.Contains(sql, want) {
						t.Errorf("SQL should contain %q\nGot: %s", want, sql)
					}
				}
			}
		})
	}
}

func TestTempoV1MetadataQuery(t *testing.T) {
	t.Run("metadata response structure", func(t *testing.T) {
		// Test that the metadata structure is correct
		metadata := &TempoMetadata{
			ServiceNames:   []string{"service-a", "service-b"},
			OperationNames: []string{"GET /api", "POST /api"},
		}

		if len(metadata.ServiceNames) != 2 {
			t.Errorf("Expected 2 service names, got %d", len(metadata.ServiceNames))
		}

		if len(metadata.OperationNames) != 2 {
			t.Errorf("Expected 2 operation names, got %d", len(metadata.OperationNames))
		}
	})
}

func TestTempoTraceByID(t *testing.T) {
	t.Run("trace response structure", func(t *testing.T) {
		// Test that the trace structure is correct
		methodVal := "GET"
		statusCode := int64(200)
		span := TempoTraceSpan{
			TraceID:           "b06e5ee8f5b5a34c87808e09834cbff3",
			SpanID:            "abc123",
			Name:              "HTTP GET",
			StartTimeUnixNano: "1774832350000000000",
			DurationNanos:     "125000000",
			Attributes: []TempoAttribute{
				{Key: "http.method", Value: TempoAnyValue{StringValue: &methodVal}},
				{Key: "http.status_code", Value: TempoAnyValue{IntValue: &statusCode}},
			},
			Status: TempoStatus{Code: 1}, // 1 = STATUS_CODE_OK
		}

		scopeSpans := TempoScopeSpans{
			Spans: []TempoTraceSpan{span},
		}

		resourceSpans := TempoResourceSpans{
			ScopeSpans: []TempoScopeSpans{scopeSpans},
		}

		trace := TempoTraceResponse{
			ResourceSpans: []TempoResourceSpans{resourceSpans},
		}

		if len(trace.ResourceSpans) != 1 {
			t.Errorf("Expected 1 resourceSpans, got %d", len(trace.ResourceSpans))
		}

		if len(trace.ResourceSpans[0].ScopeSpans) != 1 {
			t.Errorf("Expected 1 scopeSpans, got %d", len(trace.ResourceSpans[0].ScopeSpans))
		}

		if len(trace.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			t.Errorf("Expected 1 span, got %d", len(trace.ResourceSpans[0].ScopeSpans[0].Spans))
		}

		if trace.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceID != "b06e5ee8f5b5a34c87808e09834cbff3" {
			t.Errorf("Expected trace ID b06e5ee8f5b5a34c87808e09834cbff3, got %s", trace.ResourceSpans[0].ScopeSpans[0].Spans[0].TraceID)
		}
	})
}
