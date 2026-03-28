package logql

import (
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql/parser"
)

// Parser parses LogQL queries
type Parser struct {
	input string
}

// NewParser creates a new LogQL parser
func NewParser(input string) *Parser {
	return &Parser{input: strings.TrimSpace(input)}
}

// Parse parses the LogQL query
func (p *Parser) Parse() (*Query, error) {
	// Check if this is a metric query (starts with aggregation function)
	// Examples: count_over_time({...}[5m]), sum by (level) (count_over_time({...}[5m]))
	if isMetricQuery(p.input) {
		return p.parseMetricQuery()
	}

	// Otherwise it's a log range query
	return p.parseLogRangeQuery()
}

// parseLogRangeQuery parses a log range query: {stream_selector} [pipeline]
func (p *Parser) parseLogRangeQuery() (*Query, error) {
	// Split into stream selector and pipeline
	streamPart, pipelinePart, err := SplitQueryParts(p.input)
	if err != nil {
		return nil, err
	}

	// Parse stream selector using Prometheus parser (reuse!)
	selector, err := ParseStreamSelector(streamPart)
	if err != nil {
		return nil, err
	}

	// Validate stream selector
	if err := ValidateStreamSelector(selector); err != nil {
		return nil, err
	}

	// Parse pipeline stages (custom parser)
	var pipeline []PipelineStage
	if pipelinePart != "" {
		pipeline, err = ParsePipeline(pipelinePart)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pipeline: %w", err)
		}
	}

	return &Query{
		Expr: &LogRangeExpr{
			StreamSelector: selector,
			Pipeline:       pipeline,
		},
	}, nil
}

// parseMetricQuery parses a metric query using Prometheus parser
// This is possible because LogQL metric queries use PromQL-compatible syntax!
func (p *Parser) parseMetricQuery() (*Query, error) {
	// Try to parse with Prometheus parser first
	expr, err := parser.ParseExpr(p.input)
	if err != nil {
		// If PromQL parser fails, try our custom parsing
		return p.parseMetricQueryCustom()
	}

	// Check what we got
	switch e := expr.(type) {
	case *parser.Call:
		// Function call like count_over_time({...}[5m])
		return p.convertCallToMetricExpr(e)

	case *parser.AggregateExpr:
		// Aggregation like sum by (level) (count_over_time({...}[5m]))
		return p.convertAggregateToMetricExpr(e)

	default:
		return nil, fmt.Errorf("unexpected metric query type: %T", expr)
	}
}

// convertCallToMetricExpr converts a PromQL Call expression to LogQL MetricExpr
func (p *Parser) convertCallToMetricExpr(call *parser.Call) (*Query, error) {
	// Extract function name
	functionName := call.Func.Name

	// The argument should be a matrix selector (range vector)
	if len(call.Args) != 1 {
		return nil, fmt.Errorf("%s requires exactly one argument", functionName)
	}

	matrixSel, ok := call.Args[0].(*parser.MatrixSelector)
	if !ok {
		return nil, fmt.Errorf("%s requires a range vector argument", functionName)
	}

	// Extract stream selector from the matrix selector
	vs, ok := matrixSel.VectorSelector.(*parser.VectorSelector)
	if !ok {
		return nil, fmt.Errorf("invalid stream selector in metric query")
	}

	// Convert to our StreamSelector type
	selector := &StreamSelector{
		Matchers: vs.LabelMatchers,
	}

	// Create metric expression
	return &Query{
		Expr: &MetricExpr{
			Function: functionName,
			LogRange: &LogRangeExpr{
				StreamSelector: selector,
				Pipeline:       nil, // Metric queries from PromQL don't have pipelines
			},
			Range:      matrixSel.Range,
			Aggregator: nil,
		},
	}, nil
}

