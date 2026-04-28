package translator

import (
	"fmt"
	"testing"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/oql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslator_BasicQueryTranslation(t *testing.T) {
	tests := []struct {
		name               string
		signal             oql.SignalType
		tenantID           int
		expectedTable      string
		expectOrdering     bool
		expectRootFilter   bool
	}{
		{
			name:             "spans signal",
			signal:           oql.SignalSpans,
			tenantID:         0,
			expectedTable:    "otel_spans",
			expectOrdering:   true,
			expectRootFilter: false,
		},
		{
			name:             "metrics signal",
			signal:           oql.SignalMetrics,
			tenantID:         1,
			expectedTable:    "otel_metrics",
			expectOrdering:   false,
			expectRootFilter: false,
		},
		{
			name:             "logs signal",
			signal:           oql.SignalLogs,
			tenantID:         2,
			expectedTable:    "otel_logs",
			expectOrdering:   true,
			expectRootFilter: false,
		},
		{
			name:             "traces signal",
			signal:           oql.SignalTraces,
			tenantID:         3,
			expectedTable:    "otel_spans",
			expectOrdering:   true,
			expectRootFilter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(tt.tenantID)
			query := &oql.Query{
				Signal:     tt.signal,
				Operations: []oql.Operation{},
			}

			sqls, err := translator.TranslateQuery(query)
			require.NoError(t, err)
			require.Len(t, sqls, 1)

			expectedSQL := fmt.Sprintf("SELECT * FROM %s WHERE tenant_id = %d", tt.expectedTable, tt.tenantID)
			if tt.expectRootFilter {
				expectedSQL += " AND (parent_span_id IS NULL OR parent_span_id = '' OR parent_span_id = '0' OR parent_span_id = '00000000000000000000000000000000')"
			}
			if tt.expectOrdering {
				expectedSQL += " ORDER BY \"timestamp\" DESC"
			}
			assert.Equal(t, expectedSQL, sqls[0])
		})
	}
}

func TestTranslator_TenantIDInjection(t *testing.T) {
	tests := []struct {
		name     string
		tenantID int
	}{
		{name: "tenant 0", tenantID: 0},
		{name: "tenant 123", tenantID: 123},
		{name: "tenant 9999", tenantID: 9999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(tt.tenantID)
			query := &oql.Query{
				Signal:     oql.SignalSpans,
				Operations: []oql.Operation{},
			}

			sqls, err := translator.TranslateQuery(query)
			require.NoError(t, err)
			require.Len(t, sqls, 1)

			assert.Contains(t, sqls[0], "WHERE tenant_id = ")
			assert.Contains(t, sqls[0], fmt.Sprintf("tenant_id = %d", tt.tenantID))
		})
	}
}

func TestTranslator_NativeColumnDetection(t *testing.T) {
	tests := []struct {
		name           string
		attributeKey   string
		expectedColumn string
	}{
		{name: "http.method", attributeKey: "http.method", expectedColumn: "http_method"},
		{name: "http.status_code", attributeKey: "http.status_code", expectedColumn: "http_status_code"},
		{name: "service.name", attributeKey: "service.name", expectedColumn: "service_name"},
		{name: "db.system", attributeKey: "db.system", expectedColumn: "db_system"},
		{name: "error", attributeKey: "error", expectedColumn: "error"},
		{name: "log.level", attributeKey: "log.level", expectedColumn: "log_level"},
		{name: "environment", attributeKey: "environment", expectedColumn: "environment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			nativeCol := translator.getNativeColumn(tt.attributeKey)
			assert.Equal(t, tt.expectedColumn, nativeCol)
		})
	}
}

func TestTranslator_NonNativeAttributeReturnsEmpty(t *testing.T) {
	translator := NewTranslator(0)
	tests := []string{
		"custom.field",
		"application.version",
		"user.id",
		"request.id",
	}

	for _, attributeKey := range tests {
		t.Run(attributeKey, func(t *testing.T) {
			nativeCol := translator.getNativeColumn(attributeKey)
			assert.Empty(t, nativeCol, "non-native attribute should return empty string")
		})
	}
}

func TestTranslator_WhereWithNativeColumn(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "attributes.http.status_code",
					Operator: "==",
					Right:    int64(500),
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Should use native column, not JSON extraction
	assert.Contains(t, sqls[0], "http_status_code = 500")
	assert.NotContains(t, sqls[0], "JSON_EXTRACT")
}

