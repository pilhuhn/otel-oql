package api

import (
	"context"
	"strings"
	"testing"
)

// TestLogQLTraceCorrelation tests that trace_id and span_id are mapped to native columns
// This is critical for efficient log-to-trace correlation
func TestLogQLTraceCorrelation(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	tenantID := 0

	tests := []struct {
		name         string
		logql        string
		wantContains []string
		description  string
	}{
		{
			name:  "filter by trace_id",
			logql: `{trace_id="abc123"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND trace_id = 'abc123'",
			},
			description: "trace_id should use native column, not JSON extraction",
		},
		{
			name:  "filter by traceId (camelCase)",
			logql: `{traceId="abc123"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND trace_id = 'abc123'",
			},
			description: "traceId should map to trace_id native column",
		},
		{
			name:  "filter by span_id",
			logql: `{span_id="def456"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND span_id = 'def456'",
			},
			description: "span_id should use native column",
		},
		{
			name:  "filter by spanId (camelCase)",
			logql: `{spanId="def456"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND span_id = 'def456'",
			},
			description: "spanId should map to span_id native column",
		},
		{
			name:  "filter by trace_id and service",
			logql: `{trace_id="abc123", service="api"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND trace_id = 'abc123'",
				"AND service_name = 'api'",
			},
			description: "combine trace correlation with service filtering",
		},
		{
			name:  "find all logs for a trace with errors",
			logql: `{trace_id="abc123"} |= "error"`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND trace_id = 'abc123'",
				"AND body LIKE '%error%'",
			},
			description: "correlate trace with error logs",
		},
		{
			name:  "count logs per trace_id",
			logql: `sum by (trace_id) (count_over_time({service="api"}[1h]))`,
			wantContains: []string{
				"SELECT trace_id, COUNT(*) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND service_name = 'api'",
				"AND timestamp >= (now() - 3600000)",
				"GROUP BY trace_id",
			},
			description: "aggregate logs by trace_id",
		},
		{
			name:  "regex trace_id search",
			logql: `{trace_id=~"abc.*"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND REGEXP_LIKE(trace_id, 'abc.*')",
			},
			description: "regex matching on trace_id uses native column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := s.executeLogQLQuery(ctx, tt.logql, tenantID)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(queries) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(queries))
				return
			}

			sql := queries[0]

			// Verify it does NOT use JSON extraction for trace/span IDs
			if strings.Contains(sql, "JSON_EXTRACT_SCALAR(attributes, '$.trace_id'") {
				t.Errorf("trace_id should use native column, not JSON extraction:\n%s", sql)
			}
			if strings.Contains(sql, "JSON_EXTRACT_SCALAR(attributes, '$.span_id'") {
				t.Errorf("span_id should use native column, not JSON extraction:\n%s", sql)
			}

			// Verify expected SQL fragments
			for _, want := range tt.wantContains {
				if !strings.Contains(sql, want) {
					t.Errorf("%s\nSQL missing expected substring:\nwant: %s\ngot:  %s",
						tt.description, want, sql)
				}
			}
		})
	}
}

// TestLogQLNativeColumnMapping tests that all native columns are properly mapped
func TestLogQLNativeColumnMapping(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	tenantID := 0

	tests := []struct {
		name            string
		logql           string
		wantNativeCol   string
		wantNotJSONPath string
	}{
		{
			name:            "job",
			logql:           `{job="varlogs"}`,
			wantNativeCol:   "job = 'varlogs'",
			wantNotJSONPath: "$.job",
		},
		{
			name:            "instance",
			logql:           `{instance="pod-1"}`,
			wantNativeCol:   "instance = 'pod-1'",
			wantNotJSONPath: "$.instance",
		},
		{
			name:            "environment",
			logql:           `{environment="production"}`,
			wantNativeCol:   "environment = 'production'",
			wantNotJSONPath: "$.environment",
		},
		{
			name:            "level",
			logql:           `{level="error"}`,
			wantNativeCol:   "log_level = 'error'",
			wantNotJSONPath: "$.level",
		},
		{
			name:            "severity",
			logql:           `{severity="WARN"}`,
			wantNativeCol:   "severity_text = 'WARN'",
			wantNotJSONPath: "$.severity",
		},
		{
			name:            "service",
			logql:           `{service="api"}`,
			wantNativeCol:   "service_name = 'api'",
			wantNotJSONPath: "$.service",
		},
		{
			name:            "host",
			logql:           `{host="web-01"}`,
			wantNativeCol:   "host_name = 'web-01'",
			wantNotJSONPath: "$.host",
		},
		{
			name:            "source",
			logql:           `{source="/var/log/app.log"}`,
			wantNativeCol:   "log_source = '/var/log/app.log'",
			wantNotJSONPath: "$.source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := s.executeLogQLQuery(ctx, tt.logql, tenantID)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(queries) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(queries))
				return
			}

			sql := queries[0]

			// Verify it uses native column
			if !strings.Contains(sql, tt.wantNativeCol) {
				t.Errorf("expected native column usage:\nwant: %s\ngot:  %s",
					tt.wantNativeCol, sql)
			}

			// Verify it does NOT use JSON extraction
			if strings.Contains(sql, tt.wantNotJSONPath) {
				t.Errorf("should use native column, not JSON extraction for %s:\n%s",
					tt.name, sql)
			}
		})
	}
}

// TestLogQLCustomAttributeUsesJSON tests that non-native labels use JSON extraction
func TestLogQLCustomAttributeUsesJSON(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	tenantID := 0

	tests := []struct {
		name     string
		logql    string
		wantJSON string
	}{
		{
			name:     "custom label",
			logql:    `{custom_label="value"}`,
			wantJSON: "JSON_EXTRACT_SCALAR(attributes, '$.custom_label', 'STRING') = 'value'",
		},
		{
			name:     "application label",
			logql:    `{app="myapp"}`,
			wantJSON: "JSON_EXTRACT_SCALAR(attributes, '$.app', 'STRING') = 'myapp'",
		},
		{
			name:     "version label",
			logql:    `{version="1.2.3"}`,
			wantJSON: "JSON_EXTRACT_SCALAR(attributes, '$.version', 'STRING') = '1.2.3'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := s.executeLogQLQuery(ctx, tt.logql, tenantID)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(queries) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(queries))
				return
			}

			sql := queries[0]

			// Verify it uses JSON extraction
			if !strings.Contains(sql, tt.wantJSON) {
				t.Errorf("custom label should use JSON extraction:\nwant: %s\ngot:  %s",
					tt.wantJSON, sql)
			}
		})
	}
}
