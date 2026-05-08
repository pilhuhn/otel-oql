package translator

import (
	"fmt"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/oql"
	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
)

// Translator translates OQL queries to Pinot SQL
type Translator struct {
	tenantID      int
	groupByFields []string // Track group by fields for aggregations
}

// NewTranslator creates a new OQL to SQL translator
func NewTranslator(tenantID int) *Translator {
	return &Translator{
		tenantID: tenantID,
	}
}

// TranslateQuery translates an OQL query to one or more Pinot SQL queries
func (t *Translator) TranslateQuery(query *oql.Query) ([]string, error) {
	queries := make([]string, 0)

	// Start with the base table query
	tableName := t.getTableName(query.Signal)
	sql := fmt.Sprintf("SELECT * FROM %s WHERE tenant_id = %d", tableName, t.tenantID)

	// For signal=trace, filter to show only root spans (one row per trace)
	if query.Signal == oql.SignalTraces {
		sql += " AND (parent_span_id IS NULL OR parent_span_id = '' OR parent_span_id = '0' OR parent_span_id = '00000000000000000000000000000000')"
	}

	// Track if an explicit sort operation was provided
	hasSortOp := false

	// Process operations
	for _, op := range query.Operations {
		switch v := op.(type) {
		case *oql.WhereOp:
			whereSQL, err := t.translateCondition(v.Condition)
			if err != nil {
				return nil, fmt.Errorf("failed to translate where: %w", err)
			}
			sql += " AND " + whereSQL

		case *oql.LimitOp:
			sql += fmt.Sprintf(" LIMIT %d", v.Count)

		case *oql.SortOp:
			hasSortOp = true
			orderByClauses := make([]string, 0, len(v.Fields))
			for _, field := range v.Fields {
				direction := "ASC"
				if field.Desc {
					direction = "DESC"
				}
				orderByClauses = append(orderByClauses, fmt.Sprintf("%s %s", field.Field, direction))
			}
			sql += fmt.Sprintf(" ORDER BY %s", strings.Join(orderByClauses, ", "))

		case *oql.ExpandOp:
			// Expand requires a follow-up query
			// First query gets the initial results, then we expand based on trace_id
			expandSQL, err := t.translateExpand(tableName, sql, v)
			if err != nil {
				return nil, fmt.Errorf("failed to translate expand: %w", err)
			}
			sql = expandSQL

		case *oql.CorrelateOp:
			// Correlate requires two-step execution like expand
			// Use a special marker format
			correlateSQL := t.translateCorrelate(sql, v, tableName)
			sql = correlateSQL

		case *oql.GetExemplarsOp:
			// Get exemplars extracts trace_ids from metrics
			sql = t.translateGetExemplars(sql)

		case *oql.SwitchContextOp:
			// Switch context changes the table we're querying
			// This is complex - simplified implementation
			newTable := t.getTableName(v.Signal)
			sql = fmt.Sprintf("SELECT * FROM %s WHERE tenant_id = %d", newTable, t.tenantID)

		case *oql.ExtractOp:
			fieldSQL, err := t.translateFieldReference(v.Field)
			if err != nil {
				return nil, fmt.Errorf("extract: %w", err)
			}
			if err := validatePlainIdentifier(v.Alias); err != nil {
				return nil, fmt.Errorf("extract alias: %w", err)
			}
			sql = strings.Replace(sql, "SELECT *", fmt.Sprintf("SELECT %s AS %s", fieldSQL, v.Alias), 1)

		case *oql.FilterOp:
			// Filter is like where but for refining results
			filterSQL, err := t.translateCondition(v.Condition)
			if err != nil {
				return nil, fmt.Errorf("failed to translate filter: %w", err)
			}
			sql += " AND " + filterSQL

		case *oql.GroupByOp:
			t.groupByFields = nil
			for _, f := range v.Fields {
				expr, err := t.translateFieldReference(strings.TrimSpace(f))
				if err != nil {
					return nil, fmt.Errorf("group by: %w", err)
				}
				t.groupByFields = append(t.groupByFields, expr)
			}

		case *oql.AggregateOp:
			var err error
			sql, err = t.translateAggregate(sql, v)
			if err != nil {
				return nil, err
			}

		case *oql.SinceOp:
			// Since adds time range filter
			sinceSQL, err := t.translateSince(v)
			if err != nil {
				return nil, fmt.Errorf("failed to translate since: %w", err)
			}
			sql += " AND " + sinceSQL

		case *oql.BetweenOp:
			// Between adds time range filter
			betweenSQL, err := t.translateBetween(v)
			if err != nil {
				return nil, fmt.Errorf("failed to translate between: %w", err)
			}
			sql += " AND " + betweenSQL

		default:
			return nil, fmt.Errorf("unsupported operation: %T", op)
		}
	}

	// Add default ordering for event-based signals if no explicit sort was provided
	// Only add for regular queries, not special marker queries (expand, correlate)
	if !hasSortOp && !strings.Contains(sql, "__EXPAND_") && !strings.Contains(sql, "__CORRELATE__") {
		if query.Signal == oql.SignalSpans || query.Signal == oql.SignalTraces || query.Signal == oql.SignalLogs {
			sql += " ORDER BY \"timestamp\" DESC"
		}
	}

	queries = append(queries, sql)
	return queries, nil
}