func TestTranslator_WhereWithJSONExtraction(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "attributes.custom_field",
					Operator: "==",
					Right:    "value",
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Should use JSON extraction for non-native attributes
	assert.Contains(t, sqls[0], "JSON_EXTRACT_SCALAR(attributes, '$.custom_field', 'STRING')")
	assert.Contains(t, sqls[0], "= 'value'")
}

func TestTranslator_WhereWithResourceAttributes(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "resource_attributes.service.name",
					Operator: "==",
					Right:    "my-service",
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// service.name is a native column
	assert.Contains(t, sqls[0], "service_name = 'my-service'")
}

func TestTranslator_WhereWithNonDottedField(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "trace_id",
					Operator: "==",
					Right:    "test-trace-123",
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Should use field name directly
	assert.Contains(t, sqls[0], "trace_id = 'test-trace-123'")
	assert.NotContains(t, sqls[0], "JSON_EXTRACT")
}

func TestTranslator_FieldAliases(t *testing.T) {
	tests := []struct {
		name          string
		aliasField    string
		canonicalField string
		value         string
	}{
		{
			name:          "service alias",
			aliasField:    "service",
			canonicalField: "service_name",
			value:         "payment-service",
		},
		{
			name:          "status alias",
			aliasField:    "status",
			canonicalField: "http_status_code",
			value:         "200",
		},
		{
			name:          "method alias",
			aliasField:    "method",
			canonicalField: "http_method",
			value:         "GET",
		},
		{
			name:          "route alias",
			aliasField:    "route",
			canonicalField: "http_route",
			value:         "/api/v1/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			query := &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     tt.aliasField, // Use alias (what user types)
							Operator: "=",
							Right:    tt.value,
						},
					},
				},
			}

			sqls, err := translator.TranslateQuery(query)
			require.NoError(t, err)
			require.Len(t, sqls, 1)

			// Should translate alias to canonical column name
			expectedCondition := fmt.Sprintf("%s = '%s'", tt.canonicalField, tt.value)
			assert.Contains(t, sqls[0], expectedCondition,
				"Expected alias '%s' to be translated to canonical column '%s'",
				tt.aliasField, tt.canonicalField)
		})
	}
}

func TestTranslator_AndConditions(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.AndCondition{
					Conditions: []oql.Condition{
						&oql.BinaryCondition{
							Left:     "attributes.http.status_code",
							Operator: ">=",
							Right:    int64(500),
						},
						&oql.BinaryCondition{
							Left:     "duration",
							Operator: ">",
							Right:    1 * time.Second,
						},
					},
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	assert.Contains(t, sqls[0], "http_status_code >= 500")
	assert.Contains(t, sqls[0], "duration > 1000000000") // 1 second in nanoseconds
	assert.Contains(t, sqls[0], "AND")
}

func TestTranslator_OrConditions(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalLogs,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.OrCondition{
					Conditions: []oql.Condition{
						&oql.BinaryCondition{
							Left:     "severity",
							Operator: "==",
							Right:    "ERROR",
						},
						&oql.BinaryCondition{
							Left:     "severity",
							Operator: "==",
							Right:    "FATAL",
						},
					},
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	assert.Contains(t, sqls[0], "severity = 'ERROR'")
	assert.Contains(t, sqls[0], "severity = 'FATAL'")
	assert.Contains(t, sqls[0], "OR")
}

func TestTranslator_LimitOperation(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.LimitOp{Count: 100},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	assert.Contains(t, sqls[0], "LIMIT 100")
}

func TestTranslator_ExpandOperation(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "name",
					Operator: "==",
					Right:    "parent",
				},
			},
			&oql.ExpandOp{Type: "trace"},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Expand is emitted as a marker; API server runs the multi-step trace expansion
	assert.Contains(t, sqls[0], "__EXPAND_TRACE__")
	assert.Contains(t, sqls[0], "name = 'parent'")
	assert.Contains(t, sqls[0], "__TABLE__otel_spans__")
}

