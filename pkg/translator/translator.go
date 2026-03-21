package translator

import (
	"fmt"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/oql"
)

// Translator translates OQL queries to Pinot SQL
type Translator struct {
	tenantID int
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

		case *oql.ExpandOp:
			// Expand requires a follow-up query
			// First query gets the initial results, then we expand based on trace_id
			expandSQL, err := t.translateExpand(tableName, sql, v)
			if err != nil {
				return nil, fmt.Errorf("failed to translate expand: %w", err)
			}
			sql = expandSQL

		case *oql.CorrelateOp:
			// Correlate requires additional queries
			correlateQueries, err := t.translateCorrelate(sql, v)
			if err != nil {
				return nil, fmt.Errorf("failed to translate correlate: %w", err)
			}
			queries = append(queries, sql)
			queries = append(queries, correlateQueries...)
			return queries, nil

		case *oql.GetExemplarsOp:
			// Get exemplars extracts trace_ids from metrics
			sql = t.translateGetExemplars(sql)

		case *oql.SwitchContextOp:
			// Switch context changes the table we're querying
			// This is complex - simplified implementation
			newTable := t.getTableName(v.Signal)
			sql = fmt.Sprintf("SELECT * FROM %s WHERE tenant_id = %d", newTable, t.tenantID)

		case *oql.ExtractOp:
			// Extract selects specific fields
			sql = strings.Replace(sql, "SELECT *", fmt.Sprintf("SELECT %s AS %s", v.Field, v.Alias), 1)

		case *oql.FilterOp:
			// Filter is like where but for refining results
			filterSQL, err := t.translateCondition(v.Condition)
			if err != nil {
				return nil, fmt.Errorf("failed to translate filter: %w", err)
			}
			sql += " AND " + filterSQL

		default:
			return nil, fmt.Errorf("unsupported operation: %T", op)
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

// translateBinaryCondition translates a binary condition
func (t *Translator) translateBinaryCondition(cond *oql.BinaryCondition) (string, error) {
	field := cond.Left
	operator := cond.Operator
	value := cond.Right

	// Handle special field names (e.g., attributes.error -> attributes['error'])
	if strings.Contains(field, ".") {
		parts := strings.SplitN(field, ".", 2)
		if parts[0] == "attributes" || parts[0] == "resource_attributes" {
			field = fmt.Sprintf("%s['%s']", parts[0], parts[1])
		}
	}

	// Format the value
	valueStr := t.formatValue(value)

	return fmt.Sprintf("%s %s %s", field, operator, valueStr), nil
}

// formatValue formats a value for SQL
func (t *Translator) formatValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
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
		return fmt.Sprintf("'%v'", v)
	}
}

// translateExpand translates an expand operation
func (t *Translator) translateExpand(tableName, baseSQL string, expand *oql.ExpandOp) (string, error) {
	if expand.Type != "trace" {
		return "", fmt.Errorf("unsupported expand type: %s", expand.Type)
	}

	// Expand trace: get all spans with the same trace_id
	// This requires a subquery to get the trace_ids first
	sql := fmt.Sprintf(
		"SELECT * FROM %s WHERE tenant_id = %d AND trace_id IN (SELECT DISTINCT trace_id FROM (%s))",
		tableName,
		t.tenantID,
		baseSQL,
	)

	return sql, nil
}

// translateCorrelate translates a correlate operation
func (t *Translator) translateCorrelate(baseSQL string, correlate *oql.CorrelateOp) ([]string, error) {
	queries := make([]string, 0)

	// For each signal to correlate, generate a query that joins on trace_id
	for _, signal := range correlate.Signals {
		tableName := t.getTableName(signal)

		// Get trace_ids from the base query
		sql := fmt.Sprintf(
			"SELECT * FROM %s WHERE tenant_id = %d AND trace_id IN (SELECT DISTINCT trace_id FROM (%s))",
			tableName,
			t.tenantID,
			baseSQL,
		)

		queries = append(queries, sql)
	}

	return queries, nil
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
