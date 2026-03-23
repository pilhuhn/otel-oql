package oql

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_BasicSignalDeclaration(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected SignalType
	}{
		{
			name:     "metrics signal",
			query:    "signal=metrics",
			expected: SignalMetrics,
		},
		{
			name:     "logs signal",
			query:    "signal=logs",
			expected: SignalLogs,
		},
		{
			name:     "spans signal",
			query:    "signal=spans",
			expected: SignalSpans,
		},
		{
			name:     "traces signal",
			query:    "signal=traces",
			expected: SignalTraces,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			result, err := parser.Parse()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Signal)
			assert.Empty(t, result.Operations)
		})
	}
}

func TestParser_InvalidSignal(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "missing signal declaration",
			query: "where name == \"test\"",
		},
		{
			name:  "invalid signal type",
			query: "signal=invalid",
		},
		{
			name:  "empty signal",
			query: "signal=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			_, err := parser.Parse()
			assert.Error(t, err)
		})
	}
}

func TestParser_WhereOperation(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedField string
		expectedOp    string
		expectedValue interface{}
	}{
		{
			name:          "string equality",
			query:         `signal=spans | where name == "test"`,
			expectedField: "name",
			expectedOp:    "==",
			expectedValue: "test",
		},
		{
			name:          "integer comparison",
			query:         "signal=spans | where http_status_code == 500",
			expectedField: "http_status_code",
			expectedOp:    "==",
			expectedValue: int64(500),
		},
		{
			name:          "greater than",
			query:         "signal=metrics | where value > 100",
			expectedField: "value",
			expectedOp:    ">",
			expectedValue: int64(100),
		},
		{
			name:          "less than or equal",
			query:         "signal=spans | where duration <= 500ms",
			expectedField: "duration",
			expectedOp:    "<=",
			expectedValue: 500 * time.Millisecond,
		},
		{
			name:          "not equal",
			query:         `signal=logs | where severity != "INFO"`,
			expectedField: "severity",
			expectedOp:    "!=",
			expectedValue: "INFO",
		},
		{
			name:          "boolean value",
			query:         "signal=spans | where error == true",
			expectedField: "error",
			expectedOp:    "==",
			expectedValue: true,
		},
		{
			name:          "float value",
			query:         "signal=metrics | where value >= 99.5",
			expectedField: "value",
			expectedOp:    ">=",
			expectedValue: 99.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			result, err := parser.Parse()
			require.NoError(t, err)
			require.Len(t, result.Operations, 1)

			whereOp, ok := result.Operations[0].(*WhereOp)
			require.True(t, ok, "expected WhereOp")

			binCond, ok := whereOp.Condition.(*BinaryCondition)
			require.True(t, ok, "expected BinaryCondition")

			assert.Equal(t, tt.expectedField, binCond.Left)
			assert.Equal(t, tt.expectedOp, binCond.Operator)
			assert.Equal(t, tt.expectedValue, binCond.Right)
		})
	}
}

func TestParser_DurationParsing(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected time.Duration
	}{
		{
			name:     "milliseconds",
			query:    "signal=spans | where duration == 500ms",
			expected: 500 * time.Millisecond,
		},
		{
			name:     "seconds",
			query:    "signal=spans | where duration == 2s",
			expected: 2 * time.Second,
		},
		{
			name:     "minutes",
			query:    "signal=spans | where duration == 5m",
			expected: 5 * time.Minute,
		},
		{
			name:     "hours",
			query:    "signal=spans | where duration == 1h",
			expected: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			result, err := parser.Parse()
			require.NoError(t, err)

			whereOp := result.Operations[0].(*WhereOp)
			binCond := whereOp.Condition.(*BinaryCondition)

			assert.Equal(t, tt.expected, binCond.Right)
		})
	}
}

func TestParser_AndConditions(t *testing.T) {
	query := `signal=spans | where http_status_code >= 500 and duration > 1s`

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)
	require.Len(t, result.Operations, 1)

	whereOp := result.Operations[0].(*WhereOp)
	andCond, ok := whereOp.Condition.(*AndCondition)
	require.True(t, ok, "expected AndCondition")
	require.Len(t, andCond.Conditions, 2)

	// First condition
	cond1 := andCond.Conditions[0].(*BinaryCondition)
	assert.Equal(t, "http_status_code", cond1.Left)
	assert.Equal(t, ">=", cond1.Operator)
	assert.Equal(t, int64(500), cond1.Right)

	// Second condition
	cond2 := andCond.Conditions[1].(*BinaryCondition)
	assert.Equal(t, "duration", cond2.Left)
	assert.Equal(t, ">", cond2.Operator)
	assert.Equal(t, 1*time.Second, cond2.Right)
}