// convertAggregateToMetricExpr converts a PromQL Aggregate expression to LogQL MetricExpr
func (p *Parser) convertAggregateToMetricExpr(agg *parser.AggregateExpr) (*Query, error) {
	// The inner expression should be a Call
	call, ok := agg.Expr.(*parser.Call)
	if !ok {
		return nil, fmt.Errorf("aggregation must wrap a metric function, got %T", agg.Expr)
	}

	// Convert the inner call to a metric expression
	innerQuery, err := p.convertCallToMetricExpr(call)
	if err != nil {
		return nil, err
	}

	metricExpr := innerQuery.Expr.(*MetricExpr)

	// Add the aggregator
	metricExpr.Aggregator = &Aggregator{
		Op:       agg.Op.String(),
		Grouping: agg.Grouping,
		Without:  agg.Without,
	}

	return &Query{Expr: metricExpr}, nil
}

// parseMetricQueryCustom handles metric queries that PromQL parser can't handle
// (e.g., LogQL-specific functions like bytes_over_time, or PromQL functions with pipeline stages)
func (p *Parser) parseMetricQueryCustom() (*Query, error) {
	// Check for metric functions (both LogQL-specific and PromQL-compatible with pipelines)
	input := strings.TrimSpace(p.input)

	// Try to match: function_name({...}[duration])
	// or: aggregator(function_name({...}[duration]))

	// LogQL-specific and PromQL functions that might have pipeline stages
	functions := []string{
		"bytes_over_time", "bytes_rate", // LogQL-specific
		"count_over_time", "rate",       // PromQL-compatible but with pipeline
	}

	for _, fn := range functions {
		if strings.HasPrefix(input, fn+"(") {
			return p.parseLogQLMetricFunction(fn, input[len(fn)+1:])
		}
	}

	// Check for aggregations wrapping LogQL functions
	// Examples: sum by (level) (bytes_over_time({...}[5m]))
	for _, agg := range []string{"sum", "avg", "min", "max", "count"} {
		if strings.HasPrefix(input, agg+"(") || strings.HasPrefix(input, agg+" ") {
			// Check if it contains a LogQL-specific function
			for _, fn := range []string{"bytes_over_time", "bytes_rate"} {
				if strings.Contains(input, fn+"(") {
					// Parse the aggregation with custom logic
					return p.parseAggregatedLogQLMetric(input)
				}
			}
			// Not a LogQL-specific function, let PromQL parser handle it
			return nil, fmt.Errorf("custom aggregation parsing not yet implemented")
		}
	}

	return nil, fmt.Errorf("custom metric query parsing not yet implemented")
}

// parseAggregatedLogQLMetric parses aggregations wrapping LogQL-specific functions
// Example: sum by (level) (bytes_over_time({job="varlogs"}[5m]))
func (p *Parser) parseAggregatedLogQLMetric(input string) (*Query, error) {
	// Pattern: aggregator [by/without (labels)] (function({...}[duration]))

	// Extract aggregator name
	input = strings.TrimSpace(input)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty aggregation expression")
	}

	aggOp := parts[0]

	// Find the grouping clause if present: by (label1, label2) or without (label1)
	var grouping []string
	var without bool

	byIdx := strings.Index(input, " by ")
	withoutIdx := strings.Index(input, " without ")

	if byIdx != -1 {
		// Extract "by" grouping labels
		startParen := strings.Index(input[byIdx:], "(")
		endParen := strings.Index(input[byIdx:], ")")
		if startParen != -1 && endParen != -1 {
			labelStr := input[byIdx+startParen+1 : byIdx+endParen]
			for _, label := range strings.Split(labelStr, ",") {
				grouping = append(grouping, strings.TrimSpace(label))
			}
		}
	} else if withoutIdx != -1 {
		without = true
		startParen := strings.Index(input[withoutIdx:], "(")
		endParen := strings.Index(input[withoutIdx:], ")")
		if startParen != -1 && endParen != -1 {
			labelStr := input[withoutIdx+startParen+1 : withoutIdx+endParen]
			for _, label := range strings.Split(labelStr, ",") {
				grouping = append(grouping, strings.TrimSpace(label))
			}
		}
	}

	// Find the inner function call: function_name({...}[duration])
	// Pattern is: aggregator [by/without (...)] (inner_function)
	// We need to find the last top-level parenthesized expression

	// Find all top-level parenthesized sections
	var parenSections []struct {
		start int
		end   int
	}

	for i := 0; i < len(input); i++ {
		if input[i] == '(' {
			start := i
			depth := 1
			for j := i + 1; j < len(input); j++ {
				if input[j] == '(' {
					depth++
				} else if input[j] == ')' {
					depth--
					if depth == 0 {
						parenSections = append(parenSections, struct {
							start int
							end   int
						}{start, j})
						i = j
						break
					}
				}
			}
		}
	}

	if len(parenSections) == 0 {
		return nil, fmt.Errorf("malformed aggregation expression: no parenthesized expressions")
	}

	// The last parenthesized section is the inner function
	lastSection := parenSections[len(parenSections)-1]
	innerFunc := strings.TrimSpace(input[lastSection.start+1 : lastSection.end])

	// Parse the inner function using parseLogQLMetricFunction
	for _, fn := range []string{"bytes_over_time", "bytes_rate"} {
		if strings.HasPrefix(innerFunc, fn+"(") {
			innerQuery, err := p.parseLogQLMetricFunction(fn, innerFunc[len(fn)+1:])
			if err != nil {
				return nil, err
			}

			// Add the aggregator to the metric expression
			metricExpr := innerQuery.Expr.(*MetricExpr)
			metricExpr.Aggregator = &Aggregator{
				Op:       aggOp,
				Grouping: grouping,
				Without:  without,
			}

			return innerQuery, nil
		}
	}

	return nil, fmt.Errorf("failed to parse aggregated LogQL metric")
}

