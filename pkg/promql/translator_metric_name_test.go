package promql

import (
	"strings"
	"testing"
)

func TestTranslateMetricName(t *testing.T) {
	tests := []struct {
		name           string
		prometheusName string
		want           string
	}{
		{
			name:           "simple underscore translation",
			prometheusName: "http_requests_total",
			want:           "http.requests.total",
		},
		{
			name:           "JVM metric with multiple underscores",
			prometheusName: "jvm_memory_used",
			want:           "jvm.memory.used",
		},
		{
			name:           "multiple consecutive underscores",
			prometheusName: "foo__bar",
			want:           "foo..bar",
		},
		{
			name:           "no underscores (already dots)",
			prometheusName: "jvm.memory.used",
			want:           "jvm.memory.used",
		},
		{
			name:           "no separators",
			prometheusName: "requests",
			want:           "requests",
		},
		{
			name:           "trailing underscore",
			prometheusName: "metric_name_",
			want:           "metric.name.",
		},
		{
			name:           "leading underscore",
			prometheusName: "_metric_name",
			want:           ".metric.name",
		},
		{
			name:           "complex OTel-style metric",
			prometheusName: "http_server_request_duration",
			want:           "http.server.request.duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := translateMetricName(tt.prometheusName)
			if got != tt.want {
				t.Errorf("translateMetricName(%q) = %q, want %q", tt.prometheusName, got, tt.want)
			}
		})
	}
}

func TestPromQLQueryWithUnderscores(t *testing.T) {
	// Test that PromQL queries with underscores get translated to dots
	translator := NewTranslator(0)

	tests := []struct {
		name          string
		promql        string
		expectInSQL   string
		shouldContain bool
	}{
		{
			name:          "simple metric with underscores",
			promql:        "jvm_memory_used",
			expectInSQL:   "jvm.memory.used",
			shouldContain: true,
		},
		{
			name:          "metric with label matcher",
			promql:        `jvm_memory_used{job="api"}`,
			expectInSQL:   "jvm.memory.used",
			shouldContain: true,
		},
		{
			name:          "metric using __name__ label",
			promql:        `{__name__="http_requests_total"}`,
			expectInSQL:   "http.requests.total",
			shouldContain: true,
		},
		{
			name:          "metric already with dots",
			promql:        `{__name__="jvm.memory.used"}`,
			expectInSQL:   "jvm.memory.used",
			shouldContain: true,
		},
		{
			name:          "aggregation with underscores",
			promql:        "sum(http_requests_total)",
			expectInSQL:   "http.requests.total",
			shouldContain: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlQueries, err := translator.TranslateQuery(tt.promql)
			if err != nil {
				t.Fatalf("TranslateQuery(%q) failed: %v", tt.promql, err)
			}

			if len(sqlQueries) == 0 {
				t.Fatalf("TranslateQuery(%q) returned no SQL queries", tt.promql)
			}

			sql := sqlQueries[0]
			contains := strings.Contains(sql, tt.expectInSQL)

			if contains != tt.shouldContain {
				t.Errorf("TranslateQuery(%q):\nGot SQL: %s\nExpected to contain %q: %v, actually contains: %v",
					tt.promql, sql, tt.expectInSQL, tt.shouldContain, contains)
			}
		})
	}
}

func TestPromQLDoesNotBreakDotsInMetricNames(t *testing.T) {
	// Test that if someone explicitly uses dots (via __name__), we don't break it
	translator := NewTranslator(0)

	promql := `{__name__="already.has.dots"}`

	sqlQueries, err := translator.TranslateQuery(promql)
	if err != nil {
		t.Fatalf("TranslateQuery failed: %v", err)
	}

	sql := sqlQueries[0]

	// Should preserve the dots
	if !strings.Contains(sql, "already.has.dots") {
		t.Errorf("Expected SQL to contain 'already.has.dots', got: %s", sql)
	}
}

func TestPromQLUnderscoreTranslationWithTimeRange(t *testing.T) {
	// Test that translation works with time range queries
	translator := NewTranslator(0)

	promql := "jvm_memory_used[5m]"

	sqlQueries, err := translator.TranslateQuery(promql)
	if err != nil {
		t.Fatalf("TranslateQuery failed: %v", err)
	}

	sql := sqlQueries[0]

	// Should translate metric name
	if !strings.Contains(sql, "jvm.memory.used") {
		t.Errorf("Expected SQL to contain 'jvm.memory.used', got: %s", sql)
	}

	// Should also have time range
	if !strings.Contains(sql, `"timestamp" >=`) {
		t.Errorf("Expected SQL to contain time range filter, got: %s", sql)
	}
}