// translateCondition translates an OQL condition to SQL WHERE clause
func (t *Translator) translateCondition(cond oql.Condition) (string, error) {
	switch v := cond.(type) {
	case *oql.BinaryCondition:
		return t.translateBinaryCondition(v)
	case *oql.AndCondition:
		parts := make([]string, 0)
		for _, c := range v.Conditions {
			part, err := t.translateCondition(c)
			if err != nil {
				return "", err
			}
			parts = append(parts, part)
		}
		return "(" + strings.Join(parts, " AND ") + ")", nil
	case *oql.OrCondition:
		parts := make([]string, 0)
		for _, c := range v.Conditions {
			part, err := t.translateCondition(c)
			if err != nil {
				return "", err
			}
			parts = append(parts, part)
		}
		return "(" + strings.Join(parts, " OR ") + ")", nil
	default:
		return "", fmt.Errorf("unsupported condition type: %T", cond)
	}
}

// translateFieldReference maps an OQL field (column or attributes.key) to a Pinot SQL expression.
func (t *Translator) translateFieldReference(field string) (string, error) {
	field = strings.TrimSpace(field)
	if field == "" {
		return "", fmt.Errorf("empty field")
	}

	// Map field aliases to their canonical column names
	// This allows users to use shorter display names (matching table headers) in queries
	fieldAliases := map[string]string{
		"service": "service_name", // Header shows "service" but column is "service_name"
		"status":  "http_status_code",
		"method":  "http_method",
		"route":   "http_route",
	}
	if canonical, ok := fieldAliases[field]; ok {
		field = canonical
	}

	if strings.Contains(field, ".") {
		parts := strings.SplitN(field, ".", 2)
		prefix := parts[0]
		rest := parts[1]
		if rest == "" {
			return "", fmt.Errorf("invalid field: missing name after %q", prefix)
		}
		if prefix == "attributes" || prefix == "resource_attributes" {
			if err := validateAttributeKey(rest); err != nil {
				return "", err
			}
			if nativeColumn := t.getNativeColumn(rest); nativeColumn != "" {
				return nativeColumn, nil
			}
			return fmt.Sprintf("JSON_EXTRACT_SCALAR(%s, %s, 'STRING')", prefix, jsonPathLiteral(rest)), nil
		}
		return "", fmt.Errorf("invalid field %q: only attributes.<key> or resource_attributes.<key> may use dot notation", field)
	}
	if err := validatePlainIdentifier(field); err != nil {
		return "", err
	}
	// Quote reserved keywords (timestamp is reserved in Pinot SQL)
	if field == "timestamp" {
		return `"timestamp"`, nil
	}
	return field, nil
}

