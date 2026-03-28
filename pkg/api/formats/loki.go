package formats

import (
	"fmt"
	"strconv"
)

// LokiResponse represents a Loki API response
type LokiResponse struct {
	Status    string     `json:"status"`
	Data      *LokiData  `json:"data,omitempty"`
	Error     string     `json:"error,omitempty"`
	ErrorType string     `json:"errorType,omitempty"`
}

// LokiData represents the data field in a Loki response
type LokiData struct {
	ResultType string       `json:"resultType"` // "streams" or "matrix"
	Result     []LokiResult `json:"result"`
	Stats      *LokiStats   `json:"stats,omitempty"`
}

// LokiResult represents a single result (stream or metric)
type LokiResult struct {
	Stream map[string]string `json:"stream,omitempty"` // For log streams
	Metric map[string]string `json:"metric,omitempty"` // For metrics
	Values [][]interface{}   `json:"values"`           // [[timestamp, value], ...]
}

// LokiStats represents query statistics
type LokiStats struct {
	Summary *LokiSummary `json:"summary,omitempty"`
}

// LokiSummary represents query summary statistics
type LokiSummary struct {
	BytesProcessedPerSecond int64 `json:"bytesProcessedPerSecond"`
	LinesProcessedPerSecond int64 `json:"linesProcessedPerSecond"`
	TotalBytesProcessed     int64 `json:"totalBytesProcessed"`
	TotalLinesProcessed     int64 `json:"totalLinesProcessed"`
	ExecTime                float64 `json:"execTime"`
}

// TransformToLokiStreams transforms Pinot results to Loki log stream format
func TransformToLokiStreams(results []PinotResult, limit int, direction string) LokiResponse {
	if len(results) == 0 {
		return LokiResponse{
			Status: "success",
			Data: &LokiData{
				ResultType: "streams",
				Result:     []LokiResult{},
			},
		}
	}

	// Use the first result
	result := results[0]

	// If query returned no rows, return empty result (not an error)
	if len(result.Rows) == 0 {
		return LokiResponse{
			Status: "success",
			Data: &LokiData{
				ResultType: "streams",
				Result:     []LokiResult{},
			},
		}
	}

	// Find column indices
	bodyIdx := findColumn(result.Columns, "body")
	timestampIdx := findColumn(result.Columns, "timestamp")

	if bodyIdx == -1 {
		return LokiResponse{
			Status:    "error",
			ErrorType: "bad_data",
			Error:     "missing 'body' column in query results",
		}
	}

	if timestampIdx == -1 {
		return LokiResponse{
			Status:    "error",
			ErrorType: "bad_data",
			Error:     "missing 'timestamp' column in query results",
		}
	}

	// Extract label columns (everything except body, timestamp, tenant_id)
	labelColumns := make([]string, 0)
	for _, col := range result.Columns {
		if col != "body" && col != "timestamp" && col != "tenant_id" {
			labelColumns = append(labelColumns, col)
		}
	}

	// Group rows by stream labels
	streamsMap := make(map[string]*LokiResult)

	for _, row := range result.Rows {
		// Build stream labels
		stream := make(map[string]string)

		// Add label values
		for _, labelCol := range labelColumns {
			idx := findColumn(result.Columns, labelCol)
			if idx != -1 && row[idx] != nil {
				stream[labelCol] = fmt.Sprintf("%v", row[idx])
			}
		}

		// Create stream key
		streamKey := metricMapToKey(stream)

		// Get or create stream
		s, exists := streamsMap[streamKey]
		if !exists {
			s = &LokiResult{
				Stream: stream,
				Values: make([][]interface{}, 0),
			}
			streamsMap[streamKey] = s
		}

		// Add log entry [timestamp_ns, body]
		timestampNs := toNanoseconds(row[timestampIdx])
		body := fmt.Sprintf("%v", row[bodyIdx])

		s.Values = append(s.Values, []interface{}{
			strconv.FormatInt(timestampNs, 10),
			body,
		})
	}

	// Convert map to slice
	lokiResults := make([]LokiResult, 0, len(streamsMap))
	for _, stream := range streamsMap {
		// Apply limit per stream
		if limit > 0 && len(stream.Values) > limit {
			if direction == "backward" {
				stream.Values = stream.Values[:limit]
			} else {
				stream.Values = stream.Values[len(stream.Values)-limit:]
			}
		}
		lokiResults = append(lokiResults, *stream)
	}

	return LokiResponse{
		Status: "success",
		Data: &LokiData{
			ResultType: "streams",
			Result:     lokiResults,
		},
	}
}