func TestTranslator_CorrelateOperation(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "trace_id",
					Operator: "==",
					Right:    "test-trace",
				},
			},
			&oql.CorrelateOp{
				Signals: []oql.SignalType{oql.SignalLogs, oql.SignalMetrics},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Correlate is emitted as a marker; API server runs per-signal queries
	assert.Contains(t, sqls[0], "__CORRELATE__")
	assert.Contains(t, sqls[0], "otel_spans")
	assert.Contains(t, sqls[0], "trace_id = 'test-trace'")
	assert.Contains(t, sqls[0], "logs,metrics")
}

func TestTranslator_GetExemplarsOperation(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalMetrics,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "metric_name",
					Operator: "==",
					Right:    "http.server.duration",
				},
			},
			&oql.GetExemplarsOp{},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Should select exemplar fields
	assert.Contains(t, sqls[0], "exemplar_trace_id")
	assert.Contains(t, sqls[0], "exemplar_span_id")
	assert.Contains(t, sqls[0], "exemplar_trace_id IS NOT NULL")
}

func TestTranslator_SwitchContextOperation(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalMetrics,
		Operations: []oql.Operation{
			&oql.GetExemplarsOp{},
			&oql.SwitchContextOp{Signal: oql.SignalSpans},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Should switch to spans table
	assert.Contains(t, sqls[0], "otel_spans")
}

func TestTranslator_ExtractOperation(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.ExtractOp{
				Field: "trace_id",
				Alias: "tid",
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Should replace SELECT * with SELECT field AS alias
	assert.Contains(t, sqls[0], "SELECT trace_id AS tid")
	assert.NotContains(t, sqls[0], "SELECT *")
}

func TestTranslator_FilterOperation(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "tenant_id",
					Operator: "==",
					Right:    int64(0),
				},
			},
			&oql.FilterOp{
				Condition: &oql.BinaryCondition{
					Left:     "attributes.http.status_code",
					Operator: ">=",
					Right:    int64(500),
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Filter should add another AND clause
	assert.Contains(t, sqls[0], "tenant_id = 0")
	assert.Contains(t, sqls[0], "http_status_code >= 500")
}

func TestTranslator_ValueFormatting(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{
			name:     "string value",
			value:    "test",
			expected: "'test'",
		},
		{
			name:     "string with single quote",
			value:    "test's value",
			expected: "'test''s value'",
		},
		{
			name:     "integer value",
			value:    int64(123),
			expected: "123",
		},
		{
			name:     "float value",
			value:    99.5,
			expected: "99.500000",
		},
		{
			name:     "boolean true",
			value:    true,
			expected: "true",
		},
		{
			name:     "boolean false",
			value:    false,
			expected: "false",
		},
		{
			name:     "duration",
			value:    500 * time.Millisecond,
			expected: "500000000", // nanoseconds
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := translator.formatValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTranslator_ComplexQuery(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.AndCondition{
					Conditions: []oql.Condition{
						&oql.BinaryCondition{
							Left:     "tenant_id",
							Operator: "==",
							Right:    int64(0),
						},
						&oql.BinaryCondition{
							Left:     "attributes.http.status_code",
							Operator: ">=",
							Right:    int64(500),
						},
					},
				},
			},
			&oql.ExpandOp{Type: "trace"},
			&oql.LimitOp{Count: 100},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Verify all parts are present
	assert.Contains(t, sqls[0], "otel_spans")
	assert.Contains(t, sqls[0], "tenant_id = 0")
	assert.Contains(t, sqls[0], "http_status_code >= 500")
	assert.Contains(t, sqls[0], "__EXPAND_TRACE__")
	assert.Contains(t, sqls[0], "LIMIT 100")
}

func TestTranslator_MetricToTraceWormhole(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalMetrics,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "metric_name",
					Operator: "==",
					Right:    "http.server.duration",
				},
			},
			&oql.GetExemplarsOp{},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)

	// Should extract exemplar trace IDs
	assert.Contains(t, sqls[0], "otel_metrics")
	assert.Contains(t, sqls[0], "exemplar_trace_id")
	assert.Contains(t, sqls[0], "exemplar_trace_id IS NOT NULL")
}

