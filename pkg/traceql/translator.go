package traceql

import (
	"fmt"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
)

// Translator translates TraceQL queries to Pinot SQL
type Translator struct {
	tenantID int
	start    *time.Time // Optional start time for range queries
	end      *time.Time // Optional end time for range queries
}

// NewTranslator creates a new TraceQL to SQL translator
func NewTranslator(tenantID int) *Translator {
	return &Translator{tenantID: tenantID}
}

// TranslateQuery translates a TraceQL query to Pinot SQL
func (t *Translator) TranslateQuery(traceql string) ([]string, error) {
	// Parse the TraceQL query
	parser := NewParser(traceql)
	query, err := parser.Parse()
	if err != nil {
		return nil, fmt.Errorf("failed to parse TraceQL: %w", err)
	}

	// Translate based on expression type
	switch expr := query.Expr.(type) {
	case *SpanFilterExpr:
		sql, err := t.translateSpanFilterExpr(expr)
		if err != nil {
			return nil, err
		}
		return []string{sql}, nil

	case *AggregateExpr:
		sql, err := t.translateAggregateExpr(expr)
		if err != nil {
			return nil, err
		}
		return []string{sql}, nil

	case *ScalarExpr:
		sql := t.translateScalarExpr(expr)
		return []string{sql}, nil

	default:
		return nil, fmt.Errorf("unsupported TraceQL expression type: %T", expr)
	}
}

// TranslateQueryWithTimeRange translates a TraceQL query to Pinot SQL with time range filter
func (t *Translator) TranslateQueryWithTimeRange(traceql string, start, end *time.Time) ([]string, error) {
	// Store time range in translator
	t.start = start
	t.end = end

	// Use regular translation
	return t.TranslateQuery(traceql)
}

// translateSpanFilterExpr translates a span filter expression to SQL
// Example: {span.http.status_code = 500 && duration > 100ms}
func (t *Translator) translateSpanFilterExpr(expr *SpanFilterExpr) (string, error) {
	// Start with base query
	sql := fmt.Sprintf("SELECT * FROM otel_spans WHERE tenant_id = %d", t.tenantID)

	// Add each condition
	for _, condition := range expr.Conditions {
		conditionSQL, err := t.translateCondition(condition)
		if err != nil {
			return "", err
		}
		sql += " AND " + conditionSQL
	}

	// Add time range filter if provided
	if t.start != nil && t.end != nil {
		startMillis := t.start.UnixMilli()
		endMillis := t.end.UnixMilli()
		sql += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	}

	// Order by timestamp descending (most recent first)
	sql += " ORDER BY \"timestamp\" DESC"

	return sql, nil
}

