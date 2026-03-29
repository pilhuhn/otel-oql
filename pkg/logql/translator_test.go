package logql

import (
	"strings"
	"testing"
)

func TestTranslateQuery_LogRangeExpr(t *testing.T) {
	tests := []struct {
		name     string
		logql    string
		wantSQL  string
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "simple stream selector",
			logql:   `{job="varlogs"}`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs'`,
		},
		{
			name:    "stream selector with native column",
			logql:   `{level="error"}`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND log_level = 'error'`,
		},
		{
			name:    "multiple label matchers",
			logql:   `{job="varlogs", level="error"}`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND log_level = 'error'`,
		},
		{
			name:    "stream selector with != matcher",
			logql:   `{job="test", level!="debug"}`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'test' AND log_level <> 'debug'`,
		},
		{
			name:    "stream selector with regex matcher",
			logql:   `{job=~"var.*"}`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND REGEXP_LIKE(job, 'var.*')`,
		},
		{
			name:    "stream selector with negative regex matcher",
			logql:   `{job="varlogs", level!~"debug.*"}`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND NOT REGEXP_LIKE(log_level, 'debug.*')`,
		},
		{
			name:    "line filter contains",
			logql:   `{job="varlogs"} |= "error"`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND body LIKE '%error%'`,
		},
		{
			name:    "line filter not contains",
			logql:   `{job="varlogs"} != "debug"`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND body NOT LIKE '%debug%'`,
		},
		{
			name:    "line filter regex match",
			logql:   `{job="varlogs"} |~ "error|fail"`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND REGEXP_LIKE(body, 'error|fail')`,
		},
		{
			name:    "line filter regex not match",
			logql:   `{job="varlogs"} !~ "debug|trace"`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND NOT REGEXP_LIKE(body, 'debug|trace')`,
		},
		{
			name:    "multiple line filters",
			logql:   `{job="varlogs"} |= "error" != "timeout"`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND body LIKE '%error%' AND body NOT LIKE '%timeout%'`,
		},
		{
			name:    "label parser json",
			logql:   `{job="varlogs"} | json`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs'`,
		},
		{
			name:    "label parser logfmt",
			logql:   `{job="varlogs"} | logfmt`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs'`,
		},
		{
			name:   "missing stream selector",
			logql:  `|= "error"`,
			wantErr: true,
			errMsg: "query must start with stream selector",
		},
		{
			name:   "empty stream selector",
			logql:  `{}`,
			wantErr: true,
			errMsg: "parse error", // Prometheus parser rejects this
		},
		{
			name:   "only negative matchers",
			logql:  `{job!="test"}`,
			wantErr: true,
			errMsg: "parse error", // Prometheus parser rejects this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.logql)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sqls) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(sqls))
				return
			}

			if sqls[0] != tt.wantSQL {
				t.Errorf("SQL mismatch:\ngot:  %s\nwant: %s", sqls[0], tt.wantSQL)
			}
		})
	}
}

