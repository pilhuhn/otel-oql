package logql

import (
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

// Query represents a complete LogQL query
type Query struct {
	Expr Expr
}

// Expr is the root interface for all LogQL expressions
type Expr interface {
	expr()
}

// LogRangeExpr represents a log range query: {stream_selector} [pipeline_stages]
type LogRangeExpr struct {
	StreamSelector *StreamSelector
	Pipeline       []PipelineStage
}

func (LogRangeExpr) expr() {}

// MetricExpr represents a metric query from logs: aggregation(log_range)
type MetricExpr struct {
	Function   string        // count_over_time, rate, etc.
	LogRange   *LogRangeExpr // The log query to aggregate
	Range      time.Duration // Time range [5m]
	Aggregator *Aggregator   // Optional outer aggregation (sum, avg, etc.)
}

func (MetricExpr) expr() {}

// ScalarExpr represents a scalar arithmetic expression (e.g., vector(1)+vector(1) for connection tests)
type ScalarExpr struct {
	Value float64 // The computed scalar value
}

func (ScalarExpr) expr() {}

// StreamSelector represents {label="value", label2=~"regex"}
type StreamSelector struct {
	Matchers []*labels.Matcher
}

// PipelineStage represents a single stage in the log pipeline
type PipelineStage interface {
	pipelineStage()
}

// LineFilter represents line filtering: |= "text", != "text", |~ "regex", !~ "regex"
type LineFilter struct {
	Operator string // |=, !=, |~, !~
	Value    string
}

func (LineFilter) pipelineStage() {}

// LabelParser represents label extraction: | json, | logfmt, | pattern, | regexp
type LabelParser struct {
	Type   string // json, logfmt, pattern, regexp
	Params string // Parameters for the parser
}

func (LabelParser) pipelineStage() {}

// LabelFilter represents label filtering after parsing: | label="value"
type LabelFilter struct {
	Label    string
	Operator string // =, !=, =~, !~
	Value    string
}

func (LabelFilter) pipelineStage() {}

// LabelManipulation represents label manipulation: | drop label1, label2 | keep label3, label4
type LabelManipulation struct {
	Operation string   // "drop" or "keep"
	Labels    []string // Labels to drop/keep
}

func (LabelManipulation) pipelineStage() {}

// Aggregator represents an aggregation operation: sum by (label)
type Aggregator struct {
	Op       string   // sum, avg, min, max, count, etc.
	Grouping []string // Labels to group by
	Without  bool     // true if "without" instead of "by"
}

// String returns a string representation of the stream selector
func (s *StreamSelector) String() string {
	if len(s.Matchers) == 0 {
		return "{}"
	}

	result := "{"
	for i, m := range s.Matchers {
		if i > 0 {
			result += ", "
		}
		result += m.Name + m.Type.String() + `"` + m.Value + `"`
	}
	result += "}"
	return result
}
