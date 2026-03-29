package promql

import (
	"strings"
	"testing"
)

func TestVectorSelector(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name     string
		promql   string
		wantSQL  string
		wantErr  bool
	}{
		{
			name:    "simple metric name (PromQL style with underscores)",
			promql:  `http_requests_total`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'http.requests.total'",
			wantErr: false,
		},
		{
			name:    "OTel metric name with dots",
			promql:  `jvm.memory.used`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'jvm.memory.used'",
			wantErr: false,
		},
		{
			name:    "OTel metric with labels",
			promql:  `jvm.memory.used{job="myapp",area="heap"}`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'jvm.memory.used' AND job = 'myapp' AND JSON_EXTRACT_SCALAR(attributes, '$.area', 'STRING') = 'heap'",
			wantErr: false,
		},
		{
			name:    "http.server.duration - common OTel metric",
			promql:  `http.server.duration{method="GET"}`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'http.server.duration' AND http_method = 'GET'",
			wantErr: false,
		},
		{
			name:    "metric with single label (PromQL style)",
			promql:  `http_requests_total{job="api"}`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'http.requests.total' AND job = 'api'",
			wantErr: false,
		},
		{
			name:    "metric with multiple labels (PromQL style)",
			promql:  `http_requests_total{job="api",status="200"}`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'http.requests.total' AND job = 'api' AND http_status_code = '200'",
			wantErr: false,
		},
		{
			name:    "metric with label != matcher (PromQL style)",
			promql:  `http_requests_total{status!="500"}`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'http.requests.total' AND http_status_code <> '500'",
			wantErr: false,
		},
		{
			name:    "metric with regex matcher (PromQL style)",
			promql:  `http_requests_total{job=~"api.*"}`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'http.requests.total' AND REGEXP_LIKE(job, 'api.*')",
			wantErr: false,
		},
		{
			name:    "metric with negative regex matcher (PromQL style)",
			promql:  `http_requests_total{job!~"test.*"}`,
			wantSQL: "SELECT * FROM otel_metrics WHERE tenant_id = 0 AND metric_name = 'http.requests.total' AND NOT REGEXP_LIKE(job, 'test.*')",
			wantErr: false,
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

			gotSQL := strings.TrimSpace(sqlQueries[0])
			wantSQL := strings.TrimSpace(tt.wantSQL)

			if gotSQL != wantSQL {
				t.Errorf("TranslateQuery() SQL mismatch\ngot:  %s\nwant: %s", gotSQL, wantSQL)
			}
		})
	}
}

func TestMatrixSelector(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name         string
		promql       string
		wantContains []string
		wantErr      bool
	}{
		{
			name:   "5 minute range",
			promql: `http_requests_total[5m]`,
			wantContains: []string{
				"SELECT * FROM otel_metrics",
				"tenant_id = 0",
				"metric_name = 'http.requests.total'",
				"\"timestamp\" >= (now() - 300000)", // 5 minutes in ms
			},
			wantErr: false,
		},
		{
			name:   "1 hour range",
			promql: `cpu_usage{job="api"}[1h]`,
			wantContains: []string{
				"metric_name = 'cpu.usage'",
				"job = 'api'",
				"\"timestamp\" >= (now() - 3600000)", // 1 hour in ms
			},
			wantErr: false,
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

			gotSQL := sqlQueries[0]
			for _, want := range tt.wantContains {
				if !strings.Contains(gotSQL, want) {
					t.Errorf("SQL should contain %q\ngot: %s", want, gotSQL)
				}
			}
		})
	}
}

func TestAggregation(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name         string
		promql       string
		wantContains []string
		wantErr      bool
	}{
		{
			name:   "sum without grouping",
			promql: `sum(http_requests_total)`,
			wantContains: []string{
				"SELECT SUM(value)",
				"FROM otel_metrics",
				"tenant_id = 0",
				"metric_name = 'http.requests.total'",
			},
			wantErr: false,
		},
		{
			name:   "sum with grouping by single label",
			promql: `sum by (job) (http_requests_total)`,
			wantContains: []string{
				"SELECT job, SUM(value)",
				"GROUP BY job",
				"metric_name = 'http.requests.total'",
			},
			wantErr: false,
		},
		{
			name:   "sum with grouping by multiple labels",
			promql: `sum by (job, status) (http_requests_total)`,
			wantContains: []string{
				"SELECT job, http_status_code, SUM(value)",
				"GROUP BY job, http_status_code",
			},
			wantErr: false,
		},
		{
			name:   "avg aggregation",
			promql: `avg(cpu_usage)`,
			wantContains: []string{
				"SELECT AVG(value)",
				"metric_name = 'cpu.usage'",
			},
			wantErr: false,
		},
		{
			name:   "count aggregation",
			promql: `count(http_requests_total)`,
			wantContains: []string{
				"SELECT COUNT(*)",
				"metric_name = 'http.requests.total'",
			},
			wantErr: false,
		},
		{
			name:   "min aggregation",
			promql: `min(response_time)`,
			wantContains: []string{
				"SELECT MIN(value)",
				"metric_name = 'response.time'",
			},
			wantErr: false,
		},
		{
			name:   "max aggregation",
			promql: `max(response_time)`,
			wantContains: []string{
				"SELECT MAX(value)",
				"metric_name = 'response.time'",
			},
			wantErr: false,
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

			gotSQL := sqlQueries[0]
			for _, want := range tt.wantContains {
				if !strings.Contains(gotSQL, want) {
					t.Errorf("SQL should contain %q\ngot: %s", want, gotSQL)
				}
			}
		})
	}
}

