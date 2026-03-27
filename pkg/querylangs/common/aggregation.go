package common

import (
	"fmt"
	"strings"

	"github.com/prometheus/prometheus/promql/parser"
)

// AggregationInfo contains information about an aggregation operation
type AggregationInfo struct {
	Function string   // SQL function name (SUM, AVG, etc.)
	Field    string   // Field to aggregate (or empty for COUNT)
	Grouping []string // Group by fields
	Alias    string   // Optional alias for the result
}

// TranslateAggregation translates a PromQL aggregation operator to SQL function
func TranslateAggregation(op parser.ItemType, field string) (string, error) {
	switch op {
	case parser.SUM:
		if field == "" {
			return "", fmt.Errorf("sum requires a field")
		}
		return fmt.Sprintf("SUM(%s)", field), nil
	case parser.AVG:
		if field == "" {
			return "", fmt.Errorf("avg requires a field")
		}
		return fmt.Sprintf("AVG(%s)", field), nil
	case parser.MIN:
		if field == "" {
			return "", fmt.Errorf("min requires a field")
		}
		return fmt.Sprintf("MIN(%s)", field), nil
	case parser.MAX:
		if field == "" {
			return "", fmt.Errorf("max requires a field")
		}
		return fmt.Sprintf("MAX(%s)", field), nil
	case parser.COUNT:
		if field == "" {
			return "COUNT(*)", nil
		}
		return fmt.Sprintf("COUNT(%s)", field), nil
	case parser.STDDEV:
		if field == "" {
			return "", fmt.Errorf("stddev requires a field")
		}
		return fmt.Sprintf("STDDEV(%s)", field), nil
	case parser.STDVAR:
		if field == "" {
			return "", fmt.Errorf("stdvar requires a field")
		}
		return fmt.Sprintf("VARIANCE(%s)", field), nil
	default:
		return "", fmt.Errorf("unsupported aggregation function: %s", op)
	}
}

// BuildAggregationSQL builds a complete SQL SELECT clause with aggregation and grouping
func BuildAggregationSQL(baseSQL string, info AggregationInfo) string {
	var selectClause string
	var groupByClause string

	if len(info.Grouping) > 0 {
		// Aggregation with grouping
		selectClause = strings.Join(info.Grouping, ", ") + ", " + info.Function
		groupByClause = " GROUP BY " + strings.Join(info.Grouping, ", ")
	} else {
		// Just aggregation, no grouping
		selectClause = info.Function
	}

	// Add alias if provided
	if info.Alias != "" {
		selectClause += " AS " + info.Alias
	}

	// Replace SELECT * with actual SELECT clause
	sql := strings.Replace(baseSQL, "SELECT *", "SELECT "+selectClause, 1)
	sql += groupByClause

	return sql
}
