package formats

import (
	"fmt"
	"strconv"
	"time"
)

// PrometheusResponse represents a Prometheus API response
type PrometheusResponse struct {
	Status string                 `json:"status"`
	Data   *PrometheusData        `json:"data,omitempty"`
	Error  string                 `json:"error,omitempty"`
	ErrorType string              `json:"errorType,omitempty"`
}

// PrometheusData represents the data field in a Prometheus response
type PrometheusData struct {
	ResultType string             `json:"resultType"`
	Result     []PrometheusResult `json:"result"`
}

// PrometheusResult represents a single result in Prometheus response
type PrometheusResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value,omitempty"`  // For instant queries: [timestamp, "value"]
	Values [][]interface{}   `json:"values,omitempty"` // For range queries: [[timestamp, "value"], ...]
}

// PinotResult represents a result from Pinot query execution
type PinotResult struct {
	SQL     string
	Columns []string
	Rows    [][]interface{}
}

// TransformToPrometheusInstant transforms Pinot results to Prometheus instant query format
func TransformToPrometheusInstant(results []PinotResult, queryTime time.Time) PrometheusResponse {
	if len(results) == 0 {
		return PrometheusResponse{
			Status: "success",
			Data: &PrometheusData{
				ResultType: "vector",
				Result:     []PrometheusResult{},
			},
		}
	}

	// Use the first result (OQL generates multiple queries, but PromQL should be single)
	result := results[0]

	// If query returned no rows, return empty result (not an error)
	if len(result.Rows) == 0 {
		return PrometheusResponse{
			Status: "success",
			Data: &PrometheusData{
				ResultType: "vector",
				Result:     []PrometheusResult{},
			},
		}
	}

	// Find column indices
	valueIdx := findColumn(result.Columns, "value")
	timestampIdx := findColumn(result.Columns, "timestamp")
	metricNameIdx := findColumn(result.Columns, "metric_name")

	if valueIdx == -1 {
		return PrometheusResponse{
			Status:    "error",
			ErrorType: "bad_data",
			Error:     "missing 'value' column in query results",
		}
	}

	// Extract label columns (everything except value, timestamp, metric_name, tenant_id)
	labelColumns := make([]string, 0)
	for _, col := range result.Columns {
		if col != "value" && col != "timestamp" && col != "metric_name" && col != "tenant_id" {
			labelColumns = append(labelColumns, col)
		}
	}

	// Transform rows to Prometheus results
	promResults := make([]PrometheusResult, 0, len(result.Rows))
	for _, row := range result.Rows {
		// Build metric labels
		metric := make(map[string]string)

		// Add metric name if present
		if metricNameIdx != -1 && row[metricNameIdx] != nil {
			metric["__name__"] = fmt.Sprintf("%v", row[metricNameIdx])
		}

		// Add label values
		for _, labelCol := range labelColumns {
			idx := findColumn(result.Columns, labelCol)
			if idx != -1 && row[idx] != nil {
				metric[labelCol] = fmt.Sprintf("%v", row[idx])
			}
		}

		// Extract value
		value := formatValue(row[valueIdx])

		// Extract or use provided timestamp
		var timestamp int64
		if timestampIdx != -1 && row[timestampIdx] != nil {
			timestamp = toUnixSeconds(row[timestampIdx])
		} else {
			timestamp = queryTime.Unix()
		}

		promResults = append(promResults, PrometheusResult{
			Metric: metric,
			Value:  []interface{}{timestamp, value},
		})
	}

	return PrometheusResponse{
		Status: "success",
		Data: &PrometheusData{
			ResultType: "vector",
			Result:     promResults,
		},
	}
}