func TestTranslateQuery_MetricExpr(t *testing.T) {
	tests := []struct {
		name    string
		logql   string
		wantSQL string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "count_over_time simple",
			logql:   `count_over_time({job="varlogs"}[5m])`,
			wantSQL: `SELECT COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000)`,
		},
		{
			name:    "count_over_time with native column",
			logql:   `count_over_time({level="error"}[1h])`,
			wantSQL: `SELECT COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND log_level = 'error' AND "timestamp" >= (now() - 3600000)`,
		},
		{
			name:    "rate function",
			logql:   `rate({job="varlogs"}[5m])`,
			wantSQL: `SELECT COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000)`,
		},
		{
			name:    "bytes_over_time",
			logql:   `bytes_over_time({job="varlogs"}[5m])`,
			wantSQL: `SELECT SUM(LENGTH(body)) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000)`,
		},
		{
			name:    "bytes_rate",
			logql:   `bytes_rate({job="varlogs"}[10m])`,
			wantSQL: `SELECT SUM(LENGTH(body)) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 600000)`,
		},
		{
			name:    "count_over_time with line filter",
			logql:   `count_over_time({job="varlogs"} |= "error"[5m])`,
			wantSQL: `SELECT COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND body LIKE '%error%' AND "timestamp" >= (now() - 300000)`,
		},
		{
			name:    "count_over_time with multiple filters",
			logql:   `count_over_time({job="varlogs"} |= "error" != "timeout"[1h])`,
			wantSQL: `SELECT COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND body LIKE '%error%' AND body NOT LIKE '%timeout%' AND "timestamp" >= (now() - 3600000)`,
		},
		{
			name:    "sum aggregation",
			logql:   `sum(count_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000)`,
		},
		{
			name:    "sum by level",
			logql:   `sum by (level) (count_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT log_level, COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000) GROUP BY log_level`,
		},
		{
			name:    "sum by custom attribute",
			logql:   `sum by (environment) (count_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT environment, COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000) GROUP BY environment`,
		},
		{
			name:    "avg by multiple labels",
			logql:   `avg by (level, service) (count_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT log_level, service_name, COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000) GROUP BY log_level, service_name`,
		},
		{
			name:    "count aggregation",
			logql:   `count by (level) (count_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT log_level, COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000) GROUP BY log_level`,
		},
		{
			name:    "min aggregation",
			logql:   `min by (service) (count_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT service_name, COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000) GROUP BY service_name`,
		},
		{
			name:    "max aggregation",
			logql:   `max by (level) (count_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT log_level, COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000) GROUP BY log_level`,
		},
		{
			name:    "sum by with bytes_over_time",
			logql:   `sum by (level) (bytes_over_time({job="varlogs"}[5m]))`,
			wantSQL: `SELECT log_level, SUM(LENGTH(body)) FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND "timestamp" >= (now() - 300000) GROUP BY log_level`,
		},
		{
			name:   "unsupported metric function",
			logql:  `unsupported_func({job="varlogs"}[5m])`,
			wantErr: true,
			errMsg: "query must start with stream selector", // Not recognized as metric query
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.logql)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sqls) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(sqls))
				return
			}

			if sqls[0] != tt.wantSQL {
				t.Errorf("SQL mismatch:\ngot:  %s\nwant: %s", sqls[0], tt.wantSQL)
			}
		})
	}
}