func TestBinaryComparison(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name         string
		promql       string
		wantContains []string
		wantErr      bool
	}{
		{
			name:   "greater than",
			promql: `cpu_usage > 80`,
			wantContains: []string{
				"metric_name = 'cpu.usage'",
				"value > 80",
			},
			wantErr: false,
		},
		{
			name:   "less than",
			promql: `memory_usage < 50`,
			wantContains: []string{
				"metric_name = 'memory.usage'",
				"value < 50",
			},
			wantErr: false,
		},
		{
			name:   "greater than or equal",
			promql: `disk_usage >= 90`,
			wantContains: []string{
				"metric_name = 'disk.usage'",
				"value >= 90",
			},
			wantErr: false,
		},
		{
			name:   "less than or equal",
			promql: `latency <= 100`,
			wantContains: []string{
				"metric_name = 'latency'",
				"value <= 100",
			},
			wantErr: false,
		},
		{
			name:   "equal",
			promql: `status_code == 200`,
			wantContains: []string{
				"metric_name = 'status.code'",
				"value = 200",
			},
			wantErr: false,
		},
		{
			name:   "not equal",
			promql: `status_code != 500`,
			wantContains: []string{
				"metric_name = 'status.code'",
				"value <> 500",
			},
			wantErr: false,
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

			gotSQL := sqlQueries[0]
			for _, want := range tt.wantContains {
				if !strings.Contains(gotSQL, want) {
					t.Errorf("SQL should contain %q\ngot: %s", want, gotSQL)
				}
			}
		})
	}
}

func TestRateFunction(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name         string
		promql       string
		wantContains []string
		wantErr      bool
	}{
		{
			name:   "rate over 5 minutes",
			promql: `rate(http_requests_total[5m])`,
			wantContains: []string{
				"SELECT (MAX(value) - MIN(value))",
				"metric_name = 'http.requests.total'", // Translated from underscores to dots
				"\"timestamp\" >= (now() - 300000)",
			},
			wantErr: false,
		},
		{
			name:   "irate over 1 minute",
			promql: `irate(cpu_usage[1m])`,
			wantContains: []string{
				"SELECT (MAX(value) - MIN(value))",
				"metric_name = 'cpu.usage'", // Translated from underscores to dots
				"\"timestamp\" >= (now() - 60000)",
			},
			wantErr: false,
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

			gotSQL := sqlQueries[0]
			for _, want := range tt.wantContains {
				if !strings.Contains(gotSQL, want) {
					t.Errorf("SQL should contain %q\ngot: %s", want, gotSQL)
				}
			}
		})
	}
}

func TestUnsupportedFeatures(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name   string
		promql string
	}{
		{
			name:   "binary operation between metrics",
			promql: `metric1 / metric2`,
		},
		{
			name:   "subquery",
			promql: `rate(http_requests_total[5m:1m])`,
		},
		{
			name:   "histogram_quantile",
			promql: `histogram_quantile(0.95, http_request_duration_bucket)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := translator.TranslateQuery(tt.promql)
			if err == nil {
				t.Errorf("expected error for unsupported feature, got nil")
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name   string
		promql string
	}{
		{
			name:   "invalid syntax",
			promql: `http_requests_total{`,
		},
		{
			name:   "unclosed bracket",
			promql: `http_requests_total[5m`,
		},
		{
			name:   "invalid label matcher",
			promql: `http_requests_total{job}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := translator.TranslateQuery(tt.promql)
			if err == nil {
				t.Errorf("expected parse error, got nil")
			}
		})
	}
}

func TestScalarArithmetic(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name         string
		promql       string
		wantContains []string
		wantErr      bool
	}{
		{
			name:   "addition (Grafana connection test)",
			promql: `1+1`,
			wantContains: []string{
				"SELECT 2.000000 AS value",
				"FROM otel_metrics",
				"LIMIT 1",
			},
			wantErr: false,
		},
		{
			name:   "subtraction",
			promql: `10-3`,
			wantContains: []string{
				"SELECT 7.000000 AS value",
			},
			wantErr: false,
		},
		{
			name:   "multiplication",
			promql: `2*3`,
			wantContains: []string{
				"SELECT 6.000000 AS value",
			},
			wantErr: false,
		},
		{
			name:   "division",
			promql: `10/2`,
			wantContains: []string{
				"SELECT 5.000000 AS value",
			},
			wantErr: false,
		},
		{
			name:    "division by zero",
			promql:  `10/0`,
			wantErr: true,
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

			gotSQL := sqlQueries[0]
			for _, want := range tt.wantContains {
				if !strings.Contains(gotSQL, want) {
					t.Errorf("SQL should contain %q\ngot: %s", want, gotSQL)
				}
			}
		})
	}
}