func TestTranslator_MultiTenantIsolation(t *testing.T) {
	tests := []struct {
		tenantID int
	}{
		{tenantID: 0},
		{tenantID: 1},
		{tenantID: 100},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("tenant_%d", tt.tenantID), func(t *testing.T) {
			translator := NewTranslator(tt.tenantID)
			query := &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "name",
							Operator: "==",
							Right:    "test",
						},
					},
				},
			}

			sqls, err := translator.TranslateQuery(query)
			require.NoError(t, err)
			require.Len(t, sqls, 1)

			// Every query should include tenant_id filter
			assert.Contains(t, sqls[0], "tenant_id = ")
		})
	}
}

func TestTranslator_AllOperators(t *testing.T) {
	translator := NewTranslator(0)

	operators := []string{"==", "!=", ">", "<", ">=", "<="}

	for _, op := range operators {
		t.Run("operator_"+op, func(t *testing.T) {
			query := &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "duration",
							Operator: op,
							Right:    int64(1000),
						},
					},
				},
			}

			sqls, err := translator.TranslateQuery(query)
			require.NoError(t, err)
			require.Len(t, sqls, 1)

			wantOp := op
			if op == "==" {
				wantOp = "="
			} else if op == "!=" {
				wantOp = "<>"
			}
			assert.Contains(t, sqls[0], "duration "+wantOp+" 1000")
		})
	}
}

func TestTranslator_AttributeKeyWithSingleQuoteEscaped(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "attributes.test'key",
					Operator: "==",
					Right:    "v",
				},
			},
		},
	}

	sqls, err := translator.TranslateQuery(query)
	require.NoError(t, err)
	require.Len(t, sqls, 1)
	assert.Contains(t, sqls[0], "JSON_EXTRACT_SCALAR(attributes, '$.test''key', 'STRING')")
	assert.Contains(t, sqls[0], "= 'v'")
}

func TestTranslator_MaliciousFieldExpressionRejected(t *testing.T) {
	translator := NewTranslator(0)
	query := &oql.Query{
		Signal: oql.SignalSpans,
		Operations: []oql.Operation{
			&oql.WhereOp{
				Condition: &oql.BinaryCondition{
					Left:     "trace_id) OR (1=1",
					Operator: "==",
					Right:    "x",
				},
			},
		},
	}

	_, err := translator.TranslateQuery(query)
	require.Error(t, err)
}

func TestTranslator_Sort(t *testing.T) {
	tests := []struct {
		name     string
		query    *oql.Query
		wantSQL  []string
		tenantID int
	}{
		{
			name: "sort single field ascending",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.SortOp{
						Fields: []oql.SortField{{Field: "duration", Desc: false}},
					},
				},
			},
			wantSQL:  []string{"SELECT * FROM otel_spans WHERE tenant_id = 0 ORDER BY duration ASC"},
			tenantID: 0,
		},
		{
			name: "sort single field descending",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.SortOp{
						Fields: []oql.SortField{{Field: "duration", Desc: true}},
					},
				},
			},
			wantSQL:  []string{"SELECT * FROM otel_spans WHERE tenant_id = 0 ORDER BY duration DESC"},
			tenantID: 0,
		},
		{
			name: "sort multiple fields",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.SortOp{
						Fields: []oql.SortField{
							{Field: "duration", Desc: true},
							{Field: "name", Desc: false},
						},
					},
				},
			},
			wantSQL:  []string{"SELECT * FROM otel_spans WHERE tenant_id = 0 ORDER BY duration DESC, name ASC"},
			tenantID: 0,
		},
		{
			name: "sort with where and limit",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{Condition: &oql.BinaryCondition{Left: "duration", Operator: ">", Right: int64(500000000)}},
					&oql.SortOp{Fields: []oql.SortField{{Field: "duration", Desc: true}}},
					&oql.LimitOp{Count: 10},
				},
			},
			wantSQL:  []string{"SELECT * FROM otel_spans WHERE tenant_id = 0 AND duration > 500000000 ORDER BY duration DESC LIMIT 10"},
			tenantID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := NewTranslator(tt.tenantID)
			gotSQL, err := trans.TranslateQuery(tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(gotSQL) != len(tt.wantSQL) {
				t.Fatalf("got %d SQL queries, want %d", len(gotSQL), len(tt.wantSQL))
			}

			for i, want := range tt.wantSQL {
				if gotSQL[i] != want {
					t.Errorf("query %d:\ngot:  %s\nwant: %s", i, gotSQL[i], want)
				}
			}
		})
	}
}