// translateBinaryCondition translates a binary condition
func (t *Translator) translateBinaryCondition(cond *oql.BinaryCondition) (string, error) {
	fieldSQL, err := t.translateFieldReference(cond.Left)
	if err != nil {
		return "", err
	}

	valueStr := t.formatValue(cond.Right)

	// Check if there was a parse error
	if strings.HasPrefix(valueStr, "'PARSE_ERROR:") {
		errMsg := strings.TrimPrefix(valueStr, "'PARSE_ERROR: ")
		errMsg = strings.TrimSuffix(errMsg, "'")
		return "", fmt.Errorf("%s", errMsg)
	}

	// Detect wildcards in string values and convert to LIKE
	sqlOperator := cond.Operator
	if str, ok := cond.Right.(string); ok && (strings.Contains(str, "*") || strings.Contains(str, "%")) {
		// String value contains wildcards - use LIKE instead of =
		if cond.Operator == "=" || cond.Operator == "==" {
			sqlOperator = "LIKE"
		} else if cond.Operator == "!=" {
			sqlOperator = "NOT LIKE"
		}
		// Convert * to % for SQL LIKE syntax (in the already-formatted value)
		valueStr = strings.ReplaceAll(valueStr, "*", "%")
	}

	sqlOp, err := t.convertOperator(sqlOperator)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s %s", fieldSQL, sqlOp, valueStr), nil
}

// convertOperator converts OQL operators to SQL operators
func (t *Translator) convertOperator(op string) (string, error) {
	switch op {
	case "==":
		return "=", nil
	case "!=":
		return "<>", nil
	case ">", "<", ">=", "<=", "=":
		return op, nil
	case "LIKE", "NOT LIKE":
		return op, nil
	default:
		return "", fmt.Errorf("unsupported operator: %s", op)
	}
}

// getNativeColumn returns the native column name if the attribute has been extracted
func (t *Translator) getNativeColumn(attributeKey string) string {
	// Map of OTel semantic conventions to native columns
	nativeColumns := map[string]string{
		// Span attributes
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

		// Resource attributes (service.name is in both spans and metrics)
		"service.name": "service_name",
		"host.name":    "host_name",

		// Metric attributes
		"job":         "job",
		"instance":    "instance",
		"environment": "environment",

		// Log attributes
		"log.level":  "log_level",
		"log.source": "log_source",
	}

	if nativeCol, ok := nativeColumns[attributeKey]; ok {
		return nativeCol
	}

	return ""
}

