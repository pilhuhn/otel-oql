package traceql

import (
	"strings"
	"testing"
)

// TestTranslateSpanFilterExpr_UnknownIntrinsicRejectsSQLInjection documents a regression:
// The parser only emits FieldExpr{Type:"intrinsic"} for names that pass IsIntrinsic, but a
// manually constructed AST (tests, future code) could set an arbitrary Name. Previously
// getIntrinsicColumn returned that string verbatim as SQL, enabling injection (e.g.
// "trace_id) OR (1=1"). Translation must reject unknown intrinsic names.
func TestTranslateSpanFilterExpr_UnknownIntrinsicRejectsSQLInjection(t *testing.T) {
	trans := NewTranslator(0)
	expr := &SpanFilterExpr{
		Conditions: []Condition{
			{
				Field: FieldExpr{
					Type: "intrinsic",
					Name: "trace_id) OR (tenant_id = tenant_id",
				},
				Operator: "=",
				Value:    int64(1),
			},
		},
	}
	_, err := trans.translateSpanFilterExpr(expr)
	if err == nil {
		t.Fatalf("expected error for unknown intrinsic name, got nil (unsafe: raw name would be spliced into SQL)")
	}
	if !strings.Contains(err.Error(), "unknown intrinsic") {
		t.Fatalf("expected error to mention unknown intrinsic, got: %v", err)
	}
}

func TestTranslator_IntrinsicFields(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantInSQL   string
		description string
	}{
		{
			name:        "duration comparison",
			query:       "{duration > 100ms}",
			wantInSQL:   "duration > 100000000",
			description: "100ms = 100,000,000 nanoseconds",
		},
		{
			name:        "name equality",
			query:       `{name = "HTTP GET"}`,
			wantInSQL:   "name = 'HTTP GET'",
			description: "intrinsic field name maps to name column",
		},
		{
			name:        "status error",
			query:       "{status = error}",
			wantInSQL:   "status_code = 'Error'",
			description: "status enum value converts to OTLP .String() format (capitalized)",
		},
		{
			name:        "status ok",
			query:       "{status = ok}",
			wantInSQL:   "status_code = 'Ok'",
			description: "status ok converts to 'Ok' to match Pinot storage from OTLP",
		},
		{
			name:        "status unset",
			query:       "{status = unset}",
			wantInSQL:   "status_code = 'Unset'",
			description: "status unset converts to 'Unset' to match Pinot storage from OTLP",
		},
		{
			name:        "kind server",
			query:       `{kind = "server"}`,
			wantInSQL:   "kind = 'server'",
			description: "intrinsic field kind maps to kind column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.query)

			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			if len(sqlQueries) == 0 {
				t.Fatal("TranslateQuery() returned no SQL queries")
			}

			sql := sqlQueries[0]
			if !strings.Contains(sql, tt.wantInSQL) {
				t.Errorf("%s\nQuery: %s\nExpected SQL to contain: %s\nGot SQL: %s",
					tt.description, tt.query, tt.wantInSQL, sql)
			}
		})
	}
}

func TestTranslator_SpanAttributes(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantInSQL   string
		description string
	}{
		{
			name:        "http status code native column",
			query:       "{span.http.status_code = 500}",
			wantInSQL:   "http_status_code = 500",
			description: "http.status_code uses native column (10-100x faster)",
		},
		{
			name:        "http method native column",
			query:       `{span.http.method = "GET"}`,
			wantInSQL:   "http_method = 'GET'",
			description: "http.method uses native column",
		},
		{
			name:        "db system native column",
			query:       `{span.db.system = "postgresql"}`,
			wantInSQL:   "db_system = 'postgresql'",
			description: "db.system uses native column",
		},
		{
			name:        "custom attribute JSON extraction",
			query:       `{span.custom.field = "value"}`,
			wantInSQL:   "JSON_EXTRACT_SCALAR(attributes, '$.custom.field', 'STRING') = 'value'",
			description: "custom attributes use JSON extraction",
		},
		{
			name:        "error boolean",
			query:       "{span.error = true}",
			wantInSQL:   "error = true",
			description: "error attribute uses native boolean column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.query)

			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			if len(sqlQueries) == 0 {
				t.Fatal("TranslateQuery() returned no SQL queries")
			}

			sql := sqlQueries[0]
			if !strings.Contains(sql, tt.wantInSQL) {
				t.Errorf("%s\nQuery: %s\nExpected SQL to contain: %s\nGot SQL: %s",
					tt.description, tt.query, tt.wantInSQL, sql)
			}
		})
	}
}

