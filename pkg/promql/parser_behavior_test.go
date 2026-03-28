package promql

import (
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
)

// TestPrometheusParserBehavior documents what the Prometheus parser accepts
// This helps us understand edge cases and what we need to handle
func TestPrometheusParserBehavior(t *testing.T) {
	tests := []struct {
		name      string
		promql    string
		wantType  string
		wantError bool
	}{
		{
			name:      "nested aggregations",
			promql:    "sum(avg(http_requests_total))",
			wantType:  "*parser.AggregateExpr",
			wantError: false, // Parser accepts it!
		},
		{
			name:      "offset modifier",
			promql:    "http_requests_total offset 5m",
			wantType:  "*parser.VectorSelector",
			wantError: false, // Parser accepts it!
		},
		{
			name:      "subquery",
			promql:    "rate(http_requests_total[5m:1m])",
			wantType:  "*parser.Call",
			wantError: false, // Parser accepts it!
		},
		{
			name:      "binary operation",
			promql:    "metric1 / metric2",
			wantType:  "*parser.BinaryExpr",
			wantError: false,
		},
		{
			name:      "invalid syntax",
			promql:    "http_requests_total{",
			wantType:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.promql)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected parse error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected parse error: %v", err)
				return
			}

			gotType := ""
			if expr != nil {
				gotType = getTypeName(expr)
			}

			if gotType != tt.wantType {
				t.Logf("Parser accepted query: %s", tt.promql)
				t.Logf("Got type: %s, want: %s", gotType, tt.wantType)
			}
		})
	}
}

// TestNestedAggregations tests that nested aggregations are properly detected
func TestNestedAggregations(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name    string
		promql  string
		wantErr bool
	}{
		{
			name:    "simple aggregation",
			promql:  "sum(http_requests_total)",
			wantErr: false,
		},
		{
			name:    "nested aggregation",
			promql:  "sum(avg(http_requests_total))",
			wantErr: true, // Our translator should reject this
		},
		{
			name:    "aggregation of rate",
			promql:  "sum(rate(http_requests_total[5m]))",
			wantErr: false, // This is OK - rate returns a vector
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := translator.TranslateQuery(tt.promql)
			if (err != nil) != tt.wantErr {
				t.Errorf("TranslateQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestOffsetModifier tests offset modifier handling
func TestOffsetModifier(t *testing.T) {
	translator := NewTranslator(0)

	// Parse the query first to see what we get
	expr, err := parser.ParseExpr("http_requests_total offset 5m")
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Check if it's a VectorSelector with Offset
	vs, ok := expr.(*parser.VectorSelector)
	if !ok {
		t.Fatalf("Expected *parser.VectorSelector, got %T", expr)
	}

	// The offset is stored in vs.OriginalOffset
	t.Logf("VectorSelector with offset: %v", vs.OriginalOffset)

	// Our translator should handle or reject this
	_, err = translator.TranslateQuery("http_requests_total offset 5m")
	// For now, we don't support offset, so we expect either:
	// 1. An error saying offset is not supported
	// 2. Or it's ignored (which might be a bug)

	if err != nil {
		t.Logf("Offset queries rejected with: %v", err)
	} else {
		t.Logf("Offset queries accepted (offset might be ignored)")
	}
}

// TestSubqueryDetection tests subquery handling
func TestSubqueryDetection(t *testing.T) {
	translator := NewTranslator(0)

	tests := []struct {
		name    string
		promql  string
		wantErr bool
	}{
		{
			name:    "regular range query",
			promql:  "rate(http_requests_total[5m])",
			wantErr: false,
		},
		{
			name:    "subquery syntax",
			promql:  "rate(http_requests_total[5m:1m])",
			wantErr: true, // Should be detected as subquery
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := translator.TranslateQuery(tt.promql)
			if (err != nil) != tt.wantErr {
				t.Errorf("TranslateQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// getTypeName returns the type name as a string for comparison
func getTypeName(expr parser.Expr) string {
	switch expr.(type) {
	case *parser.AggregateExpr:
		return "*parser.AggregateExpr"
	case *parser.VectorSelector:
		return "*parser.VectorSelector"
	case *parser.MatrixSelector:
		return "*parser.MatrixSelector"
	case *parser.BinaryExpr:
		return "*parser.BinaryExpr"
	case *parser.Call:
		return "*parser.Call"
	case *parser.SubqueryExpr:
		return "*parser.SubqueryExpr"
	case *parser.NumberLiteral:
		return "*parser.NumberLiteral"
	case *parser.StringLiteral:
		return "*parser.StringLiteral"
	case *parser.ParenExpr:
		return "*parser.ParenExpr"
	case *parser.UnaryExpr:
		return "*parser.UnaryExpr"
	default:
		return "unknown"
	}
}
