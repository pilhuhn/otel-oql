package logql

import (
	"fmt"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/querylangs/common"
	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
)

// Translator translates LogQL queries to Pinot SQL
type Translator struct {
	tenantID int
	start    *time.Time // Optional start time for range queries
	end      *time.Time // Optional end time for range queries
}

// NewTranslator creates a new LogQL to SQL translator
func NewTranslator(tenantID int) *Translator {
	return &Translator{tenantID: tenantID}
}

// TranslateQuery translates a LogQL query to Pinot SQL
func (t *Translator) TranslateQuery(logql string) ([]string, error) {
	// Parse the LogQL query
	parser := NewParser(logql)
	query, err := parser.Parse()
	if err != nil {
		return nil, fmt.Errorf("failed to parse LogQL: %w", err)
	}

	// Translate based on expression type
	switch expr := query.Expr.(type) {
	case *LogRangeExpr:
		sql, err := t.translateLogRangeExpr(expr)
		if err != nil {
			return nil, err
		}
		return []string{sql}, nil

	case *MetricExpr:
		sql, err := t.translateMetricExpr(expr)
		if err != nil {
			return nil, err
		}
		return []string{sql}, nil

	default:
		return nil, fmt.Errorf("unsupported LogQL expression type: %T", expr)
	}
}

// TranslateQueryWithTimeRange translates a LogQL query to Pinot SQL with time range filter
func (t *Translator) TranslateQueryWithTimeRange(logql string, start, end *time.Time) ([]string, error) {
	// Store time range in translator
	t.start = start
	t.end = end

	// Use regular translation
	return t.TranslateQuery(logql)
}

// translateLogRangeExpr translates a log range query to SQL
func (t *Translator) translateLogRangeExpr(expr *LogRangeExpr) (string, error) {
	// Start with base query
	sql := fmt.Sprintf("SELECT * FROM otel_logs WHERE tenant_id = %d", t.tenantID)

	// Add stream selector conditions (reuse common code!)
	for _, matcher := range expr.StreamSelector.Matchers {
		condition, err := common.TranslateLabelMatcher(matcher, common.GetLogNativeColumn)
		if err != nil {
			return "", fmt.Errorf("failed to translate matcher: %w", err)
		}
		sql += " AND " + condition
	}

	// Add pipeline stages
	for _, stage := range expr.Pipeline {
		stageSQL, err := t.translatePipelineStage(stage)
		if err != nil {
			return "", err
		}
		if stageSQL != "" {
			sql += " AND " + stageSQL
		}
	}

	// Add time range filter if provided
	if t.start != nil && t.end != nil {
		startMillis := t.start.UnixMilli()
		endMillis := t.end.UnixMilli()
		sql += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	}

	return sql, nil
}

// translateMetricExpr translates a metric query to SQL
func (t *Translator) translateMetricExpr(expr *MetricExpr) (string, error) {
	// Start with base query
	sql := fmt.Sprintf("SELECT * FROM otel_logs WHERE tenant_id = %d", t.tenantID)

	// Add stream selector conditions
	for _, matcher := range expr.LogRange.StreamSelector.Matchers {
		condition, err := common.TranslateLabelMatcher(matcher, common.GetLogNativeColumn)
		if err != nil {
			return "", fmt.Errorf("failed to translate matcher: %w", err)
		}
		sql += " AND " + condition
	}

	// Add pipeline stages if any (before time range for consistent ordering)
	for _, stage := range expr.LogRange.Pipeline {
		stageSQL, err := t.translatePipelineStage(stage)
		if err != nil {
			return "", err
		}
		if stageSQL != "" {
			sql += " AND " + stageSQL
		}
	}

	// Add time range filter
	if t.start != nil && t.end != nil {
		// Use explicit time range from API parameters
		startMillis := t.start.UnixMilli()
		endMillis := t.end.UnixMilli()
		sql += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	} else if expr.Range > 0 {
		// Use relative time range from query (reuse common code!)
		timeFilter := common.TranslateTimeRange(expr.Range)
		sql += " AND " + timeFilter
	}

	// Apply metric function
	sql, err := t.applyMetricFunction(sql, expr.Function)
	if err != nil {
		return "", err
	}

	// Apply aggregation if present
	if expr.Aggregator != nil {
		sql, err = t.applyAggregation(sql, expr.Aggregator)
		if err != nil {
			return "", err
		}
	}

	return sql, nil
}

