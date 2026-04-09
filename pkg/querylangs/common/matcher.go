package common

import (
	"fmt"

	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
	"github.com/prometheus/prometheus/model/labels"
)

// TranslateLabelMatcher translates a Prometheus label matcher to SQL condition
// This is shared between PromQL and LogQL
func TranslateLabelMatcher(matcher *labels.Matcher, getNativeColumn func(string) string) (string, error) {
	labelName := matcher.Name
	labelValue := matcher.Value

	// Check if this label maps to a native column
	var fieldRef string
	if nativeCol := getNativeColumn(labelName); nativeCol != "" {
		fieldRef = nativeCol
	} else {
		// Use JSON extraction for attributes
		fieldRef = fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, %s, 'STRING')", sqlutil.JSONObjectKeyPathLiteral(labelName))
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

// GetMetricNativeColumn maps Prometheus metric label names to native Pinot columns
func GetMetricNativeColumn(labelName string) string {
	nativeColumns := map[string]string{
		// Metric attributes
		"job":         "job",
		"instance":    "instance",
		"environment": "environment",

		// Service attributes
		"service":      "service_name",
		"service_name": "service_name",

		// HTTP attributes
		"method":      "http_method",
		"status":      "http_status_code",
		"status_code": "http_status_code",
		"route":       "http_route",

		// Host attributes
		"host":      "host_name",
		"host_name": "host_name",
	}

	if nativeCol, ok := nativeColumns[labelName]; ok {
		return nativeCol
	}
	return ""
}

// GetLogNativeColumn maps log label names to native Pinot columns
func GetLogNativeColumn(labelName string) string {
	nativeColumns := map[string]string{
		// Trace correlation (CRITICAL for correlate operations!)
		"trace_id": "trace_id",
		"traceId":  "trace_id",
		"span_id":  "span_id",
		"spanId":   "span_id",

		// Severity/Level
		"severity":      "severity_text",
		"severity_text": "severity_text",
		"level":         "log_level",
		"log_level":     "log_level",

		// Service & Host
		"service":      "service_name",
		"service_name": "service_name",
		"host":         "host_name",
		"host_name":    "host_name",

		// Source/Filename
		"source":     "log_source",
		"log_source": "log_source",
		"filename":   "log_source",

		// Prometheus/Loki common labels
		"job":         "job",
		"instance":    "instance",
		"environment": "environment",
	}

	if nativeCol, ok := nativeColumns[labelName]; ok {
		return nativeCol
	}
	return ""
}