// formatValue formats a value for SQL
func (t *Translator) formatValue(value interface{}) string {
	switch v := value.(type) {
	case *oql.NowExpression:
		return "now()"
	case *oql.TimeArithmeticExpression:
		return t.formatTimeArithmetic(v)
	case string:
		return sqlutil.StringLiteral(v)
	case int, int64, int32:
		return fmt.Sprintf("%d", v)
	case float64, float32:
		return fmt.Sprintf("%f", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case time.Duration:
		// Convert duration to nanoseconds
		return fmt.Sprintf("%d", v.Nanoseconds())
	default:
		return sqlutil.StringLiteral(fmt.Sprintf("%v", v))
	}
}

// formatTimeArithmetic formats time arithmetic expressions (e.g., now() - 1h)
func (t *Translator) formatTimeArithmetic(expr *oql.TimeArithmeticExpression) string {
	// Parse the offset duration
	duration, err := time.ParseDuration(expr.Offset)
	if err != nil {
		// This should not happen since parser validates it, but handle gracefully
		return fmt.Sprintf("'PARSE_ERROR: %v'", err)
	}

	// Convert to milliseconds (Pinot timestamp unit)
	millis := duration.Milliseconds()

	// Format the base expression
	var base string
	switch expr.Base.(type) {
	case *oql.NowExpression:
		base = "now()"
	default:
		base = "now()" // Default to now()
	}

	// Build the arithmetic expression
	if expr.Operator == "-" {
		return fmt.Sprintf("(%s - %d)", base, millis)
	} else if expr.Operator == "+" {
		return fmt.Sprintf("(%s + %d)", base, millis)
	}

	return base
}

// translateExpand translates an expand operation
// Returns a special marker SQL that the API server will recognize and execute in two steps
func (t *Translator) translateExpand(tableName, baseSQL string, expand *oql.ExpandOp) (string, error) {
	if expand.Type != "trace" {
		return "", fmt.Errorf("unsupported expand type: %s", expand.Type)
	}

	// Pinot doesn't support subqueries in IN clauses, so we return a marker
	// The API server will:
	// 1. Execute the base query to get trace_ids
	// 2. Build an IN clause with those trace_ids
	// 3. Execute the final query

	// Use a special marker format: __EXPAND_TRACE__<baseSQL>__END_EXPAND__
	sql := fmt.Sprintf("__EXPAND_TRACE__%s__TABLE__%s__END_EXPAND__", baseSQL, tableName)

	return sql, nil
}

// translateCorrelate translates a correlate operation
// Returns a special marker SQL that the API server will recognize and execute in two steps
func (t *Translator) translateCorrelate(baseSQL string, correlate *oql.CorrelateOp, currentTable string) string {
	// Pinot doesn't support subqueries in IN clauses, so we return a marker
	// The API server will:
	// 1. Execute the base query to get trace_ids
	// 2. For each signal to correlate, build an IN clause with those trace_ids
	// 3. Execute the queries and return combined results

	// Encode the signals as comma-separated list
	signalNames := make([]string, 0)
	for _, signal := range correlate.Signals {
		signalNames = append(signalNames, string(signal))
	}
	signals := strings.Join(signalNames, ",")

	// Use a special marker format: __CORRELATE__<signals>__BASE__<baseSQL>__TABLE__<currentTable>__END_CORRELATE__
	sql := fmt.Sprintf("__CORRELATE__%s__BASE__%s__TABLE__%s__END_CORRELATE__", signals, baseSQL, currentTable)

	return sql
}

// translateGetExemplars translates get_exemplars operation
func (t *Translator) translateGetExemplars(baseSQL string) string {
	// Get exemplars extracts the trace_ids from metrics
	// Modify the query to select exemplar_trace_id
	sql := strings.Replace(
		baseSQL,
		"SELECT *",
		"SELECT exemplar_trace_id, exemplar_span_id",
		1,
	)
	sql += " AND exemplar_trace_id IS NOT NULL"

	return sql
}

// getTableName maps a signal type to a Pinot table name
func (t *Translator) getTableName(signal oql.SignalType) string {
	switch signal {
	case oql.SignalMetrics:
		return "otel_metrics"
	case oql.SignalLogs:
		return "otel_logs"
	case oql.SignalSpans, oql.SignalTraces:
		return "otel_spans"
	default:
		return "otel_spans"
	}
}

// translateAggregate translates an aggregate operation
func (t *Translator) translateAggregate(baseSQL string, agg *oql.AggregateOp) (string, error) {
	fn := strings.ToLower(strings.TrimSpace(agg.Function))
	if fn == "" {
		return "", fmt.Errorf("aggregate function is required")
	}
	allowed := map[string]struct{}{
		"avg": {}, "min": {}, "max": {}, "count": {}, "sum": {},
	}
	if _, ok := allowed[fn]; !ok {
		return "", fmt.Errorf("unsupported aggregate function: %s", agg.Function)
	}

	var fieldSQL string
	if agg.Field != "" {
		var err error
		fieldSQL, err = t.translateFieldReference(agg.Field)
		if err != nil {
			return "", fmt.Errorf("aggregate field: %w", err)
		}
	}

	var aggFunc string
	switch fn {
	case "avg":
		if agg.Field == "" {
			return "", fmt.Errorf("avg requires a field")
		}
		aggFunc = fmt.Sprintf("AVG(%s)", fieldSQL)
	case "min":
		if agg.Field == "" {
			return "", fmt.Errorf("min requires a field")
		}
		aggFunc = fmt.Sprintf("MIN(%s)", fieldSQL)
	case "max":
		if agg.Field == "" {
			return "", fmt.Errorf("max requires a field")
		}
		aggFunc = fmt.Sprintf("MAX(%s)", fieldSQL)
	case "count":
		if agg.Field == "" {
			aggFunc = "COUNT(*)"
		} else {
			aggFunc = fmt.Sprintf("COUNT(%s)", fieldSQL)
		}
	case "sum":
		if agg.Field == "" {
			return "", fmt.Errorf("sum requires a field")
		}
		aggFunc = fmt.Sprintf("SUM(%s)", fieldSQL)
	default:
		return "", fmt.Errorf("unsupported aggregate function: %s", agg.Function)
	}

	if agg.Alias != "" {
		if err := validatePlainIdentifier(agg.Alias); err != nil {
			return "", fmt.Errorf("aggregate alias: %w", err)
		}
		aggFunc = fmt.Sprintf("%s AS %s", aggFunc, agg.Alias)
	}

	var selectClause string
	if len(t.groupByFields) > 0 {
		selectClause = strings.Join(t.groupByFields, ", ") + ", " + aggFunc
	} else {
		selectClause = aggFunc
	}

	sql := strings.Replace(baseSQL, "SELECT *", "SELECT "+selectClause, 1)

	if len(t.groupByFields) > 0 {
		sql += " GROUP BY " + strings.Join(t.groupByFields, ", ")
	}

	return sql, nil
}

// translateGroupBy translates a group by operation
func (t *Translator) translateGroupBy(baseSQL string, groupBy *oql.GroupByOp) string {
	// Add fields to SELECT if using SELECT *
	if strings.Contains(baseSQL, "SELECT *") {
		// Replace SELECT * with SELECT fields, aggregations
		fields := strings.Join(groupBy.Fields, ", ")
		baseSQL = strings.Replace(baseSQL, "SELECT *", "SELECT "+fields, 1)
	}

	// Add GROUP BY clause
	groupByClause := " GROUP BY " + strings.Join(groupBy.Fields, ", ")
	sql := baseSQL + groupByClause
	return sql
}

// translateSince translates a since time range operation
func (t *Translator) translateSince(since *oql.SinceOp) (string, error) {
	duration := since.Duration

	// Check if it's a relative duration (e.g., "1h", "30m")
	if _, err := time.ParseDuration(duration); err == nil {
		// It's a duration like "1h", "30m"
		// Convert to milliseconds and subtract from current time
		d, _ := time.ParseDuration(duration)
		millis := d.Milliseconds()

		// Use Pinot's timestamp functions
		sql := fmt.Sprintf("\"timestamp\" >= (now() - %d)", millis)
		return sql, nil
	}

	// Otherwise, try to parse as a timestamp (e.g., "2024-03-20")
	// Pinot timestamps are in milliseconds since epoch
	ts, err := time.Parse("2006-01-02", duration)
	if err != nil {
		// Try with time as well
		ts, err = time.Parse("2006-01-02T15:04:05", duration)
		if err != nil {
			return "", fmt.Errorf("invalid time format in since: %s", duration)
		}
	}

	millis := ts.UnixMilli()
	sql := fmt.Sprintf("\"timestamp\" >= %d", millis)
	return sql, nil
}

// translateBetween translates a between time range operation
func (t *Translator) translateBetween(between *oql.BetweenOp) (string, error) {
	// Parse start time
	startTime, err := time.Parse("2006-01-02", between.Start)
	if err != nil {
		startTime, err = time.Parse("2006-01-02T15:04:05", between.Start)
		if err != nil {
			return "", fmt.Errorf("invalid start time format: %s", between.Start)
		}
	}

	// Parse end time
	endTime, err := time.Parse("2006-01-02", between.End)
	if err != nil {
		endTime, err = time.Parse("2006-01-02T15:04:05", between.End)
		if err != nil {
			return "", fmt.Errorf("invalid end time format: %s", between.End)
		}
	}

	startMillis := startTime.UnixMilli()
	endMillis := endTime.UnixMilli()

	sql := fmt.Sprintf("\"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	return sql, nil
}