// translatePipelineStage translates a pipeline stage to SQL
func (t *Translator) translatePipelineStage(stage PipelineStage) (string, error) {
	switch s := stage.(type) {
	case *LineFilter:
		return t.translateLineFilter(s)

	case *LabelParser:
		// Label parsers (| json, | logfmt) don't directly translate to SQL
		// They would need to be processed in the application layer
		// For now, we skip them in SQL translation
		return "", nil

	case *LabelFilter:
		// Label filters after parsing also need application-layer processing
		return "", nil

	default:
		return "", fmt.Errorf("unsupported pipeline stage: %T", stage)
	}
}

// translateLineFilter translates a line filter to SQL
func (t *Translator) translateLineFilter(filter *LineFilter) (string, error) {
	bodyField := "body" // The log message body column

	switch filter.Operator {
	case "|=":
		// Contains
		return fmt.Sprintf("%s LIKE %s", bodyField, sqlutil.StringLiteral("%"+filter.Value+"%")), nil

	case "!=":
		// Does not contain
		return fmt.Sprintf("%s NOT LIKE %s", bodyField, sqlutil.StringLiteral("%"+filter.Value+"%")), nil

	case "|~":
		// Regex match
		return fmt.Sprintf("REGEXP_LIKE(%s, %s)", bodyField, sqlutil.StringLiteral(filter.Value)), nil

	case "!~":
		// Regex not match
		return fmt.Sprintf("NOT REGEXP_LIKE(%s, %s)", bodyField, sqlutil.StringLiteral(filter.Value)), nil

	default:
		return "", fmt.Errorf("unsupported line filter operator: %s", filter.Operator)
	}
}

// applyMetricFunction applies a metric function to the base SQL
func (t *Translator) applyMetricFunction(baseSQL string, function string) (string, error) {
	switch strings.ToLower(function) {
	case "count_over_time":
		// Count the number of log lines
		return strings.Replace(baseSQL, "SELECT *", "SELECT COUNT(*)", 1), nil

	case "rate":
		// Rate of log lines per second (simplified - real rate is more complex)
		return strings.Replace(baseSQL, "SELECT *", "SELECT COUNT(*)", 1), nil

	case "bytes_over_time":
		// Total bytes (sum of log line lengths)
		return strings.Replace(baseSQL, "SELECT *", "SELECT SUM(LENGTH(body))", 1), nil

	case "bytes_rate":
		// Bytes per second (simplified)
		return strings.Replace(baseSQL, "SELECT *", "SELECT SUM(LENGTH(body))", 1), nil

	default:
		return "", fmt.Errorf("unsupported metric function: %s", function)
	}
}

// applyAggregation applies aggregation to the SQL
func (t *Translator) applyAggregation(baseSQL string, agg *Aggregator) (string, error) {
	// Extract the current aggregation function (COUNT, SUM, etc.)
	// This is a simplified approach - assumes the function is already in the SQL

	if len(agg.Grouping) == 0 {
		// No grouping, just aggregation (already done by metric function)
		return baseSQL, nil
	}

	// Build grouping fields (reuse common code!)
	groupFields := make([]string, 0, len(agg.Grouping))
	for _, label := range agg.Grouping {
		nativeColumn := common.GetLogNativeColumn(label)
		if nativeColumn != "" {
			groupFields = append(groupFields, nativeColumn)
		} else {
			groupFields = append(groupFields, fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, '$.%s', 'STRING')", label))
		}
	}

	// Build new SELECT clause with grouping
	selectClause := strings.Join(groupFields, ", ")

	// Extract the aggregation function from the current SELECT
	if strings.Contains(baseSQL, "COUNT(*)") {
		selectClause += ", COUNT(*)"
	} else if strings.Contains(baseSQL, "SUM(") {
		// Extract the SUM clause - need to find matching closing paren
		start := strings.Index(baseSQL, "SUM(")
		depth := 0
		end := -1
		for i := start; i < len(baseSQL); i++ {
			if baseSQL[i] == '(' {
				depth++
			} else if baseSQL[i] == ')' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
		if end == -1 {
			return "", fmt.Errorf("malformed SUM expression")
		}
		sumClause := baseSQL[start : end+1]
		selectClause += ", " + sumClause
	}

	// Replace SELECT clause
	sql := strings.Replace(baseSQL, strings.Split(baseSQL, " FROM ")[0], "SELECT "+selectClause, 1)

	// Add GROUP BY
	sql += " GROUP BY " + strings.Join(groupFields, ", ")

	return sql, nil
}