// TransformToLokiMatrix transforms Pinot results to Loki metric format
func TransformToLokiMatrix(results []PinotResult) LokiResponse {
	if len(results) == 0 {
		return LokiResponse{
			Status: "success",
			Data: &LokiData{
				ResultType: "matrix",
				Result:     []LokiResult{},
			},
		}
	}

	// Use the first result
	result := results[0]

	// If query returned no rows, return empty result (not an error)
	if len(result.Rows) == 0 {
		return LokiResponse{
			Status: "success",
			Data: &LokiData{
				ResultType: "matrix",
				Result:     []LokiResult{},
			},
		}
	}

	// Find column indices
	valueIdx := findColumn(result.Columns, "value")
	timestampIdx := findColumn(result.Columns, "timestamp")

	if valueIdx == -1 {
		// Check if this is a count query
		countIdx := findColumn(result.Columns, "cnt")
		if countIdx != -1 {
			valueIdx = countIdx
		} else {
			return LokiResponse{
				Status:    "error",
				ErrorType: "bad_data",
				Error:     "missing 'value' or 'cnt' column in query results",
			}
		}
	}

	if timestampIdx == -1 {
		return LokiResponse{
			Status:    "error",
			ErrorType: "bad_data",
			Error:     "missing 'timestamp' column in query results",
		}
	}

	// Extract label columns
	labelColumns := make([]string, 0)
	for _, col := range result.Columns {
		if col != "value" && col != "cnt" && col != "timestamp" && col != "tenant_id" {
			labelColumns = append(labelColumns, col)
		}
	}

	// Group rows by metric labels
	metricsMap := make(map[string]*LokiResult)

	for _, row := range result.Rows {
		// Build metric labels
		metric := make(map[string]string)

		// Add label values
		for _, labelCol := range labelColumns {
			idx := findColumn(result.Columns, labelCol)
			if idx != -1 && row[idx] != nil {
				metric[labelCol] = fmt.Sprintf("%v", row[idx])
			}
		}

		// Create metric key
		metricKey := metricMapToKey(metric)

		// Get or create metric
		m, exists := metricsMap[metricKey]
		if !exists {
			m = &LokiResult{
				Metric: metric,
				Values: make([][]interface{}, 0),
			}
			metricsMap[metricKey] = m
		}

		// Add data point [timestamp_seconds, value]
		timestamp := toUnixSeconds(row[timestampIdx])
		value := formatValue(row[valueIdx])

		m.Values = append(m.Values, []interface{}{timestamp, value})
	}

	// Convert map to slice
	lokiResults := make([]LokiResult, 0, len(metricsMap))
	for _, metric := range metricsMap {
		lokiResults = append(lokiResults, *metric)
	}

	return LokiResponse{
		Status: "success",
		Data: &LokiData{
			ResultType: "matrix",
			Result:     lokiResults,
		},
	}
}

// LokiError creates a Loki error response
func LokiError(errorType, message string) LokiResponse {
	return LokiResponse{
		Status:    "error",
		ErrorType: errorType,
		Error:     message,
	}
}

// Helper functions

func toNanoseconds(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		// If already in nanoseconds (very large number)
		if val > 1000000000000000 {
			return val
		}
		// If in milliseconds
		if val > 10000000000 {
			return val * 1000000
		}
		// If in seconds
		return val * 1000000000
	case int:
		if val > 1000000000000000 {
			return int64(val)
		}
		if val > 10000000000 {
			return int64(val) * 1000000
		}
		return int64(val) * 1000000000
	case float64:
		if val > 1000000000000000 {
			return int64(val)
		}
		if val > 10000000000 {
			return int64(val) * 1000000
		}
		return int64(val) * 1000000000
	case string:
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			if i > 1000000000000000 {
				return i
			}
			if i > 10000000000 {
				return i * 1000000
			}
			return i * 1000000000
		}
		return 0
	default:
		return 0
	}
}
