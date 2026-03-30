package traceql

import (
	"testing"
	"time"
)

func TestParser_SimpleSpanFilter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "intrinsic field duration",
			input:   "{duration > 100ms}",
			wantErr: false,
		},
		{
			name:    "intrinsic field name",
			input:   `{name = "HTTP GET"}`,
			wantErr: false,
		},
		{
			name:    "intrinsic field status",
			input:   "{status = error}",
			wantErr: false,
		},
		{
			name:    "span attribute",
			input:   "{span.http.status_code = 500}",
			wantErr: false,
		},
		{
			name:    "resource attribute",
			input:   `{resource.service.name = "api"}`,
			wantErr: false,
		},
		{
			name:    "multiple conditions",
			input:   "{span.http.status_code = 500 && duration > 100ms}",
			wantErr: false,
		},
		{
			name:    "or condition",
			input:   "{status = error || span.http.status_code >= 500}",
			wantErr: false,
		},
		{
			name:    "regex match",
			input:   `{name =~ "HTTP.*"}`,
			wantErr: false,
		},
		{
			name:    "regex not match",
			input:   `{name !~ "GET.*"}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && query == nil {
				t.Error("Parse() returned nil query")
			}
		})
	}
}

func TestParser_SpanFilterConditions(t *testing.T) {
	input := "{span.http.status_code = 500 && duration > 100ms}"
	parser := NewParser(input)
	query, err := parser.Parse()

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	spanFilter, ok := query.Expr.(*SpanFilterExpr)
	if !ok {
		t.Fatalf("expected SpanFilterExpr, got %T", query.Expr)
	}

	if len(spanFilter.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(spanFilter.Conditions))
	}

	// Check first condition: span.http.status_code = 500
	cond1 := spanFilter.Conditions[0]
	if cond1.Field.Type != "span" {
		t.Errorf("condition 1: expected field type 'span', got %q", cond1.Field.Type)
	}
	if cond1.Field.Name != "http.status_code" {
		t.Errorf("condition 1: expected field name 'http.status_code', got %q", cond1.Field.Name)
	}
	if cond1.Operator != "=" {
		t.Errorf("condition 1: expected operator '=', got %q", cond1.Operator)
	}
	if cond1.Value != float64(500) {
		t.Errorf("condition 1: expected value 500, got %v", cond1.Value)
	}

	// Check second condition: duration > 100ms
	cond2 := spanFilter.Conditions[1]
	if cond2.Field.Type != "intrinsic" {
		t.Errorf("condition 2: expected field type 'intrinsic', got %q", cond2.Field.Type)
	}
	if cond2.Field.Name != "duration" {
		t.Errorf("condition 2: expected field name 'duration', got %q", cond2.Field.Name)
	}
	if cond2.Operator != ">" {
		t.Errorf("condition 2: expected operator '>', got %q", cond2.Operator)
	}
	durationVal, ok := cond2.Value.(time.Duration)
	if !ok {
		t.Errorf("condition 2: expected time.Duration value, got %T", cond2.Value)
	}
	if durationVal != 100*time.Millisecond {
		t.Errorf("condition 2: expected value 100ms, got %v", durationVal)
	}
}

func TestParser_Aggregation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "count without grouping",
			input:   "count()",
			wantErr: false,
		},
		{
			name:    "count with grouping",
			input:   "count() by (span.service.name)",
			wantErr: false,
		},
		{
			name:    "sum with grouping",
			input:   "sum() by (resource.environment)",
			wantErr: false,
		},
		{
			name:    "avg with multiple groups",
			input:   "avg() by (span.http.method, resource.service.name)",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if query == nil {
					t.Error("Parse() returned nil query")
					return
				}

				_, ok := query.Expr.(*AggregateExpr)
				if !ok {
					t.Errorf("expected AggregateExpr, got %T", query.Expr)
				}
			}
		})
	}
}

func TestParser_AggregationGrouping(t *testing.T) {
	input := "count() by (span.http.method, resource.environment)"
	parser := NewParser(input)
	query, err := parser.Parse()

	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	aggExpr, ok := query.Expr.(*AggregateExpr)
	if !ok {
		t.Fatalf("expected AggregateExpr, got %T", query.Expr)
	}

	if aggExpr.Function != "count" {
		t.Errorf("expected function 'count', got %q", aggExpr.Function)
	}

	if len(aggExpr.Grouping) != 2 {
		t.Fatalf("expected 2 grouping fields, got %d", len(aggExpr.Grouping))
	}

	if aggExpr.Grouping[0] != "span.http.method" {
		t.Errorf("grouping[0]: expected 'span.http.method', got %q", aggExpr.Grouping[0])
	}

	if aggExpr.Grouping[1] != "resource.environment" {
		t.Errorf("grouping[1]: expected 'resource.environment', got %q", aggExpr.Grouping[1])
	}
}

func TestParser_ScalarExpressions(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect float64
	}{
		{
			name:   "simple addition",
			input:  "1+1",
			expect: 2.0,
		},
		{
			name:   "simple subtraction",
			input:  "5-3",
			expect: 2.0,
		},
		{
			name:   "simple multiplication",
			input:  "3*4",
			expect: 12.0,
		},
		{
			name:   "simple division",
			input:  "10/2",
			expect: 5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()

			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			scalarExpr, ok := query.Expr.(*ScalarExpr)
			if !ok {
				t.Fatalf("expected ScalarExpr, got %T", query.Expr)
			}

			if scalarExpr.Value != tt.expect {
				t.Errorf("expected value %f, got %f", tt.expect, scalarExpr.Value)
			}
		})
	}
}

func TestParser_ErrorCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "missing opening brace",
			input: "duration > 100ms}",
		},
		{
			name:  "missing closing brace",
			input: "{duration > 100ms",
		},
		{
			name:  "invalid operator",
			input: "{duration ? 100ms}",
		},
		{
			name:  "unterminated string",
			input: `{name = "HTTP GET}`,
		},
		{
			name:  "single ampersand",
			input: "{duration > 100ms & status = error}",
		},
		{
			name:  "single pipe",
			input: "{duration > 100ms | status = error}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			_, err := parser.Parse()

			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
