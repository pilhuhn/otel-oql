package promql

import (
	"strings"
	"testing"
)

// TestMetricNameConversion_PromQLToOTel verifies that PromQL-style metric names
// (with underscores) are converted to OTel format (with dots) in the generated SQL.
func TestMetricNameConversion_PromQLToOTel(t *testing.T) {
	tests := []struct {
		name        string
		promql      string
		wantInSQL   string
		description string
	}{
		{
			name:        "simple metric with underscores",
			promql:      "jvm_memory_used",
			wantInSQL:   "metric_name = 'jvm.memory.used'",
			description: "PromQL input jvm_memory_used should query database as jvm.memory.used",
		},
		{
			name:        "metric with underscores and labels",
			promql:      `jvm_memory_used{area="heap"}`,
			wantInSQL:   "metric_name = 'jvm.memory.used'",
			description: "PromQL input with underscores converts to dots even with labels",
		},
		{
			name:        "http server duration",
			promql:      `http_server_duration`,
			wantInSQL:   "metric_name = 'http.server.duration'",
			description: "Multi-segment underscore metric converts all underscores to dots",
		},
		{
			name:        "rate function with underscores",
			promql:      `rate(http_requests_total[5m])`,
			wantInSQL:   "metric_name = 'http.requests.total'",
			description: "Function calls also convert metric names",
		},
		{
			name:        "aggregation with underscores",
			promql:      `sum(jvm_memory_used)`,
			wantInSQL:   "metric_name = 'jvm.memory.used'",
			description: "Aggregations convert metric names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.promql)
			if err != nil {
				t.Fatalf("TranslateQuery(%q) failed: %v", tt.promql, err)
			}

			if len(sqlQueries) == 0 {
				t.Fatalf("TranslateQuery(%q) returned no SQL queries", tt.promql)
			}

			sql := sqlQueries[0]
			if !strings.Contains(sql, tt.wantInSQL) {
				t.Errorf("%s\nPromQL: %s\nExpected SQL to contain: %s\nGot SQL: %s",
					tt.description, tt.promql, tt.wantInSQL, sql)
			}
		})
	}
}

// TestMetricNameConversion_OTelInput verifies that OTel-style metric names
// (with dots) are preserved in the generated SQL.
func TestMetricNameConversion_OTelInput(t *testing.T) {
	tests := []struct {
		name        string
		promql      string
		wantInSQL   string
		description string
	}{
		{
			name:        "metric with dots preserved",
			promql:      "jvm.memory.used",
			wantInSQL:   "metric_name = 'jvm.memory.used'",
			description: "OTel input jvm.memory.used should remain as jvm.memory.used",
		},
		{
			name:        "http.server.duration preserved",
			promql:      `http.server.duration{method="GET"}`,
			wantInSQL:   "metric_name = 'http.server.duration'",
			description: "OTel dotted names are preserved when input has dots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.promql)
			if err != nil {
				t.Fatalf("TranslateQuery(%q) failed: %v", tt.promql, err)
			}

			if len(sqlQueries) == 0 {
				t.Fatalf("TranslateQuery(%q) returned no SQL queries", tt.promql)
			}

			sql := sqlQueries[0]
			if !strings.Contains(sql, tt.wantInSQL) {
				t.Errorf("%s\nPromQL: %s\nExpected SQL to contain: %s\nGot SQL: %s",
					tt.description, tt.promql, tt.wantInSQL, sql)
			}
		})
	}
}

// TestMetricNameConversion_Grafana simulates Grafana autocomplete workflow
func TestMetricNameConversion_Grafana(t *testing.T) {
	t.Run("grafana autocomplete workflow", func(t *testing.T) {
		translator := NewTranslator(0)

		// Step 1: Grafana gets "jvm_memory_used" from label values endpoint (tested separately in API tests)
		// Step 2: User selects "jvm_memory_used" and creates a query
		// Step 3: Query is sent to PromQL translator
		promqlQuery := "jvm_memory_used"

		// Step 4: Translator should convert to OTel format for database query
		sqlQueries, err := translator.TranslateQuery(promqlQuery)
		if err != nil {
			t.Fatalf("TranslateQuery(%q) failed: %v", promqlQuery, err)
		}

		sql := sqlQueries[0]

		// Step 5: Verify SQL queries the database using OTel format (dots)
		if !strings.Contains(sql, "metric_name = 'jvm.memory.used'") {
			t.Errorf("Expected SQL to query OTel format (jvm.memory.used), got: %s", sql)
		}
	})
}
