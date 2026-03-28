package logql

import (
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// ParseStreamSelector parses a LogQL stream selector using the Prometheus parser
// Examples: {job="varlogs"}, {job="varlogs", level="error"}
//
// This is a key reuse opportunity - LogQL stream selectors are identical to
// PromQL label selectors, so we can use the battle-tested Prometheus parser!
func ParseStreamSelector(input string) (*StreamSelector, error) {
	// Use Prometheus parser to parse the selector
	expr, err := parser.ParseExpr(input)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stream selector: %w", err)
	}

	// The result should be a VectorSelector
	vs, ok := expr.(*parser.VectorSelector)
	if !ok {
		return nil, fmt.Errorf("expected stream selector, got %T", expr)
	}

	// Extract the label matchers
	matchers := make([]*labels.Matcher, 0, len(vs.LabelMatchers))
	for _, m := range vs.LabelMatchers {
		// Skip the __name__ matcher if it exists (LogQL doesn't have metric names)
		if m.Name == labels.MetricName {
			continue
		}
		matchers = append(matchers, m)
	}

	return &StreamSelector{
		Matchers: matchers,
	}, nil
}

// ParseStreamSelectorWithContext parses a stream selector and returns both
// the selector and any metric name that was specified (for error messages)
func ParseStreamSelectorWithContext(input string) (*StreamSelector, string, error) {
	expr, err := parser.ParseExpr(input)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse stream selector: %w", err)
	}

	vs, ok := expr.(*parser.VectorSelector)
	if !ok {
		return nil, "", fmt.Errorf("expected stream selector, got %T", expr)
	}

	var metricName string
	matchers := make([]*labels.Matcher, 0, len(vs.LabelMatchers))

	for _, m := range vs.LabelMatchers {
		if m.Name == labels.MetricName {
			metricName = m.Value
			continue
		}
		matchers = append(matchers, m)
	}

	return &StreamSelector{Matchers: matchers}, metricName, nil
}

// ValidateStreamSelector checks if a stream selector is valid for LogQL
// LogQL requires at least one positive matcher (= or =~)
func ValidateStreamSelector(selector *StreamSelector) error {
	if len(selector.Matchers) == 0 {
		return fmt.Errorf("stream selector must have at least one label matcher")
	}

	// Check if there's at least one positive matcher
	hasPositiveMatcher := false
	for _, m := range selector.Matchers {
		if m.Type == labels.MatchEqual || m.Type == labels.MatchRegexp {
			hasPositiveMatcher = true
			break
		}
	}

	if !hasPositiveMatcher {
		return fmt.Errorf("stream selector must contain at least one positive matcher (= or =~)")
	}

	return nil
}