func TestTranslator_WildcardPatternMatching(t *testing.T) {
	tests := []struct {
		name        string
		query       *oql.Query
		wantContain string
		description string
	}{
		{
			name: "contains wildcard",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "body",
							Operator: "=",
							Right:    "*pool*",
						},
					},
				},
			},
			wantContain: "body LIKE '%pool%'",
			description: "wildcards in value should use LIKE with % conversion",
		},
		{
			name: "prefix wildcard",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "body",
							Operator: "=",
							Right:    "GET *",
						},
					},
				},
			},
			wantContain: "body LIKE 'GET %'",
			description: "trailing wildcard should use LIKE",
		},
		{
			name: "or conditions with wildcards",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.OrCondition{
							Conditions: []oql.Condition{
								&oql.BinaryCondition{
									Left:     "body",
									Operator: "=",
									Right:    "*timeout*",
								},
								&oql.BinaryCondition{
									Left:     "body",
									Operator: "=",
									Right:    "*connection*",
								},
							},
						},
					},
				},
			},
			wantContain: "body LIKE '%timeout%' OR body LIKE '%connection%'",
			description: "multiple OR conditions with wildcards",
		},
		{
			name: "no wildcard exact match",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "body",
							Operator: "=",
							Right:    "error",
						},
					},
				},
			},
			wantContain: "body = 'error'",
			description: "no wildcards should use exact match",
		},
		{
			name: "not equals with wildcard",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "body",
							Operator: "!=",
							Right:    "*debug*",
						},
					},
				},
			},
			wantContain: "body NOT LIKE '%debug%'",
			description: "!= with wildcards should use NOT LIKE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.query)
			require.NoError(t, err, tt.description)
			require.Len(t, sqls, 1)
			assert.Contains(t, sqls[0], tt.wantContain, tt.description)
		})
	}
}

func TestTranslator_NowExpression(t *testing.T) {
	tests := []struct {
		name        string
		query       *oql.Query
		wantContain string
		description string
	}{
		{
			name: "timestamp equals now()",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: "==",
							Right:    &oql.NowExpression{},
						},
					},
				},
			},
			wantContain: "\"timestamp\" = now()",
			description: "now() should translate to Pinot now() function",
		},
		{
			name: "timestamp > now()",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: ">",
							Right:    &oql.NowExpression{},
						},
					},
				},
			},
			wantContain: "\"timestamp\" > now()",
			description: "greater than now() comparison",
		},
		{
			name: "timestamp < now()",
			query: &oql.Query{
				Signal: oql.SignalMetrics,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: "<",
							Right:    &oql.NowExpression{},
						},
					},
				},
			},
			wantContain: "\"timestamp\" < now()",
			description: "less than now() comparison",
		},
		{
			name: "timestamp >= now()",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: ">=",
							Right:    &oql.NowExpression{},
						},
					},
				},
			},
			wantContain: "\"timestamp\" >= now()",
			description: "greater than or equal now() comparison",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.query)
			require.NoError(t, err, tt.description)
			require.Len(t, sqls, 1)
			assert.Contains(t, sqls[0], tt.wantContain, tt.description)
		})
	}
}

