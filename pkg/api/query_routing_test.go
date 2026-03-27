package api

import (
	"context"
	"strings"
	"testing"
)

// TestQueryLanguageRouting tests that different query languages are routed correctly
func TestQueryLanguageRouting(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	tenantID := 0

	tests := []struct {
		name       string
		execFunc   func() ([]string, error)
		wantErr    bool
		errContain string
	}{
		{
			name: "OQL query",
			execFunc: func() ([]string, error) {
				return s.executeOQLQuery(ctx, "signal=spans limit 10", tenantID)
			},
			wantErr: false,
		},
		{
			name: "PromQL query - simple",
			execFunc: func() ([]string, error) {
				return s.executePromQLQuery(ctx, "http_requests_total", tenantID)
			},
			wantErr: false,
		},
		{
			name: "PromQL query - with labels",
			execFunc: func() ([]string, error) {
				return s.executePromQLQuery(ctx, "http_requests_total{job=\"api\"}", tenantID)
			},
			wantErr: false,
		},
		{
			name: "PromQL query - aggregation",
			execFunc: func() ([]string, error) {
				return s.executePromQLQuery(ctx, "sum(http_requests_total)", tenantID)
			},
			wantErr: false,
		},
		{
			name: "PromQL query - offset (unsupported)",
			execFunc: func() ([]string, error) {
				return s.executePromQLQuery(ctx, "http_requests_total offset 5m", tenantID)
			},
			wantErr:    true,
			errContain: "offset modifier not supported",
		},
		{
			name: "PromQL query - nested aggregation (unsupported)",
			execFunc: func() ([]string, error) {
				return s.executePromQLQuery(ctx, "sum(avg(http_requests_total))", tenantID)
			},
			wantErr:    true,
			errContain: "nested aggregations not supported",
		},
		{
			name: "PromQL query - invalid syntax",
			execFunc: func() ([]string, error) {
				return s.executePromQLQuery(ctx, "http_requests_total{", tenantID)
			},
			wantErr:    true,
			errContain: "parse error",
		},
		{
			name: "LogQL query (not yet implemented)",
			execFunc: func() ([]string, error) {
				return s.executeLogQLQuery(ctx, "{job=\"varlogs\"}", tenantID)
			},
			wantErr:    true,
			errContain: "not yet implemented",
		},
		{
			name: "TraceQL query (not yet implemented)",
			execFunc: func() ([]string, error) {
				return s.executeTraceQLQuery(ctx, "{ duration > 1s }", tenantID)
			},
			wantErr:    true,
			errContain: "not yet implemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := tt.execFunc()

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("Expected error=%v, got error=%v (message: %v)", tt.wantErr, err != nil, err)
				return
			}

			// Check error message contains expected substring
			if tt.wantErr && tt.errContain != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got no error", tt.errContain)
				} else if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("Expected error to contain %q, got: %s", tt.errContain, err.Error())
				}
			}

			// If successful, verify we got SQL
			if !tt.wantErr {
				if len(queries) == 0 {
					t.Error("Expected at least one SQL query, got none")
				}
				// Verify the SQL is not empty
				for i, sql := range queries {
					if strings.TrimSpace(sql) == "" {
						t.Errorf("Query %d is empty", i)
					}
				}
			}
		})
	}
}

// TestPromQLTranslation verifies that PromQL translates to reasonable SQL
func TestPromQLTranslation(t *testing.T) {
	s := &Server{}
	ctx := context.Background()

	tests := []struct {
		name         string
		promql       string
		tenantID     int
		wantContains []string
	}{
		{
			name:     "simple metric with tenant 0",
			promql:   "up",
			tenantID: 0,
			wantContains: []string{
				"SELECT * FROM otel_metrics",
				"tenant_id = 0",
				"metric_name = 'up'",
			},
		},
		{
			name:     "metric with tenant 42",
			promql:   "http_requests_total",
			tenantID: 42,
			wantContains: []string{
				"otel_metrics",
				"tenant_id = 42",
				"metric_name = 'http_requests_total'",
			},
		},
		{
			name:     "metric with label",
			promql:   "cpu_usage{service=\"backend\"}",
			tenantID: 0,
			wantContains: []string{
				"otel_metrics",
				"metric_name = 'cpu_usage'",
				"service_name = 'backend'",
			},
		},
		{
			name:     "aggregation",
			promql:   "sum(memory_usage)",
			tenantID: 0,
			wantContains: []string{
				"SELECT SUM(value)",
				"FROM otel_metrics",
			},
		},
		{
			name:     "range query",
			promql:   "http_requests_total[5m]",
			tenantID: 0,
			wantContains: []string{
				"timestamp >= (now() - 300000)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := s.executePromQLQuery(ctx, tt.promql, tt.tenantID)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(queries) == 0 {
				t.Fatal("No SQL queries returned")
			}

			sql := queries[0]
			for _, want := range tt.wantContains {
				if !strings.Contains(sql, want) {
					t.Errorf("SQL should contain %q\nGot: %s", want, sql)
				}
			}
		})
	}
}