func TestTranslateQuery_TenantIsolation(t *testing.T) {
	tests := []struct {
		name     string
		tenantID int
		logql    string
		wantSQL  string
	}{
		{
			name:     "tenant 0",
			tenantID: 0,
			logql:    `{job="varlogs"}`,
			wantSQL:  `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs'`,
		},
		{
			name:     "tenant 1",
			tenantID: 1,
			logql:    `{job="varlogs"}`,
			wantSQL:  `SELECT * FROM otel_logs WHERE tenant_id = 1 AND job = 'varlogs'`,
		},
		{
			name:     "tenant 42",
			tenantID: 42,
			logql:    `{job="varlogs"}`,
			wantSQL:  `SELECT * FROM otel_logs WHERE tenant_id = 42 AND job = 'varlogs'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(tt.tenantID)
			sqls, err := translator.TranslateQuery(tt.logql)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sqls) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(sqls))
				return
			}

			if sqls[0] != tt.wantSQL {
				t.Errorf("SQL mismatch:\ngot:  %s\nwant: %s", sqls[0], tt.wantSQL)
			}
		})
	}
}

func TestTranslateQuery_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		logql   string
		wantErr bool
		errMsg  string
	}{
		{
			name:   "empty query",
			logql:  "",
			wantErr: true,
			errMsg: "query must start with stream selector",
		},
		{
			name:   "only whitespace",
			logql:  "   ",
			wantErr: true,
			errMsg: "query must start with stream selector",
		},
		{
			name:   "unclosed stream selector",
			logql:  `{job="varlogs"`,
			wantErr: true,
			errMsg: "unclosed stream selector",
		},
		{
			name:   "invalid label matcher",
			logql:  `{job}`,
			wantErr: true,
			errMsg: "failed to parse stream selector",
		},
		{
			name:   "invalid pipeline operator",
			logql:  `{job="varlogs"} |> "error"`,
			wantErr: true,
			errMsg: "unknown pipeline stage",
		},
		{
			name:   "unclosed string in line filter",
			logql:  `{job="varlogs"} |= "error`,
			wantErr: true,
			errMsg: "failed to parse pipeline",
		},
		{
			name:   "missing time range in metric query",
			logql:  `count_over_time({job="varlogs"})`,
			wantErr: true,
			errMsg: "requires a range vector argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			_, err := translator.TranslateQuery(tt.logql)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTranslateQuery_ScalarExpr(t *testing.T) {
	tests := []struct {
		name    string
		logql   string
		wantSQL string
		wantErr bool
	}{
		{
			name:    "vector addition - Grafana connection test",
			logql:   `vector(1)+vector(1)`,
			wantSQL: `SELECT 2.000000 AS value FROM otel_logs LIMIT 1`,
		},
		{
			name:    "vector subtraction",
			logql:   `vector(5)-vector(3)`,
			wantSQL: `SELECT 2.000000 AS value FROM otel_logs LIMIT 1`,
		},
		{
			name:    "vector multiplication",
			logql:   `vector(3)*vector(4)`,
			wantSQL: `SELECT 12.000000 AS value FROM otel_logs LIMIT 1`,
		},
		{
			name:    "vector division",
			logql:   `vector(10)/vector(2)`,
			wantSQL: `SELECT 5.000000 AS value FROM otel_logs LIMIT 1`,
		},
		{
			name:    "simple number addition",
			logql:   `1+1`,
			wantSQL: `SELECT 2.000000 AS value FROM otel_logs LIMIT 1`,
		},
		{
			name:    "single vector value",
			logql:   `vector(42)`,
			wantSQL: `SELECT 42.000000 AS value FROM otel_logs LIMIT 1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.logql)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sqls) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(sqls))
				return
			}

			if sqls[0] != tt.wantSQL {
				t.Errorf("SQL mismatch:\ngot:  %s\nwant: %s", sqls[0], tt.wantSQL)
			}
		})
	}
}

func TestTranslateQuery_WithDrop(t *testing.T) {
	tests := []struct {
		name    string
		logql   string
		wantSQL string
	}{
		{
			name:    "line filter with drop",
			logql:   `{job="varlogs"} |= "error" | drop __error__`,
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND body LIKE '%error%'`,
		},
		{
			name:    "count_over_time with drop",
			logql:   `count_over_time({host_name="snert"} |= "replicator" | drop __error__[1m])`,
			wantSQL: `SELECT COUNT(*) FROM otel_logs WHERE tenant_id = 0 AND host_name = 'snert' AND body LIKE '%replicator%' AND "timestamp" >= (now() - 60000)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.logql)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sqls) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(sqls))
				return
			}

			if sqls[0] != tt.wantSQL {
				t.Errorf("SQL mismatch:\ngot:  %s\nwant: %s", sqls[0], tt.wantSQL)
			}
		})
	}
}

func TestTranslateQuery_WithBackticks(t *testing.T) {
	tests := []struct {
		name    string
		logql   string
		wantSQL string
	}{
		{
			name:    "line filter with backticks",
			logql:   "{job=\"varlogs\"} |= `error`",
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs' AND body LIKE '%error%'`,
		},
		{
			name:    "backticks and drop - Grafana query",
			logql:   "{host_name=\"snert\"} |= `replicator` | drop __error__",
			wantSQL: `SELECT * FROM otel_logs WHERE tenant_id = 0 AND host_name = 'snert' AND body LIKE '%replicator%'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.logql)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(sqls) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(sqls))
				return
			}

			if sqls[0] != tt.wantSQL {
				t.Errorf("SQL mismatch:\ngot:  %s\nwant: %s", sqls[0], tt.wantSQL)
			}
		})
	}
}