// translateCondition translates a single condition to SQL
func (t *Translator) translateCondition(condition Condition) (string, error) {
	// Get the SQL field reference for this field
	fieldRef, err := t.translateFieldExpr(condition.Field)
	if err != nil {
		return "", err
	}

	// Special handling for duration comparisons (convert to nanoseconds)
	if condition.Field.Type == "intrinsic" && condition.Field.Name == "duration" {
		if durationValue, ok := condition.Value.(time.Duration); ok {
			// Convert duration to nanoseconds for comparison
			nanos := durationValue.Nanoseconds()
			return fmt.Sprintf("%s %s %d", fieldRef, condition.Operator, nanos), nil
		}
	}

	// Special handling for status values (convert to Pinot status codes)
	if condition.Field.Type == "intrinsic" && condition.Field.Name == "status" {
		if statusValue, ok := condition.Value.(StatusValue); ok {
			statusCode := translateStatusValue(statusValue)
			return fmt.Sprintf("%s %s %s", fieldRef, condition.Operator, sqlutil.StringLiteral(statusCode)), nil
		}
	}

	// Handle different operators and value types
	switch condition.Operator {
	case "=", "!=", ">", "<", ">=", "<=":
		valueStr, err := t.formatValue(condition.Value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s %s", fieldRef, condition.Operator, valueStr), nil

	case "=~":
		// Regex match
		valueStr, err := t.formatValue(condition.Value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("REGEXP_LIKE(%s, %s)", fieldRef, valueStr), nil

	case "!~":
		// Regex not match
		valueStr, err := t.formatValue(condition.Value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("NOT REGEXP_LIKE(%s, %s)", fieldRef, valueStr), nil

	default:
		return "", fmt.Errorf("unsupported operator: %s", condition.Operator)
	}
}

// translateFieldExpr translates a field expression to SQL column reference
func (t *Translator) translateFieldExpr(field FieldExpr) (string, error) {
	switch field.Type {
	case "intrinsic":
		return t.getIntrinsicColumn(field.Name), nil

	case "span":
		return t.getSpanAttributeColumn(field.Name), nil

	case "resource":
		return t.getResourceAttributeColumn(field.Name), nil

	default:
		return "", fmt.Errorf("unsupported field type: %s", field.Type)
	}
}

// getIntrinsicColumn maps intrinsic field names to native columns
func (t *Translator) getIntrinsicColumn(name string) string {
	// Map TraceQL intrinsic fields to Pinot columns
	intrinsicMap := map[string]string{
		"duration": "duration",
		"name":     "name",
		"status":   "status_code",
		"kind":     "kind",
		"traceid":  "trace_id",
		"spanid":   "span_id",
	}

	if column, ok := intrinsicMap[name]; ok {
		return column
	}

	// Unknown intrinsic field - shouldn't happen if parser is correct
	return name
}

// getSpanAttributeColumn maps span attribute names to native columns or JSON extraction
func (t *Translator) getSpanAttributeColumn(attrName string) string {
	// Map OTel semantic conventions to native columns
	// These are extracted for performance (10-100x faster than JSON)
	nativeColumns := map[string]string{
		"http.method":           "http_method",
		"http.status_code":      "http_status_code",
		"http.route":            "http_route",
		"http.target":           "http_target",
		"db.system":             "db_system",
		"db.statement":          "db_statement",
		"messaging.system":      "messaging_system",
		"messaging.destination": "messaging_destination",
		"rpc.service":           "rpc_service",
		"rpc.method":            "rpc_method",
		"error":                 "error",
	}

	if nativeColumn, ok := nativeColumns[attrName]; ok {
		return nativeColumn
	}

	// Not a native column - use JSON extraction
	// Convert dotted attribute name to JSON path
	// Example: custom.field.name → $.custom.field.name
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, '$.%s', 'STRING')", attrName)
}

// getResourceAttributeColumn maps resource attribute names to native columns or JSON extraction
func (t *Translator) getResourceAttributeColumn(attrName string) string {
	// Map OTel resource semantic conventions to native columns
	nativeColumns := map[string]string{
		"service.name": "service_name",
	}

	if nativeColumn, ok := nativeColumns[attrName]; ok {
		return nativeColumn
	}

	// Not a native column - use JSON extraction from resource_attributes
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(resource_attributes, '$.%s', 'STRING')", attrName)
}

// formatValue formats a value for SQL
func (t *Translator) formatValue(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return sqlutil.StringLiteral(v), nil
	case float64:
		return fmt.Sprintf("%f", v), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case time.Duration:
		// Duration in nanoseconds
		return fmt.Sprintf("%d", v.Nanoseconds()), nil
	case StatusValue:
		// Status value as string
		return sqlutil.StringLiteral(translateStatusValue(v)), nil
	default:
		return "", fmt.Errorf("unsupported value type: %T", value)
	}
}

// translateStatusValue converts TraceQL status values to OTel status codes
// These match the .String() output from OTLP Status.Code enum: "Unset", "Ok", "Error"
func translateStatusValue(status StatusValue) string {
	switch status {
	case StatusUnset:
		return "Unset"
	case StatusOK:
		return "Ok"
	case StatusError:
		return "Error"
	default:
		return string(status)
	}
}

// translateAggregateExpr translates an aggregation expression to SQL
// Example: count() by (span.service.name)
func (t *Translator) translateAggregateExpr(expr *AggregateExpr) (string, error) {
	// Start with base query
	sql := fmt.Sprintf("SELECT * FROM otel_spans WHERE tenant_id = %d", t.tenantID)

	// Add time range filter if provided
	if t.start != nil && t.end != nil {
		startMillis := t.start.UnixMilli()
		endMillis := t.end.UnixMilli()
		sql += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	}

	// Determine aggregation function
	var aggFunc string
	switch strings.ToLower(expr.Function) {
	case "count":
		aggFunc = "COUNT(*)"
	case "sum":
		aggFunc = "SUM(duration)" // Sum durations
	case "avg":
		aggFunc = "AVG(duration)" // Average duration
	case "min":
		aggFunc = "MIN(duration)" // Min duration
	case "max":
		aggFunc = "MAX(duration)" // Max duration
	default:
		return "", fmt.Errorf("unsupported aggregation function: %s", expr.Function)
	}

	// Build SELECT clause with grouping if needed
	var selectClause string
	var groupByClause string

	if len(expr.Grouping) > 0 {
		// Build grouping fields
		groupFields := make([]string, 0, len(expr.Grouping))
		for _, field := range expr.Grouping {
			fieldSQL, err := t.translateGroupingField(field)
			if err != nil {
				return "", err
			}
			groupFields = append(groupFields, fieldSQL)
		}

		selectClause = strings.Join(groupFields, ", ") + ", " + aggFunc
		groupByClause = " GROUP BY " + strings.Join(groupFields, ", ")
	} else {
		// Just aggregation, no grouping
		selectClause = aggFunc
	}

	// Replace SELECT * with actual SELECT clause
	sql = strings.Replace(sql, "SELECT *", "SELECT "+selectClause, 1)
	sql += groupByClause

	return sql, nil
}

// translateGroupingField translates a grouping field to SQL column reference
// Example: span.service.name, resource.environment, duration
func (t *Translator) translateGroupingField(field string) (string, error) {
	// Parse the field string to determine type
	if IsIntrinsic(field) {
		// Intrinsic field like duration, name, status
		return t.getIntrinsicColumn(field), nil
	}

	if strings.HasPrefix(field, "span.") {
		// Span attribute
		attrName := strings.TrimPrefix(field, "span.")
		return t.getSpanAttributeColumn(attrName), nil
	}

	if strings.HasPrefix(field, "resource.") {
		// Resource attribute
		attrName := strings.TrimPrefix(field, "resource.")
		return t.getResourceAttributeColumn(attrName), nil
	}

	return "", fmt.Errorf("invalid grouping field: %s", field)
}

// translateScalarExpr translates a scalar expression to SQL
// This handles connection tests like 1+1 from Grafana
func (t *Translator) translateScalarExpr(expr *ScalarExpr) string {
	// Return a SQL query that produces this scalar value
	// Pinot requires a FROM clause, so we use otel_spans with LIMIT 1
	return fmt.Sprintf("SELECT %f AS value FROM otel_spans LIMIT 1", expr.Value)
}
