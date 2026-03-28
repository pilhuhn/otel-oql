package promql

import (
	"fmt"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// Translator translates PromQL queries to Pinot SQL
type Translator struct {
	tenantID int
	start    *time.Time // Optional start time for range queries
	end      *time.Time // Optional end time for range queries
}

// NewTranslator creates a new PromQL to SQL translator
func NewTranslator(tenantID int) *Translator {
	return &Translator{tenantID: tenantID}
}

// TranslateQuery parses PromQL and translates to Pinot SQL
func (t *Translator) TranslateQuery(promql string) ([]string, error) {
	// Parse using Prometheus parser
	expr, err := parser.ParseExpr(promql)
	if err != nil {
		return nil, fmt.Errorf("promql parse error: %w", err)
	}

	// Translate AST to SQL
	sql, err := t.translateExpr(expr)
	if err != nil {
		return nil, err
	}

	return []string{sql}, nil
}

// TranslateQueryWithTimeRange parses PromQL and translates to Pinot SQL with time range filter
func (t *Translator) TranslateQueryWithTimeRange(promql string, start, end *time.Time) ([]string, error) {
	// Store time range in translator
	t.start = start
	t.end = end

	// Use regular translation
	return t.TranslateQuery(promql)
}

// translateExpr translates Prometheus AST expressions to SQL
func (t *Translator) translateExpr(expr parser.Expr) (string, error) {
	switch e := expr.(type) {
	case *parser.VectorSelector:
		return t.translateVectorSelector(e)
	case *parser.MatrixSelector:
		return t.translateMatrixSelector(e)
	case *parser.AggregateExpr:
		return t.translateAggregate(e)
	case *parser.BinaryExpr:
		return t.translateBinary(e)
	case *parser.Call:
		return t.translateCall(e)
	case *parser.NumberLiteral:
		// Number literals by themselves aren't meaningful in our context
		return "", fmt.Errorf("number literal queries not supported")
	case *parser.StringLiteral:
		return "", fmt.Errorf("string literal queries not supported")
	case *parser.ParenExpr:
		// Unwrap parentheses and translate inner expression
		return t.translateExpr(e.Expr)
	case *parser.UnaryExpr:
		return "", fmt.Errorf("unary expressions not yet supported")
	case *parser.SubqueryExpr:
		return "", fmt.Errorf("subqueries not yet supported")
	default:
		return "", fmt.Errorf("unsupported PromQL expression type: %T", expr)
	}
}

// translateVectorSelector translates an instant vector selector
// Example: http_requests_total{job="api", status="200"}
func (t *Translator) translateVectorSelector(vs *parser.VectorSelector) (string, error) {
	// Check for unsupported features
	if vs.OriginalOffset != 0 {
		return "", fmt.Errorf("offset modifier not supported (offset %v)", vs.OriginalOffset)
	}

	// Start with base query
	sql := fmt.Sprintf("SELECT * FROM otel_metrics WHERE tenant_id = %d", t.tenantID)

	// Extract metric name from label matchers
	metricName := ""
	additionalMatchers := make([]*labels.Matcher, 0)

	for _, matcher := range vs.LabelMatchers {
		if matcher.Name == labels.MetricName {
			// This is the metric name matcher
			if matcher.Type == labels.MatchEqual {
				metricName = matcher.Value
			} else if matcher.Type == labels.MatchRegexp {
				// Regex match on metric name
				return "", fmt.Errorf("regex matching on metric name not yet supported")
			} else {
				return "", fmt.Errorf("unsupported matcher type for metric name: %s", matcher.Type)
			}
		} else {
			additionalMatchers = append(additionalMatchers, matcher)
		}
	}

	// Add metric name filter
	if metricName != "" {
		sql += " AND metric_name = " + sqlutil.StringLiteral(metricName)
	}

	// Add label matchers
	for _, matcher := range additionalMatchers {
		condition, err := t.translateLabelMatcher(matcher)
		if err != nil {
			return "", fmt.Errorf("failed to translate label matcher: %w", err)
		}
		sql += " AND " + condition
	}

	// Add time range filter if provided
	if t.start != nil && t.end != nil {
		startMillis := t.start.UnixMilli()
		endMillis := t.end.UnixMilli()
		sql += fmt.Sprintf(" AND timestamp >= %d AND timestamp <= %d", startMillis, endMillis)
	}

	return sql, nil
}

// translateMatrixSelector translates a range vector selector
// Example: http_requests_total{job="api"}[5m]
func (t *Translator) translateMatrixSelector(ms *parser.MatrixSelector) (string, error) {
	// Start with the vector selector - need type assertion
	vs, ok := ms.VectorSelector.(*parser.VectorSelector)
	if !ok {
		return "", fmt.Errorf("matrix selector does not contain a vector selector")
	}

	// Temporarily clear time range to avoid double-applying in translateVectorSelector
	start := t.start
	end := t.end
	t.start = nil
	t.end = nil

	sql, err := t.translateVectorSelector(vs)
	if err != nil {
		return "", err
	}

	// Restore time range
	t.start = start
	t.end = end

	// Add time range filter
	if t.start != nil && t.end != nil {
		// Use explicit time range
		startMillis := t.start.UnixMilli()
		endMillis := t.end.UnixMilli()
		sql += fmt.Sprintf(" AND timestamp >= %d AND timestamp <= %d", startMillis, endMillis)
	} else {
		// Use relative time range (lookback from now)
		rangeMillis := ms.Range.Milliseconds()
		sql += fmt.Sprintf(" AND timestamp >= (now() - %d)", rangeMillis)
	}

	return sql, nil
}

// translateLabelMatcher translates a Prometheus label matcher to SQL condition
func (t *Translator) translateLabelMatcher(matcher *labels.Matcher) (string, error) {
	labelName := matcher.Name
	labelValue := matcher.Value

	// Check if this label maps to a native column
	nativeColumn := getNativeColumn(labelName)

	var fieldRef string
	if nativeColumn != "" {
		fieldRef = nativeColumn
	} else {
		// Use JSON extraction for attributes
		fieldRef = fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, '$.%s', 'STRING')", labelName)
	}

	switch matcher.Type {
	case labels.MatchEqual:
		return fmt.Sprintf("%s = %s", fieldRef, sqlutil.StringLiteral(labelValue)), nil
	case labels.MatchNotEqual:
		return fmt.Sprintf("%s <> %s", fieldRef, sqlutil.StringLiteral(labelValue)), nil
	case labels.MatchRegexp:
		// Pinot uses REGEXP_LIKE for regex matching
		return fmt.Sprintf("REGEXP_LIKE(%s, %s)", fieldRef, sqlutil.StringLiteral(labelValue)), nil
	case labels.MatchNotRegexp:
		return fmt.Sprintf("NOT REGEXP_LIKE(%s, %s)", fieldRef, sqlutil.StringLiteral(labelValue)), nil
	default:
		return "", fmt.Errorf("unsupported matcher type: %s", matcher.Type)
	}
}