func TestTranslator_TimeArithmeticExpression(t *testing.T) {
	tests := []struct {
		name        string
		query       *oql.Query
		wantContain string
		description string
	}{
		{
			name: "now() - 1h",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: ">",
							Right: &oql.TimeArithmeticExpression{
								Base:     &oql.NowExpression{},
								Operator: "-",
								Offset:   "1h",
							},
						},
					},
				},
			},
			wantContain: "\"timestamp\" > (now() - 3600000)",
			description: "1 hour = 3600000 milliseconds",
		},
		{
			name: "now() - 30m",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: ">=",
							Right: &oql.TimeArithmeticExpression{
								Base:     &oql.NowExpression{},
								Operator: "-",
								Offset:   "30m",
							},
						},
					},
				},
			},
			wantContain: "\"timestamp\" >= (now() - 1800000)",
			description: "30 minutes = 1800000 milliseconds",
		},
		{
			name: "now() - 5s",
			query: &oql.Query{
				Signal: oql.SignalMetrics,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: ">",
							Right: &oql.TimeArithmeticExpression{
								Base:     &oql.NowExpression{},
								Operator: "-",
								Offset:   "5s",
							},
						},
					},
				},
			},
			wantContain: "\"timestamp\" > (now() - 5000)",
			description: "5 seconds = 5000 milliseconds",
		},
		{
			name: "now() - 100ms",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: ">=",
							Right: &oql.TimeArithmeticExpression{
								Base:     &oql.NowExpression{},
								Operator: "-",
								Offset:   "100ms",
							},
						},
					},
				},
			},
			wantContain: "\"timestamp\" >= (now() - 100)",
			description: "100 milliseconds",
		},
		{
			name: "now() + 1h (future)",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: "<",
							Right: &oql.TimeArithmeticExpression{
								Base:     &oql.NowExpression{},
								Operator: "+",
								Offset:   "1h",
							},
						},
					},
				},
			},
			wantContain: "\"timestamp\" < (now() + 3600000)",
			description: "future time: now() + 1 hour",
		},
		{
			name: "now() + 5m (future)",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "timestamp",
							Operator: "<=",
							Right: &oql.TimeArithmeticExpression{
								Base:     &oql.NowExpression{},
								Operator: "+",
								Offset:   "5m",
							},
						},
					},
				},
			},
			wantContain: "\"timestamp\" <= (now() + 300000)",
			description: "future time: now() + 5 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.query)
			require.NoError(t, err, tt.description)
			require.Len(t, sqls, 1)
			assert.Contains(t, sqls[0], tt.wantContain, tt.description)
		})
	}
}

func TestTranslator_ComplexTimeQueries(t *testing.T) {
	tests := []struct {
		name         string
		query        *oql.Query
		wantContains []string
		description  string
	}{
		{
			name: "time range: now() - 1h to now()",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.AndCondition{
							Conditions: []oql.Condition{
								&oql.BinaryCondition{
									Left:     "timestamp",
									Operator: ">",
									Right: &oql.TimeArithmeticExpression{
										Base:     &oql.NowExpression{},
										Operator: "-",
										Offset:   "1h",
									},
								},
								&oql.BinaryCondition{
									Left:     "timestamp",
									Operator: "<",
									Right:    &oql.NowExpression{},
								},
							},
						},
					},
				},
			},
			wantContains: []string{
				"\"timestamp\" > (now() - 3600000)",
				"\"timestamp\" < now()",
			},
			description: "time range from 1 hour ago to now",
		},
		{
			name: "combined with other conditions",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.AndCondition{
							Conditions: []oql.Condition{
								&oql.BinaryCondition{
									Left:     "name",
									Operator: "==",
									Right:    "checkout",
								},
								&oql.BinaryCondition{
									Left:     "timestamp",
									Operator: ">",
									Right: &oql.TimeArithmeticExpression{
										Base:     &oql.NowExpression{},
										Operator: "-",
										Offset:   "30m",
									},
								},
							},
						},
					},
				},
			},
			wantContains: []string{
				"name = 'checkout'",
				"\"timestamp\" > (now() - 1800000)",
			},
			description: "name filter combined with time range",
		},
		{
			name: "or condition with time",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.OrCondition{
							Conditions: []oql.Condition{
								&oql.BinaryCondition{
									Left:     "log_level",
									Operator: "==",
									Right:    "error",
								},
								&oql.BinaryCondition{
									Left:     "timestamp",
									Operator: ">",
									Right: &oql.TimeArithmeticExpression{
										Base:     &oql.NowExpression{},
										Operator: "-",
										Offset:   "5m",
									},
								},
							},
						},
					},
				},
			},
			wantContains: []string{
				"log_level = 'error'",
				"\"timestamp\" > (now() - 300000)",
				" OR ",
			},
			description: "error logs OR recent logs within 5 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.query)
			require.NoError(t, err, tt.description)
			require.Len(t, sqls, 1)

			for _, want := range tt.wantContains {
				assert.Contains(t, sqls[0], want, tt.description)
			}
		})
	}
}