// TransformToPrometheusRange transforms Pinot results to Prometheus range query format
func TransformToPrometheusRange(results []PinotResult) PrometheusResponse {
	if len(results) == 0 {
		return PrometheusResponse{
			Status: "success",
			Data: &PrometheusData{
				ResultType: "matrix",
				Result:     []PrometheusResult{},
			},
		}
	}

	// Use the first result
	result := results[0]

	// If query returned no rows, return empty result (not an error)
	if len(result.Rows) == 0 {
		return PrometheusResponse{
			Status: "success",
			Data: &PrometheusData{
				ResultType: "matrix",
				Result:     []PrometheusResult{},
			},
		}
	}

	// Find column indices
	valueIdx := findColumn(result.Columns, "value")
	timestampIdx := findColumn(result.Columns, "timestamp")
	// Also check for "ts" which is used in bucketed queries (timestamp is a reserved keyword)
	if timestampIdx == -1 {
		timestampIdx = findColumn(result.Columns, "ts")
	}
	metricNameIdx := findColumn(result.Columns, "metric_name")

	if valueIdx == -1 {
		return PrometheusResponse{
			Status:    "error",
			ErrorType: "bad_data",
			Error:     "missing 'value' column in query results",
		}
	}

	if timestampIdx == -1 {
		return PrometheusResponse{
			Status:    "error",
			ErrorType: "bad_data",
			Error:     "missing 'timestamp' or 'ts' column in query results",
		}
	}

	// Extract label columns (excluding known system columns)
	labelColumns := make([]string, 0)
	for _, col := range result.Columns {
		if col != "value" && col != "timestamp" && col != "ts" && col != "metric_name" && col != "tenant_id" {
			labelColumns = append(labelColumns, col)
		}
	}

	// Group rows by metric labels (time series)
	seriesMap := make(map[string]*PrometheusResult)

	for _, row := range result.Rows {
		// Build metric labels
		metric := make(map[string]string)

		// Add metric name if present
		if metricNameIdx != -1 && row[metricNameIdx] != nil {
			metric["__name__"] = fmt.Sprintf("%v", row[metricNameIdx])
		}

		// Add label values
		for _, labelCol := range labelColumns {
			idx := findColumn(result.Columns, labelCol)
			if idx != -1 && row[idx] != nil {
				metric[labelCol] = fmt.Sprintf("%v", row[idx])
			}
		}

		// Create series key from metric labels
		seriesKey := metricMapToKey(metric)

		// Get or create series
		series, exists := seriesMap[seriesKey]
		if !exists {
			series = &PrometheusResult{
				Metric: metric,
				Values: make([][]interface{}, 0),
			}
			seriesMap[seriesKey] = series
		}

		// Add data point
		value := formatValue(row[valueIdx])
		timestamp := toUnixSeconds(row[timestampIdx])
		series.Values = append(series.Values, []interface{}{timestamp, value})
	}

	// Convert map to slice
	promResults := make([]PrometheusResult, 0, len(seriesMap))
	for _, series := range seriesMap {
		promResults = append(promResults, *series)
	}

	return PrometheusResponse{
		Status: "success",
		Data: &PrometheusData{
			ResultType: "matrix",
			Result:     promResults,
		},
	}
}

// PrometheusError creates a Prometheus error response
func PrometheusError(errorType, message string) PrometheusResponse {
	return PrometheusResponse{
		Status:    "error",
		ErrorType: errorType,
		Error:     message,
	}
}

// PrometheusLabelsResponse represents response for /api/v1/labels
type PrometheusLabelsResponse struct {
	Status    string   `json:"status"`
	Data      []string `json:"data,omitempty"`
	Error     string   `json:"error,omitempty"`
	ErrorType string   `json:"errorType,omitempty"`
}

// TransformToPrometheusLabels transforms Pinot results to Prometheus labels response
func TransformToPrometheusLabels(results []PinotResult) PrometheusLabelsResponse {
	if len(results) == 0 {
		return PrometheusLabelsResponse{
			Status: "success",
			Data:   []string{},
		}
	}

	// For now, return a hardcoded list of common labels
	// In production, this should query schema or extract from data
	labels := []string{
		"__name__",
		"job",
		"instance",
		"service_name",
		"host_name",
		"environment",
	}

	return PrometheusLabelsResponse{
		Status: "success",
		Data:   labels,
	}
}

// TransformToPrometheusLabelValues transforms Pinot results to Prometheus label values response
func TransformToPrometheusLabelValues(results []PinotResult) PrometheusLabelsResponse {
	if len(results) == 0 {
		return PrometheusLabelsResponse{
			Status: "success",
			Data:   []string{},
		}
	}

	result := results[0]
	if len(result.Rows) == 0 {
		return PrometheusLabelsResponse{
			Status: "success",
			Data:   []string{},
		}
	}

	// Extract unique values from first column
	values := make([]string, 0, len(result.Rows))
	seen := make(map[string]bool)

	for _, row := range result.Rows {
		if len(row) > 0 && row[0] != nil {
			value := fmt.Sprintf("%v", row[0])
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	}

	return PrometheusLabelsResponse{
		Status: "success",
		Data:   values,
	}
}

// Helper functions

func findColumn(columns []string, name string) int {
	for i, col := range columns {
		if col == name {
			return i
		}
	}
	return -1
}

func formatValue(v interface{}) string {
	if v == nil {
		return "NaN"
	}

	switch val := v.(type) {
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toUnixSeconds(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		// If value is too large, it's likely milliseconds
		if val > 10000000000 {
			return val / 1000
		}
		return val
	case int:
		if val > 10000000000 {
			return int64(val) / 1000
		}
		return int64(val)
	case float64:
		if val > 10000000000 {
			return int64(val) / 1000
		}
		return int64(val)
	case string:
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			if i > 10000000000 {
				return i / 1000
			}
			return i
		}
		return time.Now().Unix()
	default:
		return time.Now().Unix()
	}
}

func metricMapToKey(metric map[string]string) string {
	// Simple key generation - concatenate sorted label key-value pairs
	// This is not perfect but sufficient for grouping
	key := ""
	for k, v := range metric {
		key += k + "=" + v + ","
	}
	return key
}
