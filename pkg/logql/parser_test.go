package logql

import (
	"strings"
	"testing"
	"time"
)

func TestParse_LogRangeQuery(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkExpr func(*testing.T, Expr)
		wantErr   bool
		errMsg    string
	}{
		{
			name:  "simple stream selector",
			input: `{job="varlogs"}`,
			checkExpr: func(t *testing.T, expr Expr) {
				lr, ok := expr.(*LogRangeExpr)
				if !ok {
					t.Errorf("expected LogRangeExpr, got %T", expr)
					return
				}
				if len(lr.StreamSelector.Matchers) != 1 {
					t.Errorf("expected 1 matcher, got %d", len(lr.StreamSelector.Matchers))
				}
				if len(lr.Pipeline) != 0 {
					t.Errorf("expected 0 pipeline stages, got %d", len(lr.Pipeline))
				}
			},
		},
		{
			name:  "stream selector with pipeline",
			input: `{job="varlogs"} |= "error"`,
			checkExpr: func(t *testing.T, expr Expr) {
				lr, ok := expr.(*LogRangeExpr)
				if !ok {
					t.Errorf("expected LogRangeExpr, got %T", expr)
					return
				}
				if len(lr.Pipeline) != 1 {
					t.Errorf("expected 1 pipeline stage, got %d", len(lr.Pipeline))
				}
			},
		},
		{
			name:  "stream selector with multiple pipeline stages",
			input: `{job="varlogs"} |= "error" | json`,
			checkExpr: func(t *testing.T, expr Expr) {
				lr, ok := expr.(*LogRangeExpr)
				if !ok {
					t.Errorf("expected LogRangeExpr, got %T", expr)
					return
				}
				if len(lr.Pipeline) != 2 {
					t.Errorf("expected 2 pipeline stages, got %d", len(lr.Pipeline))
				}
			},
		},
		{
			name:    "invalid stream selector",
			input:   `{invalid}`,
			wantErr: true,
			errMsg:  "failed to parse stream selector",
		},
		{
			name:    "empty stream selector",
			input:   `{}`,
			wantErr: true,
			errMsg:  "parse error", // Prometheus parser error
		},
		{
			name:    "only negative matchers",
			input:   `{job!="test"}`,
			wantErr: true,
			errMsg:  "parse error", // Prometheus parser error
		},
		{
			name:    "invalid pipeline",
			input:   `{job="varlogs"} |> "error"`,
			wantErr: true,
			errMsg:  "unknown pipeline stage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()

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

			if tt.checkExpr != nil {
				tt.checkExpr(t, query.Expr)
			}
		})
	}
}

func TestParse_MetricQuery(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkExpr func(*testing.T, Expr)
		wantErr   bool
		errMsg    string
	}{
		{
			name:  "count_over_time simple",
			input: `count_over_time({job="varlogs"}[5m])`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Function != "count_over_time" {
					t.Errorf("function = %s, want count_over_time", me.Function)
				}
				if me.Range != 5*time.Minute {
					t.Errorf("range = %v, want 5m", me.Range)
				}
				if me.Aggregator != nil {
					t.Errorf("expected no aggregator, got %+v", me.Aggregator)
				}
			},
		},
		{
			name:  "rate function",
			input: `rate({job="varlogs"}[10m])`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Function != "rate" {
					t.Errorf("function = %s, want rate", me.Function)
				}
				if me.Range != 10*time.Minute {
					t.Errorf("range = %v, want 10m", me.Range)
				}
			},
		},
		{
			name:  "bytes_over_time",
			input: `bytes_over_time({job="varlogs"}[1h])`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Function != "bytes_over_time" {
					t.Errorf("function = %s, want bytes_over_time", me.Function)
				}
				if me.Range != time.Hour {
					t.Errorf("range = %v, want 1h", me.Range)
				}
			},
		},
		{
			name:  "bytes_rate",
			input: `bytes_rate({job="varlogs"}[30m])`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Function != "bytes_rate" {
					t.Errorf("function = %s, want bytes_rate", me.Function)
				}
			},
		},
		{
			name:  "sum aggregation",
			input: `sum(count_over_time({job="varlogs"}[5m]))`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Aggregator == nil {
					t.Errorf("expected aggregator, got nil")
					return
				}
				if me.Aggregator.Op != "sum" {
					t.Errorf("aggregator op = %s, want sum", me.Aggregator.Op)
				}
				if len(me.Aggregator.Grouping) != 0 {
					t.Errorf("expected no grouping, got %v", me.Aggregator.Grouping)
				}
			},
		},
		{
			name:  "sum by level",
			input: `sum by (level) (count_over_time({job="varlogs"}[5m]))`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Aggregator == nil {
					t.Errorf("expected aggregator, got nil")
					return
				}
				if me.Aggregator.Op != "sum" {
					t.Errorf("aggregator op = %s, want sum", me.Aggregator.Op)
				}
				if len(me.Aggregator.Grouping) != 1 || me.Aggregator.Grouping[0] != "level" {
					t.Errorf("grouping = %v, want [level]", me.Aggregator.Grouping)
				}
				if me.Aggregator.Without {
					t.Errorf("expected by grouping, got without")
				}
			},
		},
		{
			name:  "avg by multiple labels",
			input: `avg by (level, service) (count_over_time({job="varlogs"}[5m]))`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Aggregator.Op != "avg" {
					t.Errorf("aggregator op = %s, want avg", me.Aggregator.Op)
				}
				if len(me.Aggregator.Grouping) != 2 {
					t.Errorf("expected 2 grouping labels, got %d", len(me.Aggregator.Grouping))
				}
			},
		},
		{
			name:  "count aggregation",
			input: `count(count_over_time({job="varlogs"}[5m]))`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Aggregator.Op != "count" {
					t.Errorf("aggregator op = %s, want count", me.Aggregator.Op)
				}
			},
		},
		{
			name:  "min aggregation",
			input: `min(count_over_time({job="varlogs"}[5m]))`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Aggregator.Op != "min" {
					t.Errorf("aggregator op = %s, want min", me.Aggregator.Op)
				}
			},
		},
		{
			name:  "max aggregation",
			input: `max(count_over_time({job="varlogs"}[5m]))`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if me.Aggregator.Op != "max" {
					t.Errorf("aggregator op = %s, want max", me.Aggregator.Op)
				}
			},
		},
		{
			name:  "without grouping",
			input: `sum without (level) (count_over_time({job="varlogs"}[5m]))`,
			checkExpr: func(t *testing.T, expr Expr) {
				me, ok := expr.(*MetricExpr)
				if !ok {
					t.Errorf("expected MetricExpr, got %T", expr)
					return
				}
				if !me.Aggregator.Without {
					t.Errorf("expected without grouping, got by")
				}
			},
		},
		{
			name:    "missing time range",
			input:   `count_over_time({job="varlogs"})`,
			wantErr: true,
			errMsg:  "requires a range vector argument",
		},
		{
			name:    "invalid function",
			input:   `invalid_func({job="varlogs"}[5m])`,
			wantErr: true,
		},
		{
			name:    "aggregation without inner function",
			input:   `sum({job="varlogs"}[5m])`,
			wantErr: true,
			errMsg:  "custom aggregation parsing not yet implemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			query, err := parser.Parse()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.checkExpr != nil {
				tt.checkExpr(t, query.Expr)
			}
		})
	}
}

func TestIsMetricQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{
			name:  "count_over_time",
			query: `count_over_time({job="varlogs"}[5m])`,
			want:  true,
		},
		{
			name:  "rate",
			query: `rate({job="varlogs"}[5m])`,
			want:  true,
		},
		{
			name:  "bytes_over_time",
			query: `bytes_over_time({job="varlogs"}[5m])`,
			want:  true,
		},
		{
			name:  "bytes_rate",
			query: `bytes_rate({job="varlogs"}[5m])`,
			want:  true,
		},
		{
			name:  "sum aggregation",
			query: `sum(count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
		{
			name:  "avg aggregation",
			query: `avg(count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
		{
			name:  "min aggregation",
			query: `min(count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
		{
			name:  "max aggregation",
			query: `max(count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
		{
			name:  "count aggregation",
			query: `count(count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
		{
			name:  "topk aggregation",
			query: `topk(10, count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
		{
			name:  "bottomk aggregation",
			query: `bottomk(10, count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
		{
			name:  "log range query",
			query: `{job="varlogs"}`,
			want:  false,
		},
		{
			name:  "log range with pipeline",
			query: `{job="varlogs"} |= "error"`,
			want:  false,
		},
		{
			name:  "whitespace before function",
			query: `  sum(count_over_time({job="varlogs"}[5m]))`,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMetricQuery(tt.query)
			if got != tt.want {
				t.Errorf("isMetricQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParse_ComplexQueries(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "stream selector with multiple matchers and complex pipeline",
			input:   `{job="varlogs", level!="debug", service=~"api.*"} |= "error" != "timeout" | json |~ "fail.*"`,
			wantErr: false,
		},
		{
			name:    "metric query with multiple label matchers",
			input:   `count_over_time({job="varlogs", level="error", service=~"api.*"}[1h])`,
			wantErr: false,
		},
		{
			name:    "aggregation with multiple grouping labels",
			input:   `sum by (level, service, environment) (count_over_time({job="varlogs"}[5m]))`,
			wantErr: false,
		},
		{
			name:    "bytes_over_time with line filters",
			input:   `bytes_over_time({job="varlogs"} |= "error" != "debug"[10m])`,
			wantErr: false,
		},
		{
			name:    "rate with regex line filter",
			input:   `rate({job="varlogs"} |~ "error|fail|timeout"[5m])`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			_, err := parser.Parse()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConvertCallToMetricExpr(t *testing.T) {
	// Test that the conversion properly extracts function name, stream selector, and time range
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid count_over_time",
			input:   `count_over_time({job="varlogs"}[5m])`,
			wantErr: false,
		},
		{
			name:    "valid rate",
			input:   `rate({job="varlogs"}[10m])`,
			wantErr: false,
		},
		{
			name:    "missing argument",
			input:   `count_over_time()`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			_, err := parser.Parse()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConvertAggregateToMetricExpr(t *testing.T) {
	// Test that aggregation conversion properly extracts the aggregator and inner metric expression
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid sum by",
			input:   `sum by (level) (count_over_time({job="varlogs"}[5m]))`,
			wantErr: false,
		},
		{
			name:    "valid avg without",
			input:   `avg without (debug) (count_over_time({job="varlogs"}[5m]))`,
			wantErr: false,
		},
		{
			name:    "aggregation without inner function",
			input:   `sum({job="varlogs"}[5m])`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.input)
			_, err := parser.Parse()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
