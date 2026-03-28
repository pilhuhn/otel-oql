package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// PrometheusQueryParams represents parameters for Prometheus instant query
type PrometheusQueryParams struct {
	Query   string
	Time    time.Time
	Timeout time.Duration
}

// PrometheusRangeParams represents parameters for Prometheus range query
type PrometheusRangeParams struct {
	Query   string
	Start   time.Time
	End     time.Time
	Step    time.Duration
	Timeout time.Duration
}

// LokiQueryParams represents parameters for Loki instant query
type LokiQueryParams struct {
	Query     string
	Time      time.Time
	Limit     int
	Direction string // "forward" or "backward"
}

// LokiRangeParams represents parameters for Loki range query
type LokiRangeParams struct {
	Query     string
	Start     time.Time
	End       time.Time
	Limit     int
	Step      time.Duration
	Interval  time.Duration
	Direction string
}

// ParsePrometheusQueryParams parses parameters for /api/v1/query
func ParsePrometheusQueryParams(r *http.Request) (*PrometheusQueryParams, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("failed to parse form: %w", err)
	}

	query := r.FormValue("query")
	if query == "" {
		return nil, fmt.Errorf("missing required parameter: query")
	}

	params := &PrometheusQueryParams{
		Query: query,
		Time:  time.Now(), // Default to current time
	}

	// Parse optional time parameter
	if timeStr := r.FormValue("time"); timeStr != "" {
		t, err := parseTime(timeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid time parameter: %w", err)
		}
		params.Time = t
	}

	// Parse optional timeout parameter
	if timeoutStr := r.FormValue("timeout"); timeoutStr != "" {
		timeout, err := parseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout parameter: %w", err)
		}
		params.Timeout = timeout
	}

	return params, nil
}

// ParsePrometheusRangeParams parses parameters for /api/v1/query_range
func ParsePrometheusRangeParams(r *http.Request) (*PrometheusRangeParams, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("failed to parse form: %w", err)
	}

	query := r.FormValue("query")
	if query == "" {
		return nil, fmt.Errorf("missing required parameter: query")
	}

	startStr := r.FormValue("start")
	if startStr == "" {
		return nil, fmt.Errorf("missing required parameter: start")
	}

	endStr := r.FormValue("end")
	if endStr == "" {
		return nil, fmt.Errorf("missing required parameter: end")
	}

	stepStr := r.FormValue("step")
	if stepStr == "" {
		return nil, fmt.Errorf("missing required parameter: step")
	}

	start, err := parseTime(startStr)
	if err != nil {
		return nil, fmt.Errorf("invalid start parameter: %w", err)
	}

	end, err := parseTime(endStr)
	if err != nil {
		return nil, fmt.Errorf("invalid end parameter: %w", err)
	}

	step, err := parseDuration(stepStr)
	if err != nil {
		return nil, fmt.Errorf("invalid step parameter: %w", err)
	}

	params := &PrometheusRangeParams{
		Query: query,
		Start: start,
		End:   end,
		Step:  step,
	}

	// Parse optional timeout parameter
	if timeoutStr := r.FormValue("timeout"); timeoutStr != "" {
		timeout, err := parseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout parameter: %w", err)
		}
		params.Timeout = timeout
	}

	return params, nil
}

// ParseLokiQueryParams parses parameters for /loki/api/v1/query
func ParseLokiQueryParams(r *http.Request) (*LokiQueryParams, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("failed to parse form: %w", err)
	}

	query := r.FormValue("query")
	if query == "" {
		return nil, fmt.Errorf("missing required parameter: query")
	}

	params := &LokiQueryParams{
		Query:     query,
		Time:      time.Now(), // Default to current time
		Limit:     100,        // Default limit
		Direction: "backward", // Default direction
	}

	// Parse optional time parameter
	if timeStr := r.FormValue("time"); timeStr != "" {
		t, err := parseTime(timeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid time parameter: %w", err)
		}
		params.Time = t
	}

	// Parse optional limit parameter
	if limitStr := r.FormValue("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid limit parameter: %w", err)
		}
		params.Limit = limit
	}

	// Parse optional direction parameter
	if direction := r.FormValue("direction"); direction != "" {
		if direction != "forward" && direction != "backward" {
			return nil, fmt.Errorf("invalid direction parameter: must be 'forward' or 'backward'")
		}
		params.Direction = direction
	}

	return params, nil
}

// ParseLokiRangeParams parses parameters for /loki/api/v1/query_range
func ParseLokiRangeParams(r *http.Request) (*LokiRangeParams, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("failed to parse form: %w", err)
	}

	query := r.FormValue("query")
	if query == "" {
		return nil, fmt.Errorf("missing required parameter: query")
	}

	startStr := r.FormValue("start")
	if startStr == "" {
		return nil, fmt.Errorf("missing required parameter: start")
	}

	endStr := r.FormValue("end")
	if endStr == "" {
		return nil, fmt.Errorf("missing required parameter: end")
	}

	start, err := parseTime(startStr)
	if err != nil {
		return nil, fmt.Errorf("invalid start parameter: %w", err)
	}

	end, err := parseTime(endStr)
	if err != nil {
		return nil, fmt.Errorf("invalid end parameter: %w", err)
	}

	params := &LokiRangeParams{
		Query:     query,
		Start:     start,
		End:       end,
		Limit:     5000,       // Default limit for range queries
		Direction: "backward", // Default direction
	}

	// Parse optional limit parameter
	if limitStr := r.FormValue("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid limit parameter: %w", err)
		}
		params.Limit = limit
	}

	// Parse optional step parameter (for metric queries)
	if stepStr := r.FormValue("step"); stepStr != "" {
		step, err := parseDuration(stepStr)
		if err != nil {
			return nil, fmt.Errorf("invalid step parameter: %w", err)
		}
		params.Step = step
	}

	// Parse optional interval parameter (for metric queries)
	if intervalStr := r.FormValue("interval"); intervalStr != "" {
		interval, err := parseDuration(intervalStr)
		if err != nil {
			return nil, fmt.Errorf("invalid interval parameter: %w", err)
		}
		params.Interval = interval
	}

	// Parse optional direction parameter
	if direction := r.FormValue("direction"); direction != "" {
		if direction != "forward" && direction != "backward" {
			return nil, fmt.Errorf("invalid direction parameter: must be 'forward' or 'backward'")
		}
		params.Direction = direction
	}

	return params, nil
}

// parseTime parses a timestamp in multiple formats:
// - RFC3339: "2024-03-20T10:00:00Z"
// - Unix timestamp (seconds): "1710928800"
// - Unix timestamp (milliseconds): "1710928800000"
func parseTime(s string) (time.Time, error) {
	// Try RFC3339 format first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try Unix timestamp (seconds or milliseconds)
	if timestamp, err := strconv.ParseInt(s, 10, 64); err == nil {
		// If timestamp is too large for seconds, assume milliseconds
		if timestamp > 10000000000 {
			return time.Unix(0, timestamp*int64(time.Millisecond)), nil
		}
		return time.Unix(timestamp, 0), nil
	}

	return time.Time{}, fmt.Errorf("invalid timestamp format: %s", s)
}

// parseDuration parses a duration string:
// - Prometheus format: "5m", "1h", "30s"
// - Go duration format: "5m0s", "1h0m0s"
// - Seconds as number: "300"
func parseDuration(s string) (time.Duration, error) {
	// Try Go duration format first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Try parsing as seconds (numeric)
	if seconds, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(seconds * float64(time.Second)), nil
	}

	return 0, fmt.Errorf("invalid duration format: %s", s)
}