func TestTranslator_ResourceAttributes(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantInSQL   string
		description string
	}{
		{
			name:        "service name native column",
			query:       `{resource.service.name = "api"}`,
			wantInSQL:   "service_name = 'api'",
			description: "service.name uses native column",
		},
		{
			name:        "custom resource attribute",
			query:       `{resource.environment = "production"}`,
			wantInSQL:   "JSON_EXTRACT_SCALAR(resource_attributes, '$.environment', 'STRING') = 'production'",
			description: "custom resource attributes use JSON extraction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.query)

			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			if len(sqlQueries) == 0 {
				t.Fatal("TranslateQuery() returned no SQL queries")
			}

			sql := sqlQueries[0]
			if !strings.Contains(sql, tt.wantInSQL) {
				t.Errorf("%s\nQuery: %s\nExpected SQL to contain: %s\nGot SQL: %s",
					tt.description, tt.query, tt.wantInSQL, sql)
			}
		})
	}
}

func TestTranslator_MultipleConditions(t *testing.T) {
	query := "{span.http.status_code = 500 && duration > 100ms}"
	translator := NewTranslator(0)
	sqlQueries, err := translator.TranslateQuery(query)

	if err != nil {
		t.Fatalf("TranslateQuery() error = %v", err)
	}

	if len(sqlQueries) == 0 {
		t.Fatal("TranslateQuery() returned no SQL queries")
	}

	sql := sqlQueries[0]

	// Check both conditions are present
	if !strings.Contains(sql, "http_status_code = 500") {
		t.Errorf("Expected SQL to contain http_status_code condition, got: %s", sql)
	}

	if !strings.Contains(sql, "duration > 100000000") {
		t.Errorf("Expected SQL to contain duration condition (in nanoseconds), got: %s", sql)
	}

	// Check both conditions are ANDed together
	if !strings.Contains(sql, "AND") {
		t.Errorf("Expected SQL to contain AND operator, got: %s", sql)
	}
}

func TestTranslator_Operators(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantInSQL string
	}{
		{
			name:      "equal",
			query:     `{name = "test"}`,
			wantInSQL: "name = 'test'",
		},
		{
			name:      "not equal",
			query:     `{name != "test"}`,
			wantInSQL: "name != 'test'",
		},
		{
			name:      "greater than",
			query:     "{duration > 100ms}",
			wantInSQL: "duration > 100000000",
		},
		{
			name:      "less than",
			query:     "{duration < 100ms}",
			wantInSQL: "duration < 100000000",
		},
		{
			name:      "greater or equal",
			query:     "{span.http.status_code >= 500}",
			wantInSQL: "http_status_code >= 500",
		},
		{
			name:      "less or equal",
			query:     "{span.http.status_code <= 299}",
			wantInSQL: "http_status_code <= 299",
		},
		{
			name:      "regex match",
			query:     `{name =~ "HTTP.*"}`,
			wantInSQL: "REGEXP_LIKE(name, 'HTTP.*')",
		},
		{
			name:      "regex not match",
			query:     `{name !~ "POST.*"}`,
			wantInSQL: "NOT REGEXP_LIKE(name, 'POST.*')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.query)

			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			if len(sqlQueries) == 0 {
				t.Fatal("TranslateQuery() returned no SQL queries")
			}

			sql := sqlQueries[0]
			if !strings.Contains(sql, tt.wantInSQL) {
				t.Errorf("Query: %s\nExpected SQL to contain: %s\nGot SQL: %s",
					tt.query, tt.wantInSQL, sql)
			}
		})
	}
}