func TestParser_OrConditions(t *testing.T) {
	query := `signal=logs | where severity == "ERROR" or severity == "FATAL"`

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)
	require.Len(t, result.Operations, 1)

	whereOp := result.Operations[0].(*WhereOp)
	orCond, ok := whereOp.Condition.(*OrCondition)
	require.True(t, ok, "expected OrCondition")
	require.Len(t, orCond.Conditions, 2)

	// First condition
	cond1 := orCond.Conditions[0].(*BinaryCondition)
	assert.Equal(t, "severity", cond1.Left)
	assert.Equal(t, "==", cond1.Operator)
	assert.Equal(t, "ERROR", cond1.Right)

	// Second condition
	cond2 := orCond.Conditions[1].(*BinaryCondition)
	assert.Equal(t, "severity", cond2.Left)
	assert.Equal(t, "==", cond2.Operator)
	assert.Equal(t, "FATAL", cond2.Right)
}

func TestParser_LimitOperation(t *testing.T) {
	query := "signal=spans | where tenant_id == 0 | limit 10"

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)
	require.Len(t, result.Operations, 2)

	limitOp, ok := result.Operations[1].(*LimitOp)
	require.True(t, ok, "expected LimitOp")
	assert.Equal(t, 10, limitOp.Count)
}

func TestParser_ExpandOperation(t *testing.T) {
	query := "signal=spans | where name == \"parent\" | expand trace"

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)
	require.Len(t, result.Operations, 2)

	expandOp, ok := result.Operations[1].(*ExpandOp)
	require.True(t, ok, "expected ExpandOp")
	assert.Equal(t, "trace", expandOp.Type)
}

func TestParser_ExpandInvalidType(t *testing.T) {
	query := "signal=spans | expand invalid"

	parser := NewParser(query)
	_, err := parser.Parse()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expand only supports 'trace'")
}

func TestParser_CorrelateOperation(t *testing.T) {
	tests := []struct {
		name            string
		query           string
		expectedSignals []SignalType
	}{
		{
			name:            "correlate single signal",
			query:           "signal=spans | where trace_id == \"123\" | correlate logs",
			expectedSignals: []SignalType{SignalLogs},
		},
		{
			name:            "correlate multiple signals",
			query:           "signal=metrics | correlate spans, logs",
			expectedSignals: []SignalType{SignalSpans, SignalLogs},
		},
		{
			name:            "correlate with spaces",
			query:           "signal=logs | correlate spans , metrics",
			expectedSignals: []SignalType{SignalSpans, SignalMetrics},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			result, err := parser.Parse()
			require.NoError(t, err)

			var correlateOp *CorrelateOp
			for _, op := range result.Operations {
				if c, ok := op.(*CorrelateOp); ok {
					correlateOp = c
					break
				}
			}

			require.NotNil(t, correlateOp, "expected CorrelateOp")
			assert.Equal(t, tt.expectedSignals, correlateOp.Signals)
		})
	}
}

func TestParser_GetExemplarsOperation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "get_exemplars without parentheses",
			query: "signal=metrics | get_exemplars",
		},
		{
			name:  "get_exemplars with empty parentheses",
			query: "signal=metrics | get_exemplars()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			result, err := parser.Parse()
			require.NoError(t, err)
			require.Len(t, result.Operations, 1)

			_, ok := result.Operations[0].(*GetExemplarsOp)
			require.True(t, ok, "expected GetExemplarsOp")
		})
	}
}

func TestParser_SwitchContextOperation(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedSignal SignalType
	}{
		{
			name:           "switch to spans",
			query:          "signal=metrics | get_exemplars() | switch_context signal=spans",
			expectedSignal: SignalSpans,
		},
		{
			name:           "switch to logs",
			query:          "signal=spans | switch_context signal=logs",
			expectedSignal: SignalLogs,
		},
		{
			name:           "switch to metrics",
			query:          "signal=logs | switch_context signal=metrics",
			expectedSignal: SignalMetrics,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			result, err := parser.Parse()
			require.NoError(t, err)

			var switchOp *SwitchContextOp
			for _, op := range result.Operations {
				if s, ok := op.(*SwitchContextOp); ok {
					switchOp = s
					break
				}
			}

			require.NotNil(t, switchOp, "expected SwitchContextOp")
			assert.Equal(t, tt.expectedSignal, switchOp.Signal)
		})
	}
}

func TestParser_SwitchContextMissingSignal(t *testing.T) {
	query := "signal=metrics | switch_context"

	parser := NewParser(query)
	_, err := parser.Parse()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "switch_context requires 'signal=' parameter")
}

func TestParser_ExtractOperation(t *testing.T) {
	query := "signal=spans | extract trace_id as tid"

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)
	require.Len(t, result.Operations, 1)

	extractOp, ok := result.Operations[0].(*ExtractOp)
	require.True(t, ok, "expected ExtractOp")
	assert.Equal(t, "trace_id", extractOp.Field)
	assert.Equal(t, "tid", extractOp.Alias)
}

