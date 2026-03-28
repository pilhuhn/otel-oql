package api

import (
	"context"
	"strings"
	"testing"
)

// TestLogQLTranslation tests that LogQL queries translate to correct SQL
func TestLogQLTranslation(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	tenantID := 0

	tests := []struct {
		name         string
		logql        string
		wantContains []string
		wantErr      bool
		errContain   string
	}{
		{
			name:  "simple stream selector",
			logql: `{job="varlogs"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
			},
		},
		{
			name:  "stream selector with native column",
			logql: `{level="error"}`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND log_level = 'error'",
			},
		},
		{
			name:  "line filter contains",
			logql: `{job="varlogs"} |= "error"`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND body LIKE '%error%'",
			},
		},
		{
			name:  "line filter regex",
			logql: `{job="varlogs"} |~ "error|fail"`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND REGEXP_LIKE(body, 'error|fail')",
			},
		},
		{
			name:  "count_over_time",
			logql: `count_over_time({job="varlogs"}[5m])`,
			wantContains: []string{
				"SELECT COUNT(*) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 300000)",
			},
		},
		{
			name:  "bytes_over_time",
			logql: `bytes_over_time({job="varlogs"}[5m])`,
			wantContains: []string{
				"SELECT SUM(LENGTH(body)) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 300000)",
			},
		},
		{
			name:  "count_over_time with line filter",
			logql: `count_over_time({job="varlogs"} |= "error"[5m])`,
			wantContains: []string{
				"SELECT COUNT(*) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND body LIKE '%error%'",
				"AND timestamp >= (now() - 300000)",
			},
		},
		{
			name:  "sum by level",
			logql: `sum by (level) (count_over_time({job="varlogs"}[5m]))`,
			wantContains: []string{
				"SELECT log_level, COUNT(*) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 300000)",
				"GROUP BY log_level",
			},
		},
		{
			name:  "sum by custom attribute",
			logql: `sum by (environment) (count_over_time({job="varlogs"}[5m]))`,
			wantContains: []string{
				"SELECT environment, COUNT(*) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 300000)",
				"GROUP BY environment",
			},
		},
		{
			name:  "avg by multiple labels",
			logql: `avg by (level, service) (count_over_time({job="varlogs"}[5m]))`,
			wantContains: []string{
				"SELECT log_level, service_name, COUNT(*) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 300000)",
				"GROUP BY log_level, service_name",
			},
		},
		{
			name:       "invalid syntax - unclosed selector",
			logql:      `{job="varlogs"`,
			wantErr:    true,
			errContain: "unclosed stream selector",
		},
		{
			name:       "invalid syntax - empty selector",
			logql:      `{}`,
			wantErr:    true,
			errContain: "parse error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := s.executeLogQLQuery(ctx, tt.logql, tenantID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContain)
					return
				}
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error = %v, want error containing %q", err, tt.errContain)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(queries) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(queries))
				return
			}

			sql := queries[0]
			for _, want := range tt.wantContains {
				if !strings.Contains(sql, want) {
					t.Errorf("SQL missing expected substring:\nwant: %s\ngot:  %s", want, sql)
				}
			}
		})
	}
}

// TestLogQLTenantIsolation tests that LogQL queries enforce tenant isolation
func TestLogQLTenantIsolation(t *testing.T) {
	s := &Server{}
	ctx := context.Background()

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
			wantSQL:  "SELECT * FROM otel_logs WHERE tenant_id = 0 AND job = 'varlogs'",
		},
		{
			name:     "tenant 1",
			tenantID: 1,
			logql:    `{job="varlogs"}`,
			wantSQL:  "SELECT * FROM otel_logs WHERE tenant_id = 1 AND job = 'varlogs'",
		},
		{
			name:     "tenant 42",
			tenantID: 42,
			logql:    `{job="varlogs"}`,
			wantSQL:  "SELECT * FROM otel_logs WHERE tenant_id = 42 AND job = 'varlogs'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := s.executeLogQLQuery(ctx, tt.logql, tt.tenantID)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(queries) != 1 {
				t.Errorf("expected 1 SQL query, got %d", len(queries))
				return
			}

			if queries[0] != tt.wantSQL {
				t.Errorf("SQL mismatch:\ngot:  %s\nwant: %s", queries[0], tt.wantSQL)
			}
		})
	}
}

// TestLogQLComplexQueries tests more complex LogQL query patterns
func TestLogQLComplexQueries(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	tenantID := 0

	tests := []struct {
		name         string
		logql        string
		wantContains []string
	}{
		{
			name:  "multiple matchers and pipeline",
			logql: `{job="varlogs", level!="debug"} |= "error" != "timeout"`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND log_level <> 'debug'",
				"AND body LIKE '%error%'",
				"AND body NOT LIKE '%timeout%'",
			},
		},
		{
			name:  "regex matchers",
			logql: `{job=~"var.*", level!~"debug.*"} |~ "error|fail"`,
			wantContains: []string{
				"SELECT * FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND REGEXP_LIKE(job, 'var.*')",
				"AND NOT REGEXP_LIKE(log_level, 'debug.*')",
				"AND REGEXP_LIKE(body, 'error|fail')",
			},
		},
		{
			name:  "rate function",
			logql: `rate({job="varlogs"}[10m])`,
			wantContains: []string{
				"SELECT COUNT(*) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 600000)",
			},
		},
		{
			name:  "bytes_rate function",
			logql: `bytes_rate({job="varlogs"}[10m])`,
			wantContains: []string{
				"SELECT SUM(LENGTH(body)) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 600000)",
			},
		},
		{
			name:  "sum by with bytes_over_time",
			logql: `sum by (level) (bytes_over_time({job="varlogs"}[5m]))`,
			wantContains: []string{
				"SELECT log_level, SUM(LENGTH(body)) FROM otel_logs",
				"WHERE tenant_id = 0",
				"AND job = 'varlogs'",
				"AND timestamp >= (now() - 300000)",
				"GROUP BY log_level",
			},
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
			for _, want := range tt.wantContains {
				if !strings.Contains(sql, want) {
					t.Errorf("SQL missing expected substring:\nwant: %s\ngot:  %s", want, sql)
				}
			}
		})
	}
}