// translateAggregate translates an aggregation expression
// Example: sum(http_requests_total) or sum by (service) (http_requests_total)
func (t *Translator) translateAggregate(ae *parser.AggregateExpr) (string, error) {
	// Check if inner expression is another aggregation (nested aggregations not supported)
	if _, ok := ae.Expr.(*parser.AggregateExpr); ok {
		return "", fmt.Errorf("nested aggregations not supported: %s(%s(...))", ae.Op, ae.Op)
	}

	// First translate the inner expression to get base query
	baseSQL, err := t.translateExpr(ae.Expr)
	if err != nil {
		return "", fmt.Errorf("failed to translate aggregate inner expression: %w", err)
	}

	// Determine aggregation function
	var aggFunc string
	switch ae.Op {
	case parser.SUM:
		aggFunc = "SUM(value)"
	case parser.AVG:
		aggFunc = "AVG(value)"
	case parser.MIN:
		aggFunc = "MIN(value)"
	case parser.MAX:
		aggFunc = "MAX(value)"
	case parser.COUNT:
		aggFunc = "COUNT(*)"
	case parser.STDDEV:
		aggFunc = "STDDEV(value)"
	case parser.STDVAR:
		aggFunc = "VARIANCE(value)"
	default:
		return "", fmt.Errorf("unsupported aggregation function: %s", ae.Op)
	}

	// Build SELECT clause with grouping if needed
	var selectClause string
	var groupByClause string

	if len(ae.Grouping) > 0 {
		// sum by (label1, label2)
		groupFields := make([]string, 0, len(ae.Grouping))
		for _, label := range ae.Grouping {
			nativeColumn := getNativeColumn(label)
			if nativeColumn != "" {
				groupFields = append(groupFields, nativeColumn)
			} else {
				groupFields = append(groupFields, fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, '$.%s', 'STRING')", label))
			}
		}
		selectClause = strings.Join(groupFields, ", ") + ", " + aggFunc
		groupByClause = " GROUP BY " + strings.Join(groupFields, ", ")
	} else if ae.Without {
		// sum without (label1, label2) - not commonly used, more complex
		return "", fmt.Errorf("aggregation with 'without' clause not yet supported")
	} else {
		// Just aggregation, no grouping
		selectClause = aggFunc
	}

	// Replace SELECT * with actual SELECT clause
	sql := strings.Replace(baseSQL, "SELECT *", "SELECT "+selectClause, 1)
	sql += groupByClause

	return sql, nil
}

