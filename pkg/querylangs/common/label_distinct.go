package common

import (
	"fmt"

	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
)

// MetricLabelDistinctExpr returns a SQL expression for selecting distinct values of a
// Prometheus-style label from otel_metrics. Label names from the API are never
// interpolated as bare identifiers; unknown labels use JSON_EXTRACT_SCALAR on attributes.
func MetricLabelDistinctExpr(labelName string) string {
	if labelName == "__name__" {
		return "metric_name"
	}
	if col := GetMetricNativeColumn(labelName); col != "" {
		return col
	}
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, %s, 'STRING')", sqlutil.JSONObjectKeyPathLiteral(labelName))
}

// LogLabelDistinctExpr returns a SQL expression for selecting distinct values of a
// Loki-style label from otel_logs. Label names from the API are never interpolated
// as bare identifiers; unknown labels use JSON_EXTRACT_SCALAR on attributes.
func LogLabelDistinctExpr(labelName string) string {
	if col := GetLogNativeColumn(labelName); col != "" {
		return col
	}
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, %s, 'STRING')", sqlutil.JSONObjectKeyPathLiteral(labelName))
}
