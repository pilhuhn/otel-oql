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
		name          string
		signal        oql.SignalType
		tenantID      int
		expectedTable string
	}{
		{
			name:          "spans signal",
			signal:        oql.SignalSpans,
			tenantID:      0,
			expectedTable: "otel_spans",
		},
		{
			name:          "metrics signal",
			signal:        oql.SignalMetrics,
			tenantID:      1,
			expectedTable: "otel_metrics",
		},
		{
			name:          "logs signal",
			signal:        oql.SignalLogs,
			tenantID:      2,
			expectedTable: "otel_logs",
		},
		{
			name:          "traces signal",
			signal:        oql.SignalTraces,
			tenantID:      3,
			expectedTable: "otel_spans",
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
