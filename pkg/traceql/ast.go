package traceql

import (
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

// Query represents a parsed TraceQL query
type Query struct {
	Expr Expr
}

// Expr is the interface for all TraceQL expressions
type Expr interface {
	expr()
}

// SpanFilterExpr represents a span selection query
// Example: {span.http.status_code = 500 && duration > 100ms}
type SpanFilterExpr struct {
	Conditions []Condition
}

func (e *SpanFilterExpr) expr() {}

// Condition represents a single filter condition
// Example: span.http.status_code = 500
type Condition struct {
	Field    FieldExpr
	Operator string // =, !=, >, <, >=, <=, =~, !~
	Value    interface{}
}

// FieldExpr represents a field reference in TraceQL
type FieldExpr struct {
	Type string // "span", "resource", "intrinsic"
	Name string // e.g., "http.status_code", "service.name", "duration"
}

// LogicalExpr represents a logical combination of conditions
// Example: condition1 && condition2
type LogicalExpr struct {
	Left     Expr
	Operator string // "&&", "||"
	Right    Expr
}

func (e *LogicalExpr) expr() {}

// AggregateExpr represents an aggregation over spans
// Example: count() by (span.service.name)
type AggregateExpr struct {
	Function string   // "count", "sum", "avg", etc.
	Inner    Expr     // The span filter expression
	Grouping []string // Group by fields
}

func (e *AggregateExpr) expr() {}

// ScalarExpr represents a scalar value (for connection tests)
type ScalarExpr struct {
	Value float64
}

func (e *ScalarExpr) expr() {}

// IntrinsicField represents TraceQL intrinsic fields
type IntrinsicField string

const (
	IntrinsicDuration IntrinsicField = "duration"
	IntrinsicName     IntrinsicField = "name"
	IntrinsicStatus   IntrinsicField = "status"
	IntrinsicKind     IntrinsicField = "kind"
	IntrinsicTraceID  IntrinsicField = "traceid"
	IntrinsicSpanID   IntrinsicField = "spanid"
)

// IsIntrinsic checks if a field name is an intrinsic field
func IsIntrinsic(name string) bool {
	intrinsics := []string{
		"duration", "name", "status", "kind", "traceid", "spanid",
	}
	for _, i := range intrinsics {
		if name == i {
			return true
		}
	}
	return false
}

// MatcherType represents the type of matcher (similar to Prometheus labels)
type MatcherType int

const (
	MatchEqual MatcherType = iota
	MatchNotEqual
	MatchRegexp
	MatchNotRegexp
	MatchGreater
	MatchLess
	MatchGreaterOrEqual
	MatchLessOrEqual
)

// Matcher represents a field matcher (similar to Prometheus label matcher)
type Matcher struct {
	Type  MatcherType
	Name  string
	Value string
}

// ToPrometheusMatcherType converts TraceQL matcher type to Prometheus matcher type
// This allows us to reuse common.TranslateLabelMatcher
func (m *Matcher) ToPrometheusMatcherType() labels.MatchType {
	switch m.Type {
	case MatchEqual:
		return labels.MatchEqual
	case MatchNotEqual:
		return labels.MatchNotEqual
	case MatchRegexp:
		return labels.MatchRegexp
	case MatchNotRegexp:
		return labels.MatchNotRegexp
	default:
		return labels.MatchEqual
	}
}

// ToPrometheusLabelMatcher converts a TraceQL Matcher to a Prometheus LabelMatcher
// This allows reuse of common label matching code
func (m *Matcher) ToPrometheusLabelMatcher() *labels.Matcher {
	return &labels.Matcher{
		Type:  m.ToPrometheusMatcherType(),
		Name:  m.Name,
		Value: m.Value,
	}
}

// DurationLiteral represents a duration value
// Example: 100ms, 5s, 1m
type DurationLiteral struct {
	Duration time.Duration
}

// StatusValue represents a status enum value
// TraceQL supports: unset, ok, error
type StatusValue string

const (
	StatusUnset StatusValue = "unset"
	StatusOK    StatusValue = "ok"
	StatusError StatusValue = "error"
)