// translateBinary translates binary expressions
// Example: metric1 > 100
func (t *Translator) translateBinary(be *parser.BinaryExpr) (string, error) {
	// Handle comparison with scalar on RHS
	// e.g., http_requests_total > 100
	if be.Op.IsComparisonOperator() {
		// Check if RHS is a number literal
		if numLit, ok := be.RHS.(*parser.NumberLiteral); ok {
			// LHS should be a selector
			baseSQL, err := t.translateExpr(be.LHS)
			if err != nil {
				return "", err
			}

			// Add value comparison
			var op string
			switch be.Op {
			case parser.EQL, parser.EQLC:
				op = "="
			case parser.NEQ:
				op = "<>"
			case parser.GTR:
				op = ">"
			case parser.LSS:
				op = "<"
			case parser.GTE:
				op = ">="
			case parser.LTE:
				op = "<="
			default:
				return "", fmt.Errorf("unsupported comparison operator: %s", be.Op)
			}

			sql := baseSQL + fmt.Sprintf(" AND value %s %f", op, numLit.Val)
			return sql, nil
		}
	}

	// Binary operations between metrics not yet supported
	return "", fmt.Errorf("binary operations between metrics not yet supported")
}

// translateCall translates function calls
// Example: rate(http_requests_total[5m])
func (t *Translator) translateCall(call *parser.Call) (string, error) {
	switch call.Func.Name {
	case "rate", "irate":
		return t.translateRateFunction(call)
	default:
		return "", fmt.Errorf("function '%s' not yet supported", call.Func.Name)
	}
}

// translateRateFunction translates rate/irate functions
// Example: rate(http_requests_total[5m])
func (t *Translator) translateRateFunction(call *parser.Call) (string, error) {
	// Rate requires a matrix selector (range vector) as argument
	if len(call.Args) != 1 {
		return "", fmt.Errorf("rate() requires exactly one argument")
	}

	matrixSel, ok := call.Args[0].(*parser.MatrixSelector)
	if !ok {
		// Check if it's a subquery
		if _, isSubquery := call.Args[0].(*parser.SubqueryExpr); isSubquery {
			return "", fmt.Errorf("subqueries not supported")
		}
		return "", fmt.Errorf("rate() requires a range vector argument")
	}

	// Get base query with time range
	baseSQL, err := t.translateMatrixSelector(matrixSel)
	if err != nil {
		return "", err
	}

	// Calculate rate: (latest_value - earliest_value) / time_range_seconds
	// This is a simplified implementation - real rate calculation is more complex
	rangeSeconds := matrixSel.Range.Seconds()

	// Note: This is a simplified rate calculation
	// Real Prometheus rate() is more sophisticated (handles counter resets, etc.)
	sql := strings.Replace(
		baseSQL,
		"SELECT *",
		fmt.Sprintf("SELECT (MAX(value) - MIN(value)) / %f AS rate", rangeSeconds),
		1,
	)

	return sql, nil
}

// getNativeColumn maps label names to native Pinot columns
// Reuses the same mappings as OQL translator
func getNativeColumn(labelName string) string {
	// Map of common Prometheus labels to OTel semantic conventions / native columns
	nativeColumns := map[string]string{
		// Metric attributes
		"job":         "job",
		"instance":    "instance",
		"environment": "environment",

		// Service attributes
		"service":      "service_name",
		"service_name": "service_name",

		// HTTP attributes
		"method":        "http_method",
		"status":        "http_status_code",
		"status_code":   "http_status_code",
		"route":         "http_route",

		// Host attributes
		"host":      "host_name",
		"host_name": "host_name",
	}

	if nativeCol, ok := nativeColumns[labelName]; ok {
		return nativeCol
	}

	return ""
}