// parseLogQLMetricFunction parses LogQL-specific metric functions
func (p *Parser) parseLogQLMetricFunction(function string, rest string) (*Query, error) {
	// Expected format: {selector}[duration])
	// Find the closing parenthesis
	closeParen := strings.LastIndex(rest, ")")
	if closeParen == -1 {
		return nil, fmt.Errorf("%s requires closing parenthesis", function)
	}

	inner := strings.TrimSpace(rest[:closeParen])

	// Split into selector+pipeline and duration
	// Find the range: [duration]
	rangeStart := strings.LastIndex(inner, "[")
	rangeEnd := strings.LastIndex(inner, "]")
	if rangeStart == -1 || rangeEnd == -1 || rangeEnd <= rangeStart {
		return nil, fmt.Errorf("%s requires a range vector argument [duration]", function)
	}

	durationStr := inner[rangeStart+1 : rangeEnd]
	selectorAndPipeline := strings.TrimSpace(inner[:rangeStart])

	// Parse the duration
	modelDuration, err := model.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid duration %q: %w", durationStr, err)
	}
	duration := time.Duration(modelDuration)

	// Parse stream selector and pipeline
	streamPart, pipelinePart, err := SplitQueryParts(selectorAndPipeline)
	if err != nil {
		return nil, err
	}

	selector, err := ParseStreamSelector(streamPart)
	if err != nil {
		return nil, err
	}

	if err := ValidateStreamSelector(selector); err != nil {
		return nil, err
	}

	var pipeline []PipelineStage
	if pipelinePart != "" {
		pipeline, err = ParsePipeline(pipelinePart)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pipeline: %w", err)
		}
	}

	return &Query{
		Expr: &MetricExpr{
			Function: function,
			LogRange: &LogRangeExpr{
				StreamSelector: selector,
				Pipeline:       pipeline,
			},
			Range:      duration,
			Aggregator: nil,
		},
	}, nil
}

// isMetricQuery checks if a query is a metric query
// Metric queries start with function names like count_over_time, rate, sum, etc.
func isMetricQuery(query string) bool {
	query = strings.TrimSpace(query)

	// LogQL-specific metric functions (not in PromQL)
	logQLFunctions := []string{
		"bytes_over_time", "bytes_rate",
	}

	// PromQL-compatible functions
	promQLFunctions := []string{
		"count_over_time", "rate",
		"sum", "avg", "min", "max", "count", "stddev", "stdvar",
		"topk", "bottomk",
	}

	// Check LogQL-specific functions first
	for _, fn := range logQLFunctions {
		if strings.HasPrefix(query, fn+"(") || strings.HasPrefix(query, fn+" ") {
			return true
		}
	}

	// Then check PromQL-compatible functions
	for _, fn := range promQLFunctions {
		if strings.HasPrefix(query, fn+"(") || strings.HasPrefix(query, fn+" ") {
			return true
		}
	}

	return false
}