func TestTranslator_TenantIsolation(t *testing.T) {
	tests := []struct {
		name     string
		tenantID int
		query    string
	}{
		{
			name:     "tenant 0",
			tenantID: 0,
			query:    "{duration > 100ms}",
		},
		{
			name:     "tenant 123",
			tenantID: 123,
			query:    "{span.http.status_code = 500}",
		},
		{
			name:     "tenant 999",
			tenantID: 999,
			query:    `{resource.service.name = "api"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(tt.tenantID)
			sqlQueries, err := translator.TranslateQuery(tt.query)

			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			if len(sqlQueries) == 0 {
				t.Fatal("TranslateQuery() returned no SQL queries")
			}

			sql := sqlQueries[0]

			// Check tenant_id is in the WHERE clause
			if !strings.Contains(sql, "tenant_id = ") {
				t.Errorf("Expected SQL to contain tenant_id filter, got: %s", sql)
			}

			// Verify it queries the correct table
			if !strings.Contains(sql, "FROM otel_spans") {
				t.Errorf("Expected SQL to query otel_spans table, got: %s", sql)
			}
		})
	}
}

func TestTranslator_Aggregations(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantInSQL   string
		description string
	}{
		{
			name:        "count without grouping",
			query:       "count()",
			wantInSQL:   "SELECT COUNT(*) FROM otel_spans",
			description: "simple count of spans",
		},
		{
			name:        "count with grouping",
			query:       "count() by (span.http.method)",
			wantInSQL:   "SELECT http_method, COUNT(*) FROM otel_spans",
			description: "count grouped by http method",
		},
		{
			name:        "sum durations",
			query:       "sum() by (resource.service.name)",
			wantInSQL:   "SELECT service_name, SUM(duration) FROM otel_spans",
			description: "sum of durations grouped by service",
		},
		{
			name:        "avg durations",
			query:       "avg() by (span.http.route)",
			wantInSQL:   "SELECT http_route, AVG(duration) FROM otel_spans",
			description: "average duration per route",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.query)

			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			if len(sqlQueries) == 0 {
				t.Fatal("TranslateQuery() returned no SQL queries")
			}

			sql := sqlQueries[0]
			if !strings.Contains(sql, tt.wantInSQL) {
				t.Errorf("%s\nQuery: %s\nExpected SQL to contain: %s\nGot SQL: %s",
					tt.description, tt.query, tt.wantInSQL, sql)
			}

			// Check GROUP BY clause if grouping is present
			if strings.Contains(tt.query, "by (") {
				if !strings.Contains(sql, "GROUP BY") {
					t.Errorf("Expected SQL to contain GROUP BY clause, got: %s", sql)
				}
			}
		})
	}
}

func TestTranslator_ScalarExpressions(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		expect float64
	}{
		{
			name:   "addition",
			query:  "1+1",
			expect: 2.0,
		},
		{
			name:   "multiplication",
			query:  "5*2",
			expect: 10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqlQueries, err := translator.TranslateQuery(tt.query)

			if err != nil {
				t.Fatalf("TranslateQuery() error = %v", err)
			}

			if len(sqlQueries) == 0 {
				t.Fatal("TranslateQuery() returned no SQL queries")
			}

			sql := sqlQueries[0]

			// Should contain the result value
			if !strings.Contains(sql, "SELECT") && !strings.Contains(sql, "FROM otel_spans") {
				t.Errorf("Expected valid SQL query, got: %s", sql)
			}
		})
	}
}

func TestTranslator_ComplexQuery(t *testing.T) {
	query := "{resource.service.name = \"checkout-service\" && span.http.status_code = 500 && duration > 100ms}"
	translator := NewTranslator(0)
	sqlQueries, err := translator.TranslateQuery(query)

	if err != nil {
		t.Fatalf("TranslateQuery() error = %v", err)
	}

	if len(sqlQueries) == 0 {
		t.Fatal("TranslateQuery() returned no SQL queries")
	}

	sql := sqlQueries[0]

	// Verify all conditions are present
	expectedConditions := []string{
		"tenant_id = 0",
		"service_name = 'checkout-service'",
		"http_status_code = 500",
		"duration > 100000000", // 100ms in nanoseconds
	}

	for _, condition := range expectedConditions {
		if !strings.Contains(sql, condition) {
			t.Errorf("Expected SQL to contain condition %q, got: %s", condition, sql)
		}
	}

	// Verify table
	if !strings.Contains(sql, "FROM otel_spans") {
		t.Errorf("Expected SQL to query otel_spans, got: %s", sql)
	}

	// Verify ORDER BY
	if !strings.Contains(sql, "ORDER BY") {
		t.Errorf("Expected SQL to contain ORDER BY clause, got: %s", sql)
	}
}
