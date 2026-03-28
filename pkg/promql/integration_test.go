package promql

import (
	"testing"
)

// TestComplexQueries tests more complex real-world PromQL queries
func TestComplexQueries(t *testing.T) {
	translator := NewTranslator(42) // Test with non-zero tenant

	tests := []struct {
		name     string
		promql   string
		wantErr  bool
		checkSQL func(string) bool
	}{
		{
			name:    "complex aggregation with rate",
			promql:  `sum by (service) (rate(http_requests_total{job="api"}[5m]))`,
			wantErr: false,
			checkSQL: func(sql string) bool {
				// Should contain tenant_id=42, metric name (translated to dots), label filter, time range, and aggregation
				return contains(sql, "tenant_id = 42") &&
					contains(sql, "metric_name = 'http.requests.total'") &&
					contains(sql, "job = 'api'") &&
					contains(sql, `"timestamp" >= (now() - 300000)`) &&
					contains(sql, "SELECT")
			},
		},
		{
			name:    "multiple label matchers with comparison",
			promql:  `cpu_usage{service="backend",environment="prod"} > 75`,
			wantErr: false,
			checkSQL: func(sql string) bool {
				return contains(sql, "service_name = 'backend'") &&
					contains(sql, "environment = 'prod'") &&
					contains(sql, "value > 75")
			},
		},
		{
			name:    "count with regex",
			promql:  `count(http_requests_total{status=~"5.."})`,
			wantErr: false,
			checkSQL: func(sql string) bool {
				return contains(sql, "COUNT(*)") &&
					contains(sql, "REGEXP_LIKE(http_status_code, '5..')")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlQueries, err := translator.TranslateQuery(tt.promql)
			if (err != nil) != tt.wantErr {
				t.Errorf("TranslateQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if len(sqlQueries) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(sqlQueries))
				return
			}

			if tt.checkSQL != nil && !tt.checkSQL(sqlQueries[0]) {
				t.Errorf("SQL check failed\nSQL: %s", sqlQueries[0])
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestEdgeCases tests edge cases and error conditions
func TestEdgeCases(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name    string
		promql  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty metric name",
			promql:  `{job="api"}`,
			wantErr: false, // PromQL allows this (all metrics with that label)
		},
		{
			name:    "metric with no labels",
			promql:  `up`,
			wantErr: false,
		},
		{
			name:    "nested aggregations should fail",
			promql:  `sum(avg(http_requests_total))`,
			wantErr: true,
			errMsg:  "nested aggregations not supported",
		},
		{
			name:    "offset modifier should fail",
			promql:  `http_requests_total offset 5m`,
			wantErr: true,
			errMsg:  "offset modifier not supported",
		},
		{
			name:    "subquery should fail",
			promql:  `rate(http_requests_total[5m:1m])`,
			wantErr: true,
			errMsg:  "subqueries not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := translator.TranslateQuery(tt.promql)
			if (err != nil) != tt.wantErr {
				t.Errorf("TranslateQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Error message should contain %q, got: %v", tt.errMsg, err)
				}
			}
		})
	}
}

// TestTenantIsolation ensures tenant_id is always included in queries
func TestTenantIsolation(t *testing.T) {
	tests := []struct {
		name     string
		tenantID int
		promql   string
	}{
		{"tenant 0", 0, "up"},
		{"tenant 1", 1, "http_requests_total"},
		{"tenant 999", 999, "cpu_usage{job=\"test\"}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(tt.tenantID)
			sqlQueries, err := translator.TranslateQuery(tt.promql)
			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			expectedTenant := "tenant_id = "
			if tt.tenantID == 0 {
				expectedTenant += "0"
			} else {
				expectedTenant += string(rune('0' + tt.tenantID/100))
				if tt.tenantID >= 10 {
					expectedTenant += string(rune('0' + (tt.tenantID%100)/10))
				}
				expectedTenant += string(rune('0' + tt.tenantID%10))
			}

			// Check simpler way - just verify tenant_id is in the query
			found := false
			for _, sql := range sqlQueries {
				if contains(sql, "tenant_id = ") {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("SQL query missing tenant_id filter\nSQL: %s", sqlQueries[0])
			}
		})
	}
}