func TestTranslator_TracesVsSpans(t *testing.T) {
	tests := []struct {
		name             string
		signal           oql.SignalType
		expectRootFilter bool
		expectedSQL      string
		description      string
	}{
		{
			name:             "signal=spans shows all spans",
			signal:           oql.SignalSpans,
			expectRootFilter: false,
			expectedSQL:      "SELECT * FROM otel_spans WHERE tenant_id = 0 AND service_name = 'test-service' ORDER BY \"timestamp\" DESC",
			description:      "spans query should show all spans, not just root spans",
		},
		{
			name:             "signal=trace shows only root spans",
			signal:           oql.SignalTraces,
			expectRootFilter: true,
			expectedSQL:      "SELECT * FROM otel_spans WHERE tenant_id = 0 AND (parent_span_id IS NULL OR parent_span_id = '' OR parent_span_id = '0' OR parent_span_id = '00000000000000000000000000000000') AND service_name = 'test-service' ORDER BY \"timestamp\" DESC",
			description:      "traces query should filter to show only root spans (one per trace)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			query := &oql.Query{
				Signal: tt.signal,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "service_name",
							Operator: "==",
							Right:    "test-service",
						},
					},
				},
			}

			sqls, err := translator.TranslateQuery(query)
			require.NoError(t, err, tt.description)
			require.Len(t, sqls, 1)

			// Verify exact SQL matches
			assert.Equal(t, tt.expectedSQL, sqls[0], tt.description)

			// Also verify specific parts
			if tt.expectRootFilter {
				assert.Contains(t, sqls[0], "parent_span_id IS NULL", tt.description)
			} else {
				assert.NotContains(t, sqls[0], "parent_span_id IS NULL", tt.description)
			}
		})
	}
}

func TestTranslator_DefaultTimestampOrdering(t *testing.T) {
	tests := []struct {
		name        string
		query       *oql.Query
		wantOrdering bool
		description string
	}{
		{
			name: "spans without explicit sort gets default ordering",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "name",
							Operator: "==",
							Right:    "test",
						},
					},
				},
			},
			wantOrdering: true,
			description:  "spans query should have default timestamp DESC ordering",
		},
		{
			name: "logs without explicit sort gets default ordering",
			query: &oql.Query{
				Signal: oql.SignalLogs,
				Operations: []oql.Operation{
					&oql.LimitOp{Count: 100},
				},
			},
			wantOrdering: true,
			description:  "logs query should have default timestamp DESC ordering",
		},
		{
			name: "traces without explicit sort gets default ordering",
			query: &oql.Query{
				Signal: oql.SignalTraces,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "trace_id",
							Operator: "==",
							Right:    "abc123",
						},
					},
				},
			},
			wantOrdering: true,
			description:  "traces query should have default timestamp DESC ordering and root span filter",
		},
		{
			name: "metrics without explicit sort has no default ordering",
			query: &oql.Query{
				Signal: oql.SignalMetrics,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "metric_name",
							Operator: "==",
							Right:    "http.server.duration",
						},
					},
				},
			},
			wantOrdering: false,
			description:  "metrics query should NOT have default ordering",
		},
		{
			name: "spans with explicit sort does not get default ordering",
			query: &oql.Query{
				Signal: oql.SignalSpans,
				Operations: []oql.Operation{
					&oql.WhereOp{
						Condition: &oql.BinaryCondition{
							Left:     "duration",
							Operator: ">",
							Right:    int64(1000),
						},
					},
					&oql.SortOp{
						Fields: []oql.SortField{{Field: "duration", Desc: true}},
					},
				},
			},
			wantOrdering: false,
			description:  "explicit sort should override default ordering",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewTranslator(0)
			sqls, err := translator.TranslateQuery(tt.query)
			require.NoError(t, err, tt.description)
			require.Len(t, sqls, 1)

			// Check for root span filter on traces
			if tt.query.Signal == oql.SignalTraces {
				assert.Contains(t, sqls[0], "parent_span_id IS NULL", tt.description)
			}

			if tt.wantOrdering {
				assert.Contains(t, sqls[0], "ORDER BY \"timestamp\" DESC", tt.description)
			} else {
				// If there's an explicit sort, check for that instead
				if len(tt.query.Operations) > 0 {
					if _, ok := tt.query.Operations[len(tt.query.Operations)-1].(*oql.SortOp); ok {
						assert.Contains(t, sqls[0], "ORDER BY", tt.description)
						assert.NotContains(t, sqls[0], "ORDER BY \"timestamp\" DESC", tt.description)
						return
					}
				}
				// Metrics should have no ORDER BY at all
				assert.NotContains(t, sqls[0], "ORDER BY", tt.description)
			}
		})
	}
}