func TestParser_ExtractInvalid(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "missing field",
			query: "signal=spans | extract",
		},
		{
			name:  "missing as keyword",
			query: "signal=spans | extract trace_id tid",
		},
		{
			name:  "missing alias",
			query: "signal=spans | extract trace_id as",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			_, err := parser.Parse()
			assert.Error(t, err)
		})
	}
}

func TestParser_FilterOperation(t *testing.T) {
	query := "signal=spans | where tenant_id == 0 | filter http_status_code >= 500"

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)
	require.Len(t, result.Operations, 2)

	filterOp, ok := result.Operations[1].(*FilterOp)
	require.True(t, ok, "expected FilterOp")

	binCond := filterOp.Condition.(*BinaryCondition)
	assert.Equal(t, "http_status_code", binCond.Left)
	assert.Equal(t, ">=", binCond.Operator)
	assert.Equal(t, int64(500), binCond.Right)
}

func TestParser_ComplexQuery(t *testing.T) {
	query := `signal=spans | where tenant_id == 0 and http_status_code >= 500 | expand trace | limit 100`

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)

	assert.Equal(t, SignalSpans, result.Signal)
	require.Len(t, result.Operations, 3)

	// First operation: where with AND condition
	whereOp, ok := result.Operations[0].(*WhereOp)
	require.True(t, ok)
	andCond, ok := whereOp.Condition.(*AndCondition)
	require.True(t, ok)
	assert.Len(t, andCond.Conditions, 2)

	// Second operation: expand
	expandOp, ok := result.Operations[1].(*ExpandOp)
	require.True(t, ok)
	assert.Equal(t, "trace", expandOp.Type)

	// Third operation: limit
	limitOp, ok := result.Operations[2].(*LimitOp)
	require.True(t, ok)
	assert.Equal(t, 100, limitOp.Count)
}

func TestParser_MetricToTraceWormhole(t *testing.T) {
	query := `signal=metrics | where metric_name == "http.server.duration" | get_exemplars() | switch_context signal=spans | where trace_id == {exemplar}`

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)

	assert.Equal(t, SignalMetrics, result.Signal)
	require.Len(t, result.Operations, 4)

	// Verify operation types
	_, ok := result.Operations[0].(*WhereOp)
	assert.True(t, ok, "expected WhereOp")

	_, ok = result.Operations[1].(*GetExemplarsOp)
	assert.True(t, ok, "expected GetExemplarsOp")

	switchOp, ok := result.Operations[2].(*SwitchContextOp)
	assert.True(t, ok, "expected SwitchContextOp")
	assert.Equal(t, SignalSpans, switchOp.Signal)

	_, ok = result.Operations[3].(*WhereOp)
	assert.True(t, ok, "expected WhereOp for trace_id")
}

func TestParser_LogToSpanCorrelation(t *testing.T) {
	query := `signal=logs | where trace_id == "test-trace-456" | correlate spans`

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)

	assert.Equal(t, SignalLogs, result.Signal)
	require.Len(t, result.Operations, 2)

	correlateOp, ok := result.Operations[1].(*CorrelateOp)
	require.True(t, ok)
	assert.Equal(t, []SignalType{SignalSpans}, correlateOp.Signals)
}

func TestParser_UnknownOperation(t *testing.T) {
	query := "signal=spans | unknown_op"

	parser := NewParser(query)
	_, err := parser.Parse()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown operation")
}

func TestParser_InvalidCondition(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "no operator",
			query: "signal=spans | where name test",
		},
		{
			name:  "incomplete condition",
			query: "signal=spans | where name ==",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			_, err := parser.Parse()
			assert.Error(t, err)
		})
	}
}

func TestParser_InvalidLimit(t *testing.T) {
	query := "signal=spans | limit abc"

	parser := NewParser(query)
	_, err := parser.Parse()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid limit count")
}

func TestParser_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "extra spaces",
			query: "signal=spans  |  where  name  ==  \"test\"  |  limit  10",
		},
		{
			name:  "tabs and newlines",
			query: "signal=spans\t|\twhere\tname\t==\t\"test\"\n|\nlimit\n10",
		},
		{
			name:  "minimal spaces",
			query: "signal=spans|where name==\"test\"|limit 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.query)
			result, err := parser.Parse()
			require.NoError(t, err)
			assert.Equal(t, SignalSpans, result.Signal)
			assert.Len(t, result.Operations, 2)
		})
	}
}

func TestParser_AttributeDotNotation(t *testing.T) {
	query := `signal=spans | where attributes.custom_field == "value"`

	parser := NewParser(query)
	result, err := parser.Parse()
	require.NoError(t, err)

	whereOp := result.Operations[0].(*WhereOp)
	binCond := whereOp.Condition.(*BinaryCondition)
	assert.Equal(t, "attributes.custom_field", binCond.Left)
	assert.Equal(t, "value", binCond.Right)
}
