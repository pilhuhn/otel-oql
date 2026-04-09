package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"github.com/pilhuhn/otel-oql/pkg/traceql"

	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	resource "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// TempoEchoResponse represents the response for /api/echo
type TempoEchoResponse struct {
	Message string `json:"message"`
}

// handleTempoEcho handles GET /api/echo
// Health check endpoint used by Grafana to test datasource connectivity
func (s *Server) handleTempoEcho(w http.ResponseWriter, r *http.Request) {
	response := TempoEchoResponse{
		Message: "ok",
	}
	writeJSON(w, http.StatusOK, response)
}

// TempoTagsResponse represents the response for /api/v2/search/tags
type TempoTagsResponse struct {
	TagNames []string `json:"tagNames"`
}

// TempoTagValuesResponse represents the response for /api/v2/search/tag/{tag}/values
type TempoTagValuesResponse struct {
	TagValues []TagValue       `json:"tagValues"`
	Metrics   *TagValueMetrics `json:"metrics,omitempty"`
}

// TagValue represents a single tag value with its type
type TagValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// TagValueMetrics contains metrics about the tag values query
type TagValueMetrics struct {
	InspectedBytes string `json:"inspectedBytes,omitempty"`
}

// handleTempoSearchTags handles GET /api/v2/search/tags
// Returns list of all available TraceQL tags/fields
func (s *Server) handleTempoSearchTags(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.tempo.search_tags")
	defer span.End()

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search/tags", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Tempo search tags (tenant_id=%d)\n", tenantID)
	}

	// Return list of all available TraceQL tags
	// These include intrinsic fields, common span attributes, and resource attributes
	tags := []string{
		// Intrinsic fields
		"name",
		"duration",
		"status",
		"kind",

		// Common span attributes (OTel semantic conventions)
		"span.http.method",
		"span.http.status_code",
		"span.http.route",
		"span.http.target",
		"span.db.system",
		"span.db.statement",
		"span.messaging.system",
		"span.messaging.destination",
		"span.rpc.service",
		"span.rpc.method",
		"span.error",

		// Common resource attributes
		"resource.service.name",
	}

	response := TempoTagsResponse{
		TagNames: tags,
	}

	s.obs.RecordRequest(ctx, "/api/v2/search/tags", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, response)
}

// handleTempoSearchTagValues handles GET /api/v2/search/tag/{tag}/values
// Returns distinct values for a specific tag
func (s *Server) handleTempoSearchTagValues(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.tempo.search_tag_values")
	defer span.End()

	// Extract tag name from URL path
	// Path is /api/v2/search/tag/{tag}/values
	path := r.URL.Path
	prefix := "/api/v2/search/tag/"
	suffix := "/values"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		s.obs.RecordRequest(ctx, "/api/v2/search/tag/*/values", time.Since(start), http.StatusBadRequest)
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	tagName := path[len(prefix) : len(path)-len(suffix)]

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search/tag/*/values", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	q := query.Get("q") // TraceQL query filter (optional, e.g., "{}")

	// Parse time range
	var startTime, endTime *time.Time
	if startStr := query.Get("start"); startStr != "" {
		if ts, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			startTime = &t
		}
	}
	if endStr := query.Get("end"); endStr != "" {
		if ts, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			endTime = &t
		}
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Tempo search tag values (tenant_id=%d, tag=%s, q=%s, start=%v, end=%v)\n",
			tenantID, tagName, q, startTime, endTime)
	}

	// Map TraceQL tag name to Pinot column
	column, err := s.mapTraceQLTagToColumn(tagName)
	if err != nil {
		s.obs.RecordError(ctx, "unknown_tag", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search/tag/*/values", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("unknown tag: %s", tagName), http.StatusBadRequest)
		return
	}

	// Build SQL to get distinct values
	sql := s.buildTempoTagValuesSQL(tenantID, column, startTime, endTime)

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] Tempo tag values SQL: %s\n", sql)
	}

	// Execute query against Pinot
	results, err := s.executeQueries(ctx, []string{sql})
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search/tag/*/values", time.Since(start), http.StatusInternalServerError)
		http.Error(w, fmt.Sprintf("query execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract values from results
	tagValues := []TagValue{}
	var totalBytes int64

	if len(results) > 0 && len(results[0].Rows) > 0 {
		for _, row := range results[0].Rows {
			if len(row) > 0 {
				value, valueType := s.extractTagValue(row[0])
				if value != "" {
					tagValues = append(tagValues, TagValue{
						Type:  valueType,
						Value: value,
					})
				}
			}
		}

		// Calculate inspected bytes from query stats
		totalBytes = results[0].Stats.NumDocsScanned
	}

	response := TempoTagValuesResponse{
		TagValues: tagValues,
		Metrics: &TagValueMetrics{
			InspectedBytes: fmt.Sprintf("%d", totalBytes),
		},
	}

	s.obs.RecordRequest(ctx, "/api/v2/search/tag/*/values", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, response)
}

// mapTraceQLTagToColumn maps a TraceQL tag name to a Pinot column
func (s *Server) mapTraceQLTagToColumn(tagName string) (string, error) {
	// Intrinsic fields
	intrinsicMap := map[string]string{
		"name":     "name",
		"duration": "duration",
		"status":   "status_code",
		"kind":     "kind",
	}
	if column, ok := intrinsicMap[tagName]; ok {
		return column, nil
	}

	// Span attributes
	if strings.HasPrefix(tagName, "span.") {
		attrName := strings.TrimPrefix(tagName, "span.")
		return s.mapSpanAttributeToColumn(attrName), nil
	}

	// Resource attributes
	if strings.HasPrefix(tagName, "resource.") {
		attrName := strings.TrimPrefix(tagName, "resource.")
		return s.mapResourceAttributeToColumn(attrName), nil
	}

	return "", fmt.Errorf("unknown tag: %s", tagName)
}

// mapSpanAttributeToColumn maps a span attribute to Pinot column
func (s *Server) mapSpanAttributeToColumn(attrName string) string {
	// Map OTel semantic conventions to native columns
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

	if column, ok := nativeColumns[attrName]; ok {
		return column
	}

	// Not a native column - use JSON extraction
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, %s, 'STRING')", sqlutil.JSONObjectKeyPathLiteral(attrName))
}

// mapResourceAttributeToColumn maps a resource attribute to Pinot column
func (s *Server) mapResourceAttributeToColumn(attrName string) string {
	// Map OTel resource semantic conventions to native columns
	if attrName == "service.name" {
		return "service_name"
	}

	// Not a native column - use JSON extraction
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(resource_attributes, %s, 'STRING')", sqlutil.JSONObjectKeyPathLiteral(attrName))
}

// buildTempoTagValuesSQL builds SQL to get distinct values for a tag
func (s *Server) buildTempoTagValuesSQL(tenantID int, column string, start, end *time.Time) string {
	sql := fmt.Sprintf("SELECT DISTINCT %s FROM otel_spans WHERE tenant_id = %d AND %s IS NOT NULL",
		column, tenantID, column)

	// Add time range if provided
	if start != nil && end != nil {
		startMillis := start.UnixMilli()
		endMillis := end.UnixMilli()
		sql += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	}

	sql += " LIMIT 100" // Limit to 100 values for autocomplete

	return sql
}

// extractTagValue extracts the value and determines its type from a Pinot result
func (s *Server) extractTagValue(val interface{}) (string, string) {
	if val == nil {
		return "", "string"
	}

	switch v := val.(type) {
	case string:
		return v, "string"
	case int:
		return fmt.Sprintf("%d", v), "int"
	case int64:
		return fmt.Sprintf("%d", v), "int"
	case float64:
		// Check if it's actually an integer value
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), "int"
		}
		return fmt.Sprintf("%f", v), "float"
	case bool:
		return fmt.Sprintf("%t", v), "bool"
	default:
		return fmt.Sprintf("%v", v), "string"
	}
}

// TempoSearchResponse represents the response for /api/v2/search
type TempoSearchResponse struct {
	Traces []TempoTrace `json:"traces"`
}

// TempoTrace represents a single trace in the search results
type TempoTrace struct {
	TraceID           string                  `json:"traceID"`
	RootServiceName   string                  `json:"rootServiceName,omitempty"`
	RootTraceName     string                  `json:"rootTraceName,omitempty"`
	StartTimeUnixNano int64                   `json:"startTimeUnixNano"`
	DurationMs        int                     `json:"durationMs"`
	SpanSets          []TempoSpanSet          `json:"spanSets,omitempty"`
	ServiceStats      map[string]ServiceStats `json:"serviceStats,omitempty"`
}

// ServiceStats represents per-service statistics in a trace
type ServiceStats struct {
	SpanCount  int `json:"spanCount"`
	ErrorCount int `json:"errorCount,omitempty"`
}

// TempoSpanSet represents a set of matching spans
type TempoSpanSet struct {
	Spans   []TempoSpan `json:"spans"`
	Matched int         `json:"matched"`
}

// TempoSpan represents a span in the search results
type TempoSpan struct {
	SpanID            string            `json:"spanID"`
	Name              string            `json:"name"`
	StartTimeUnixNano int64             `json:"startTimeUnixNano"`
	DurationNanos     int64             `json:"durationNanos"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

// handleTempoSearch handles GET/POST /api/v2/search
// Executes TraceQL queries and returns matching traces/spans
func (s *Server) handleTempoSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.tempo.search")
	defer span.End()

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	q := query.Get("q") // TraceQL query

	if q == "" {
		s.obs.RecordError(ctx, "missing_query", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search", time.Since(start), http.StatusBadRequest)
		http.Error(w, "missing query parameter 'q'", http.StatusBadRequest)
		return
	}

	// Parse time range
	var startTime, endTime *time.Time
	if startStr := query.Get("start"); startStr != "" {
		if ts, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			startTime = &t
		}
	}
	if endStr := query.Get("end"); endStr != "" {
		if ts, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			endTime = &t
		}
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Tempo search (tenant_id=%d, q=%s, start=%v, end=%v)\n",
			tenantID, q, startTime, endTime)
	}

	// Translate TraceQL query to SQL
	translator := traceql.NewTranslator(tenantID)
	var sqlQueries []string
	var err error

	if startTime != nil && endTime != nil {
		sqlQueries, err = translator.TranslateQueryWithTimeRange(q, startTime, endTime)
	} else {
		sqlQueries, err = translator.TranslateQuery(q)
	}

	if err != nil {
		if s.debugQuery {
			fmt.Printf("[DEBUG QUERY] TraceQL translation failed: %v\n", err)
		}
		s.obs.RecordError(ctx, "translation_error", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("TraceQL parse error: %v", err), http.StatusBadRequest)
		return
	}

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] TraceQL query translated to %d SQL statements:\n", len(sqlQueries))
		for i, sql := range sqlQueries {
			fmt.Printf("[DEBUG TRANSLATION]   [%d] %s\n", i+1, sql)
		}
	}

	// Execute queries against Pinot
	results, err := s.executeQueries(ctx, sqlQueries)
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/v2/search", time.Since(start), http.StatusInternalServerError)
		http.Error(w, fmt.Sprintf("query execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Transform results to Tempo format
	traces := s.transformToTempoTraces(results)

	response := TempoSearchResponse{
		Traces: traces,
	}

	s.obs.RecordRequest(ctx, "/api/v2/search", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, response)
}

// transformToTempoTraces transforms Pinot results to Tempo trace format
func (s *Server) transformToTempoTraces(results []QueryResult) []TempoTrace {
	// Group spans by trace_id
	traceMap := make(map[string][]map[string]interface{})

	for _, result := range results {
		for _, row := range result.Rows {
			if len(row) < len(result.Columns) {
				continue
			}

			// Create span map
			spanData := make(map[string]interface{})
			for i, col := range result.Columns {
				spanData[col] = row[i]
			}

			// Extract trace_id
			traceID := ""
			if tid, ok := spanData["trace_id"].(string); ok {
				traceID = tid
			}

			if traceID != "" {
				traceMap[traceID] = append(traceMap[traceID], spanData)
			}
		}
	}

	// Convert to Tempo trace format
	traces := []TempoTrace{}
	for traceID, spans := range traceMap {
		if len(spans) == 0 {
			continue
		}

		// Find root span and calculate trace duration
		var rootServiceName, rootTraceName string
		var minStartTime int64 = 0
		var maxEndTime int64 = 0
		serviceStats := make(map[string]ServiceStats)

		for _, span := range spans {
			// Check if this is the root span (no parent_span_id)
			isRootSpan := false
			if parentSpanID, ok := span["parent_span_id"].(string); ok {
				// Root span has empty or all-zero parent_span_id
				if parentSpanID == "" || parentSpanID == "0000000000000000" {
					isRootSpan = true
				}
			}

			// Extract span name - use root span's name for rootTraceName
			if isRootSpan {
				if name, ok := span["name"].(string); ok {
					rootTraceName = name
				}
			}

			// Extract service name for root and stats
			serviceName := "unknown"
			if service, ok := span["service_name"].(string); ok && service != "" {
				serviceName = service
				if isRootSpan {
					rootServiceName = service
				}
			}

			// Calculate per-service stats
			stats := serviceStats[serviceName]
			stats.SpanCount++

			// Check if span has error flag
			if errorVal, ok := span["error"]; ok {
				if isError, ok := errorVal.(bool); ok && isError {
					stats.ErrorCount++
				}
			}
			serviceStats[serviceName] = stats

			// Calculate trace time bounds
			var tsMillis int64
			if tsVal := span["timestamp"]; tsVal != nil {
				switch v := tsVal.(type) {
				case int64:
					tsMillis = v
				case int:
					tsMillis = int64(v)
				case float64:
					tsMillis = int64(v)
				case string:
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
						tsMillis = parsed
					}
				}
			}

			if tsMillis > 0 {
				if minStartTime == 0 || tsMillis < minStartTime {
					minStartTime = tsMillis
				}

				// Calculate end time
				var durationNanos int64
				if durationVal := span["duration"]; durationVal != nil {
					switch v := durationVal.(type) {
					case int64:
						durationNanos = v
					case int:
						durationNanos = int64(v)
					case float64:
						durationNanos = int64(v)
					case string:
						if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
							durationNanos = parsed
						}
					}
				}

				if durationNanos > 0 {
					endTime := tsMillis + (durationNanos / 1000000) // Convert nanos to millis
					if endTime > maxEndTime {
						maxEndTime = endTime
					}
				}
			}
		}

		durationMs := 0
		if maxEndTime > minStartTime {
			durationMs = int(maxEndTime - minStartTime)
		}

		traces = append(traces, TempoTrace{
			TraceID:           traceID,
			RootServiceName:   rootServiceName,
			RootTraceName:     rootTraceName,
			StartTimeUnixNano: minStartTime * 1000000, // Convert millis to nanos
			DurationMs:        durationMs,
			ServiceStats:      serviceStats,
		})
	}

	return traces
}

// TempoV1SearchResponse represents the response for /api/search (v1 API)
type TempoV1SearchResponse struct {
	Traces []TempoV1Trace `json:"traces"`
}

// TempoV1Trace represents a trace in v1 search results
type TempoV1Trace struct {
	TraceID           string                  `json:"traceID"`
	RootServiceName   string                  `json:"rootServiceName"`
	RootTraceName     string                  `json:"rootTraceName"`
	StartTimeUnixNano string                  `json:"startTimeUnixNano"` // v1 uses string
	DurationMs        int                     `json:"durationMs"`
	ServiceStats      map[string]ServiceStats `json:"serviceStats,omitempty"`
}

// TempoMetadata contains metadata about available values
type TempoMetadata struct {
	ServiceNames   []string `json:"serviceNames,omitempty"`
	OperationNames []string `json:"operationNames,omitempty"`
}

// handleTempoV1Search handles GET /api/search (Tempo v1 API)
// Used by Grafana to get trace metadata and search results
func (s *Server) handleTempoV1Search(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.tempo.v1.search")
	defer span.End()

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/search", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	q := query.Get("q") // TraceQL query (often just "{}")
	limitStr := query.Get("limit")

	limit := 20 // Default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Parse time range
	var startTime, endTime *time.Time
	if startStr := query.Get("start"); startStr != "" {
		if ts, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			startTime = &t
		}
	}
	if endStr := query.Get("end"); endStr != "" {
		if ts, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			t := time.Unix(ts, 0)
			endTime = &t
		}
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Tempo v1 search (tenant_id=%d, q=%s, limit=%d, start=%v, end=%v)\n",
			tenantID, q, limit, startTime, endTime)
	}

	// If query is empty or just "{}", return empty traces
	// (Grafana gets metadata from /api/v2/search/tags endpoints instead)
	if q == "" || q == "{}" {
		response := TempoV1SearchResponse{
			Traces: []TempoV1Trace{}, // Empty traces list
		}

		s.obs.RecordRequest(ctx, "/api/search", time.Since(start), http.StatusOK)
		writeJSON(w, http.StatusOK, response)
		return
	}

	// Execute TraceQL query
	translator := traceql.NewTranslator(tenantID)
	var sqlQueries []string
	var err error

	if startTime != nil && endTime != nil {
		sqlQueries, err = translator.TranslateQueryWithTimeRange(q, startTime, endTime)
	} else {
		sqlQueries, err = translator.TranslateQuery(q)
	}

	if err != nil {
		if s.debugQuery {
			fmt.Printf("[DEBUG QUERY] TraceQL translation failed: %v\n", err)
		}
		s.obs.RecordError(ctx, "translation_error", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/search", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("TraceQL parse error: %v", err), http.StatusBadRequest)
		return
	}

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] Tempo v1 search query translated to %d SQL statements:\n", len(sqlQueries))
		for i, sql := range sqlQueries {
			fmt.Printf("[DEBUG TRANSLATION]   [%d] %s\n", i+1, sql)
		}
	}

	// Add LIMIT to SQL
	if len(sqlQueries) > 0 {
		sqlQueries[0] = sqlQueries[0] + fmt.Sprintf(" LIMIT %d", limit)
	}

	// Execute queries against Pinot
	results, err := s.executeQueries(ctx, sqlQueries)
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/search", time.Since(start), http.StatusInternalServerError)
		http.Error(w, fmt.Sprintf("query execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Transform results to v1 format
	traces := s.transformToTempoV1Traces(results)

	response := TempoV1SearchResponse{
		Traces: traces,
	}

	s.obs.RecordRequest(ctx, "/api/search", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, response)
}

// getTempoMetadata returns metadata about available service names and operation names
func (s *Server) getTempoMetadata(ctx context.Context, tenantID int, start, end *time.Time) (*TempoMetadata, error) {
	// Build SQL to get distinct service names
	serviceNamesSQL := fmt.Sprintf("SELECT DISTINCT service_name FROM otel_spans WHERE tenant_id = %d AND service_name IS NOT NULL", tenantID)

	if start != nil && end != nil {
		startMillis := start.UnixMilli()
		endMillis := end.UnixMilli()
		serviceNamesSQL += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	}

	serviceNamesSQL += " LIMIT 100"

	// Build SQL to get distinct operation names (span names)
	operationNamesSQL := fmt.Sprintf("SELECT DISTINCT name FROM otel_spans WHERE tenant_id = %d AND name IS NOT NULL", tenantID)

	if start != nil && end != nil {
		startMillis := start.UnixMilli()
		endMillis := end.UnixMilli()
		operationNamesSQL += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
	}

	operationNamesSQL += " LIMIT 100"

	// Execute both queries
	results, err := s.executeQueries(ctx, []string{serviceNamesSQL, operationNamesSQL})
	if err != nil {
		return nil, err
	}

	metadata := &TempoMetadata{
		ServiceNames:   []string{},
		OperationNames: []string{},
	}

	// Extract service names from first result
	if len(results) > 0 && len(results[0].Rows) > 0 {
		for _, row := range results[0].Rows {
			if len(row) > 0 {
				if val, ok := row[0].(string); ok && val != "" {
					metadata.ServiceNames = append(metadata.ServiceNames, val)
				}
			}
		}
	}

	// Extract operation names from second result
	if len(results) > 1 && len(results[1].Rows) > 0 {
		for _, row := range results[1].Rows {
			if len(row) > 0 {
				if val, ok := row[0].(string); ok && val != "" {
					metadata.OperationNames = append(metadata.OperationNames, val)
				}
			}
		}
	}

	return metadata, nil
}

// transformToTempoV1Traces transforms Pinot results to Tempo v1 trace format
func (s *Server) transformToTempoV1Traces(results []QueryResult) []TempoV1Trace {
	// Group spans by trace_id (reuse logic from v2)
	traceMap := make(map[string][]map[string]interface{})

	for _, result := range results {
		for _, row := range result.Rows {
			if len(row) < len(result.Columns) {
				continue
			}

			// Create span map
			spanData := make(map[string]interface{})
			for i, col := range result.Columns {
				spanData[col] = row[i]
			}

			// Extract trace_id
			traceID := ""
			if tid, ok := spanData["trace_id"].(string); ok {
				traceID = tid
			}

			if traceID != "" {
				traceMap[traceID] = append(traceMap[traceID], spanData)
			}
		}
	}

	// Convert to Tempo v1 trace format
	traces := []TempoV1Trace{}
	for traceID, spans := range traceMap {
		if len(spans) == 0 {
			continue
		}

		// Find root span and calculate trace duration
		var rootServiceName, rootTraceName string
		var minStartTime int64 = 0
		var maxEndTime int64 = 0
		serviceStats := make(map[string]ServiceStats)

		for _, span := range spans {
			// Check if this is the root span (no parent_span_id)
			isRootSpan := false
			if parentSpanID, ok := span["parent_span_id"].(string); ok {
				// Root span has empty or all-zero parent_span_id
				if parentSpanID == "" || parentSpanID == "0000000000000000" {
					isRootSpan = true
				}
			}

			// Extract span name - use root span's name for rootTraceName
			if isRootSpan {
				if name, ok := span["name"].(string); ok {
					rootTraceName = name
				}
			}

			// Extract service name for root and stats
			serviceName := "unknown"
			if service, ok := span["service_name"].(string); ok && service != "" {
				serviceName = service
				if isRootSpan {
					rootServiceName = service
				}
			}

			// Calculate per-service stats
			stats := serviceStats[serviceName]
			stats.SpanCount++

			// Check if span has error flag
			if errorVal, ok := span["error"]; ok {
				if isError, ok := errorVal.(bool); ok && isError {
					stats.ErrorCount++
				}
			}
			serviceStats[serviceName] = stats

			// Calculate trace time bounds - handle multiple types from Pinot
			var tsMillis int64
			if tsVal := span["timestamp"]; tsVal != nil {
				switch v := tsVal.(type) {
				case int64:
					tsMillis = v
				case int:
					tsMillis = int64(v)
				case float64:
					tsMillis = int64(v)
				case string:
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
						tsMillis = parsed
					}
				}
			}

			if tsMillis > 0 {
				if minStartTime == 0 || tsMillis < minStartTime {
					minStartTime = tsMillis
				}

				// Calculate end time
				var durationNanos int64
				if durationVal := span["duration"]; durationVal != nil {
					switch v := durationVal.(type) {
					case int64:
						durationNanos = v
					case int:
						durationNanos = int64(v)
					case float64:
						durationNanos = int64(v)
					case string:
						if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
							durationNanos = parsed
						}
					}
				}

				if durationNanos > 0 {
					endTime := tsMillis + (durationNanos / 1000000) // Convert nanos to millis
					if endTime > maxEndTime {
						maxEndTime = endTime
					}
				}
			}
		}

		durationMs := 0
		if maxEndTime > minStartTime {
			durationMs = int(maxEndTime - minStartTime)
		}

		// v1 uses string for startTimeUnixNano
		startTimeNano := minStartTime * 1000000 // Convert millis to nanos

		traces = append(traces, TempoV1Trace{
			TraceID:           traceID,
			RootServiceName:   rootServiceName,
			RootTraceName:     rootTraceName,
			StartTimeUnixNano: fmt.Sprintf("%d", startTimeNano),
			DurationMs:        durationMs,
			ServiceStats:      serviceStats,
		})
	}

	return traces
}

// TempoTraceResponse represents the response for /api/traces/{traceID}
// Uses OTLP structure with resourceSpans and scopeSpans
type TempoTraceResponse struct {
	ResourceSpans []TempoResourceSpans `json:"resourceSpans"`
}

// TempoTraceResponseV1 represents the v1 response format with batches
type TempoTraceResponseV1 struct {
	Batches []TempoBatch `json:"batches"`
}

// TempoBatch represents a batch in the v1 format (same structure as resourceSpans)
type TempoBatch struct {
	Resource   TempoResource     `json:"resource"`
	ScopeSpans []TempoScopeSpans `json:"scopeSpans"`
}

// TempoResourceSpans represents spans from a single resource
type TempoResourceSpans struct {
	Resource   TempoResource     `json:"resource"`
	ScopeSpans []TempoScopeSpans `json:"scopeSpans"`
}

// TempoScopeSpans represents spans from a single instrumentation scope
type TempoScopeSpans struct {
	Scope TempoScope       `json:"scope,omitempty"`
	Spans []TempoTraceSpan `json:"spans"`
}

// TempoScope represents instrumentation scope (library) info
type TempoScope struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// TempoResource represents resource attributes
type TempoResource struct {
	Attributes []TempoAttribute `json:"attributes,omitempty"`
}

// TempoTraceSpan represents a full span with all details
type TempoTraceSpan struct {
	TraceID           string           `json:"traceId"`
	SpanID            string           `json:"spanId"`
	ParentSpanID      string           `json:"parentSpanId,omitempty"`
	Name              string           `json:"name"`
	Kind              int              `json:"kind,omitempty"` // OTLP SpanKind enum (0-5)
	StartTimeUnixNano string           `json:"startTimeUnixNano"`
	EndTimeUnixNano   string           `json:"endTimeUnixNano"`         // Required by OTLP
	DurationNanos     string           `json:"durationNanos,omitempty"` // Optional, for convenience
	Attributes        []TempoAttribute `json:"attributes,omitempty"`
	Status            TempoStatus      `json:"status,omitempty"`
}

// TempoAttribute represents a key-value attribute in OTLP format
type TempoAttribute struct {
	Key   string        `json:"key"`
	Value TempoAnyValue `json:"value"`
}

// TempoAnyValue represents an OTLP AnyValue (type-wrapped value)
type TempoAnyValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *int64   `json:"intValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

// Helper functions to create TempoAnyValue
func stringValue(s string) TempoAnyValue {
	return TempoAnyValue{StringValue: &s}
}

func intValue(i int64) TempoAnyValue {
	return TempoAnyValue{IntValue: &i}
}

func intValueFromInt(i int) TempoAnyValue {
	val := int64(i)
	return TempoAnyValue{IntValue: &val}
}

func boolValue(b bool) TempoAnyValue {
	return TempoAnyValue{BoolValue: &b}
}

// TempoStatus represents span status
type TempoStatus struct {
	Code    int    `json:"code,omitempty"` // OTLP StatusCode enum (0-2)
	Message string `json:"message,omitempty"`
}

// handleTempoTraceByID handles GET /api/traces/{traceID}
// Returns full trace details with all spans
func (s *Server) handleTempoTraceByID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.tempo.trace_by_id")
	defer span.End()

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Extract trace ID from URL path
	// Path can be /api/traces/{traceID} (v1) or /api/v2/traces/{traceID} (v2)
	// Both now return the same OTLP format with resourceSpans
	path := r.URL.Path
	var traceID string

	if strings.HasPrefix(path, "/api/v2/traces/") {
		traceID = path[len("/api/v2/traces/"):]
	} else if strings.HasPrefix(path, "/api/traces/") {
		traceID = path[len("/api/traces/"):]
	} else {
		s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusBadRequest)
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	if traceID == "" {
		s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusBadRequest)
		http.Error(w, "missing trace ID", http.StatusBadRequest)
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Tempo get trace by ID (tenant_id=%d, traceID=%s)\n", tenantID, traceID)
	}

	// Build SQL to get all spans for this trace
	sql := fmt.Sprintf(
		"SELECT * FROM otel_spans WHERE tenant_id = %d AND trace_id = %s ORDER BY \"timestamp\" ASC",
		tenantID,
		sqlutil.StringLiteral(traceID),
	)

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] Trace by ID SQL: %s\n", sql)
	}

	// Execute query against Pinot
	results, err := s.executeQueries(ctx, []string{sql})
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "tempo_api")
		s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusInternalServerError)
		http.Error(w, fmt.Sprintf("query execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Check if trace was found
	if len(results) == 0 || len(results[0].Rows) == 0 {
		s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusNotFound)
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}

	// Check Accept header to determine response format
	acceptHeader := r.Header.Get("Accept")
	wantsProtobuf := strings.Contains(acceptHeader, "application/protobuf")

	if wantsProtobuf {
		// Grafana requests protobuf - return OTLP ExportTraceServiceRequest format
		traceRequest := s.transformToOTLPProtobuf(results[0])

		if s.debugQuery {
			fmt.Printf("[DEBUG] Protobuf ExportTraceServiceRequest: %d resourceSpans\n", len(traceRequest.ResourceSpans))
			if len(traceRequest.ResourceSpans) > 0 {
				fmt.Printf("[DEBUG]   ResourceSpans[0]: %d scopeSpans\n", len(traceRequest.ResourceSpans[0].ScopeSpans))
				if len(traceRequest.ResourceSpans[0].ScopeSpans) > 0 {
					fmt.Printf("[DEBUG]     ScopeSpans[0]: %d spans\n", len(traceRequest.ResourceSpans[0].ScopeSpans[0].Spans))
					if len(traceRequest.ResourceSpans[0].ScopeSpans[0].Spans) > 0 {
						span := traceRequest.ResourceSpans[0].ScopeSpans[0].Spans[0]
						fmt.Printf("[DEBUG]       Span[0]: traceId len=%d, spanId len=%d, name=%s, kind=%d, startTime=%d, endTime=%d, status=%d\n",
							len(span.TraceId), len(span.SpanId), span.Name, span.Kind, span.StartTimeUnixNano, span.EndTimeUnixNano, span.Status.Code)
					}
				}
			}
		}

		data, err := proto.Marshal(traceRequest)
		if err != nil {
			s.obs.RecordError(ctx, "protobuf_marshal_error", "tempo_api")
			s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusInternalServerError)
			http.Error(w, fmt.Sprintf("failed to marshal protobuf: %v", err), http.StatusInternalServerError)
			return
		}

		if s.debugQuery {
			fmt.Printf("[DEBUG] Marshaled protobuf: %d bytes\n", len(data))
		}

		w.Header().Set("Content-Type", "application/protobuf")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		written, err := w.Write(data)
		if err != nil {
			fmt.Printf("[ERROR] Failed to write protobuf response: %v\n", err)
		}
		if s.debugQuery {
			fmt.Printf("[DEBUG] Wrote %d bytes of protobuf data\n", written)
		}
		s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusOK)
	} else {
		// JSON response for other clients (Perses, curl, etc.)
		// Use Tempo v1 format with batches
		trace := s.transformToTempoTraceV1(results[0])
		s.obs.RecordRequest(ctx, "/api/traces/*", time.Since(start), http.StatusOK)
		writeJSON(w, http.StatusOK, trace)
	}
}

// transformToTempoTraceV1 transforms Pinot query results to Tempo v1 trace format (batches)
func (s *Server) transformToTempoTraceV1(result QueryResult) TempoTraceResponseV1 {
	// Group spans by service_name
	// Each service gets its own batch/resource in OTLP
	serviceSpans := make(map[string][]TempoTraceSpan)

	// Convert each row to a span
	for _, row := range result.Rows {
		if len(row) < len(result.Columns) {
			continue
		}

		// Create map of column name to value
		spanData := make(map[string]interface{})
		for i, col := range result.Columns {
			spanData[col] = row[i]
		}

		// Extract service name
		serviceName := "unknown"
		if svc, ok := spanData["service_name"].(string); ok && svc != "" {
			serviceName = svc
		}

		// Build span
		span := s.buildTempoTraceSpan(spanData)

		// Group by service
		serviceSpans[serviceName] = append(serviceSpans[serviceName], span)
	}

	// Build batches - one per service
	batches := make([]TempoBatch, 0, len(serviceSpans))
	for serviceName, spans := range serviceSpans {
		// Create resource for this service
		resource := TempoResource{
			Attributes: []TempoAttribute{
				{
					Key:   "service.name",
					Value: stringValue(serviceName),
				},
			},
		}

		// Create scope spans
		scopeSpans := TempoScopeSpans{
			Scope: TempoScope{},
			Spans: spans,
		}

		// Build batch
		batch := TempoBatch{
			Resource:   resource,
			ScopeSpans: []TempoScopeSpans{scopeSpans},
		}

		batches = append(batches, batch)
	}

	return TempoTraceResponseV1{
		Batches: batches,
	}
}

// transformToTempoTraceV2 transforms Pinot query results to OTLP v2 trace format (resourceSpans)
func (s *Server) transformToTempoTraceV2(result QueryResult) TempoTraceResponse {
	// Group spans by service_name
	// Each service gets its own resourceSpan in OTLP
	serviceSpans := make(map[string][]TempoTraceSpan)

	// Convert each row to a span
	for _, row := range result.Rows {
		if len(row) < len(result.Columns) {
			continue
		}

		// Create map of column name to value
		spanData := make(map[string]interface{})
		for i, col := range result.Columns {
			spanData[col] = row[i]
		}

		// Extract service name
		serviceName := "unknown"
		if svc, ok := spanData["service_name"].(string); ok && svc != "" {
			serviceName = svc
		}

		// Build span
		span := s.buildTempoTraceSpan(spanData)

		// Group by service
		serviceSpans[serviceName] = append(serviceSpans[serviceName], span)
	}

	// Build resourceSpans - one per service
	resourceSpans := make([]TempoResourceSpans, 0, len(serviceSpans))
	for serviceName, spans := range serviceSpans {
		// Create resource for this service
		resource := TempoResource{
			Attributes: []TempoAttribute{
				{
					Key:   "service.name",
					Value: stringValue(serviceName),
				},
			},
		}

		// Create scope spans
		scopeSpans := TempoScopeSpans{
			Scope: TempoScope{},
			Spans: spans,
		}

		// Build resourceSpan
		resourceSpan := TempoResourceSpans{
			Resource:   resource,
			ScopeSpans: []TempoScopeSpans{scopeSpans},
		}

		resourceSpans = append(resourceSpans, resourceSpan)
	}

	return TempoTraceResponse{
		ResourceSpans: resourceSpans,
	}
}

// transformToOTLPProtobuf converts Pinot query results to OTLP protobuf format
// This is used when Grafana requests protobuf (Accept: application/protobuf)
func (s *Server) transformToOTLPProtobuf(result QueryResult) *collectortrace.ExportTraceServiceRequest {
	// Group spans by service_name
	// Each service gets its own ResourceSpans in OTLP
	serviceSpans := make(map[string][]*tracepb.Span)

	// Convert each row to a span
	for _, row := range result.Rows {
		if len(row) < len(result.Columns) {
			continue
		}

		// Create map of column name to value
		spanData := make(map[string]interface{})
		for i, col := range result.Columns {
			spanData[col] = row[i]
		}

		// Extract service name
		serviceName := "unknown"
		if svc, ok := spanData["service_name"].(string); ok && svc != "" {
			serviceName = svc
		}

		// Build OTLP span
		span := s.buildOTLPSpan(spanData)
		if span != nil {
			serviceSpans[serviceName] = append(serviceSpans[serviceName], span)
		}
	}

	// Build ResourceSpans - one per service
	resourceSpansList := make([]*tracepb.ResourceSpans, 0, len(serviceSpans))
	for serviceName, spans := range serviceSpans {
		resourceSpan := &tracepb.ResourceSpans{
			Resource: &resource.Resource{
				Attributes: []*common.KeyValue{
					{
						Key: "service.name",
						Value: &common.AnyValue{
							Value: &common.AnyValue_StringValue{
								StringValue: serviceName,
							},
						},
					},
				},
			},
			ScopeSpans: []*tracepb.ScopeSpans{
				{
					Scope: &common.InstrumentationScope{},
					Spans: spans,
				},
			},
		}
		resourceSpansList = append(resourceSpansList, resourceSpan)
	}

	return &collectortrace.ExportTraceServiceRequest{
		ResourceSpans: resourceSpansList,
	}
}

// buildOTLPSpan creates an OTLP protobuf Span from Pinot span data
func (s *Server) buildOTLPSpan(spanData map[string]interface{}) *tracepb.Span {
	span := &tracepb.Span{
		Attributes: []*common.KeyValue{},
		Status:     &tracepb.Status{},
	}

	// Extract trace_id (hex string -> bytes, must be exactly 16 bytes)
	if traceID, ok := spanData["trace_id"].(string); ok && traceID != "" {
		traceIDBytes, err := hex.DecodeString(traceID)
		if err == nil && len(traceIDBytes) == 16 {
			span.TraceId = traceIDBytes
		} else if s.debugQuery {
			fmt.Printf("[DEBUG] Invalid trace_id: %s (decoded len=%d, error=%v)\n", traceID, len(traceIDBytes), err)
		}
	}

	// Extract span_id (hex string -> bytes, must be exactly 8 bytes)
	if spanID, ok := spanData["span_id"].(string); ok && spanID != "" {
		spanIDBytes, err := hex.DecodeString(spanID)
		if err == nil && len(spanIDBytes) == 8 {
			span.SpanId = spanIDBytes
		} else if s.debugQuery {
			fmt.Printf("[DEBUG] Invalid span_id: %s (decoded len=%d, error=%v)\n", spanID, len(spanIDBytes), err)
		}
	}

	// Extract parent_span_id (hex string -> bytes, must be exactly 8 bytes)
	if parentSpanID, ok := spanData["parent_span_id"].(string); ok && parentSpanID != "" {
		parentSpanIDBytes, err := hex.DecodeString(parentSpanID)
		if err == nil && len(parentSpanIDBytes) == 8 {
			span.ParentSpanId = parentSpanIDBytes
		} else if s.debugQuery && parentSpanID != "" {
			fmt.Printf("[DEBUG] Invalid parent_span_id: %s (decoded len=%d, error=%v)\n", parentSpanID, len(parentSpanIDBytes), err)
		}
	}

	// Extract name
	if name, ok := spanData["name"].(string); ok {
		span.Name = name
	}

	// Extract kind (Pinot stores as "Server", "Client", etc.)
	if kind, ok := spanData["kind"].(string); ok {
		switch kind {
		case "Internal", "SPAN_KIND_INTERNAL":
			span.Kind = tracepb.Span_SPAN_KIND_INTERNAL
		case "Server", "SPAN_KIND_SERVER":
			span.Kind = tracepb.Span_SPAN_KIND_SERVER
		case "Client", "SPAN_KIND_CLIENT":
			span.Kind = tracepb.Span_SPAN_KIND_CLIENT
		case "Producer", "SPAN_KIND_PRODUCER":
			span.Kind = tracepb.Span_SPAN_KIND_PRODUCER
		case "Consumer", "SPAN_KIND_CONSUMER":
			span.Kind = tracepb.Span_SPAN_KIND_CONSUMER
		default:
			span.Kind = tracepb.Span_SPAN_KIND_UNSPECIFIED
		}
	}

	// Extract timestamp (milliseconds -> nanoseconds uint64)
	var startTimeNanos uint64
	if tsVal := spanData["timestamp"]; tsVal != nil {
		var tsMillis int64
		switch v := tsVal.(type) {
		case int64:
			tsMillis = v
		case int:
			tsMillis = int64(v)
		case float64:
			tsMillis = int64(v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				tsMillis = parsed
			}
		}
		if tsMillis > 0 {
			startTimeNanos = uint64(tsMillis * 1000000)
			span.StartTimeUnixNano = startTimeNanos
		}
	}

	// Extract duration and calculate end time
	if durationVal := spanData["duration"]; durationVal != nil {
		var durationNanos int64
		switch v := durationVal.(type) {
		case int64:
			durationNanos = v
		case int:
			durationNanos = int64(v)
		case float64:
			durationNanos = int64(v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				durationNanos = parsed
			}
		}
		if durationNanos > 0 && startTimeNanos > 0 {
			span.EndTimeUnixNano = startTimeNanos + uint64(durationNanos)
		}
	}

	// Extract status (Pinot stores as "Ok", "Error", "Unset")
	if statusCode, ok := spanData["status_code"].(string); ok {
		switch statusCode {
		case "Ok", "STATUS_CODE_OK":
			span.Status.Code = tracepb.Status_STATUS_CODE_OK
		case "Error", "STATUS_CODE_ERROR":
			span.Status.Code = tracepb.Status_STATUS_CODE_ERROR
		default:
			span.Status.Code = tracepb.Status_STATUS_CODE_UNSET
		}
	}

	// Extract native column attributes
	nativeAttributes := map[string]string{
		"http_method":           "http.method",
		"http_status_code":      "http.status_code",
		"db_system":             "db.system",
		"db_statement":          "db.statement",
		"messaging_system":      "messaging.system",
		"messaging_destination": "messaging.destination",
		"rpc_service":           "rpc.service",
		"rpc_method":            "rpc.method",
	}

	for column, attrKey := range nativeAttributes {
		if val := spanData[column]; val != nil {
			var anyValue *common.AnyValue
			switch v := val.(type) {
			case string:
				if v != "" {
					anyValue = &common.AnyValue{
						Value: &common.AnyValue_StringValue{StringValue: v},
					}
				}
			case int64:
				anyValue = &common.AnyValue{
					Value: &common.AnyValue_IntValue{IntValue: v},
				}
			case int:
				anyValue = &common.AnyValue{
					Value: &common.AnyValue_IntValue{IntValue: int64(v)},
				}
			case float64:
				anyValue = &common.AnyValue{
					Value: &common.AnyValue_DoubleValue{DoubleValue: v},
				}
			}
			if anyValue != nil {
				span.Attributes = append(span.Attributes, &common.KeyValue{
					Key:   attrKey,
					Value: anyValue,
				})
			}
		}
	}

	// TODO: Extract custom attributes from JSON attributes column if needed

	return span
}

// spanKindToInt converts OTLP string span kind to integer enum
// Per https://github.com/open-telemetry/opentelemetry-proto/blob/v1.5.0/opentelemetry/proto/trace/v1/trace.proto#L152
func spanKindToInt(kind string) int {
	switch strings.ToUpper(kind) {
	case "INTERNAL":
		return 1
	case "SERVER":
		return 2
	case "CLIENT":
		return 3
	case "PRODUCER":
		return 4
	case "CONSUMER":
		return 5
	default:
		return 0 // UNSPECIFIED
	}
}

// statusCodeToInt converts OTLP string status code to integer enum
// Per https://github.com/open-telemetry/opentelemetry-proto/blob/v1.5.0/opentelemetry/proto/trace/v1/trace.proto#L303
func statusCodeToInt(code string) int {
	switch strings.ToUpper(code) {
	case "OK":
		return 1
	case "ERROR":
		return 2
	default:
		return 0 // UNSET
	}
}

// buildTempoTraceSpan builds a Tempo span from Pinot row data
func (s *Server) buildTempoTraceSpan(spanData map[string]interface{}) TempoTraceSpan {
	span := TempoTraceSpan{
		Attributes: []TempoAttribute{},
	}

	// Extract core fields
	if val, ok := spanData["trace_id"].(string); ok {
		span.TraceID = val
	}
	if val, ok := spanData["span_id"].(string); ok {
		span.SpanID = val
	}
	if val, ok := spanData["parent_span_id"].(string); ok && val != "" {
		span.ParentSpanID = val
	}
	if val, ok := spanData["name"].(string); ok {
		span.Name = val
	}
	if val, ok := spanData["kind"].(string); ok {
		span.Kind = spanKindToInt(val)
	}

	// Timestamp and duration
	// Pinot can return numeric values as different types, so we need to handle multiple cases
	var startTimeNanos int64
	if tsVal := spanData["timestamp"]; tsVal != nil {
		var tsMillis int64
		switch v := tsVal.(type) {
		case int64:
			tsMillis = v
		case int:
			tsMillis = int64(v)
		case float64:
			tsMillis = int64(v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				tsMillis = parsed
			}
		}
		if tsMillis > 0 {
			// Convert milliseconds to nanoseconds
			startTimeNanos = tsMillis * 1000000
			span.StartTimeUnixNano = fmt.Sprintf("%d", startTimeNanos)
		}
	}

	// Duration and end time calculation
	if durationVal := spanData["duration"]; durationVal != nil {
		var durationNanos int64
		switch v := durationVal.(type) {
		case int64:
			durationNanos = v
		case int:
			durationNanos = int64(v)
		case float64:
			durationNanos = int64(v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
				durationNanos = parsed
			}
		}
		if durationNanos > 0 {
			span.DurationNanos = fmt.Sprintf("%d", durationNanos)

			// Calculate end time (required by OTLP)
			if startTimeNanos > 0 {
				endTimeNanos := startTimeNanos + durationNanos
				span.EndTimeUnixNano = fmt.Sprintf("%d", endTimeNanos)
			}
		}
	}

	// Status
	if statusCode, ok := spanData["status_code"].(string); ok && statusCode != "" {
		span.Status.Code = statusCodeToInt(statusCode)
	}
	if statusMsg, ok := spanData["status_message"].(string); ok && statusMsg != "" {
		span.Status.Message = statusMsg
	}

	// Add native column attributes
	s.addNativeColumnAttributes(&span, spanData)

	// Parse JSON attributes if present
	if attrsJSON, ok := spanData["attributes"].(string); ok && attrsJSON != "" {
		// TODO: Parse JSON attributes and add to span.Attributes
		// For now, we'll just add a note that attributes exist
	}

	return span
}

// isValidAttributeValue checks if a value should be included as an attribute
func isValidAttributeValue(val string) bool {
	return val != "" && val != "null"
}

// addNativeColumnAttributes adds attributes from native Pinot columns
func (s *Server) addNativeColumnAttributes(span *TempoTraceSpan, spanData map[string]interface{}) {
	// HTTP attributes
	if val, ok := spanData["http_method"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "http.method",
			Value: stringValue(val),
		})
	}
	if val, ok := spanData["http_status_code"].(int); ok && val > 0 {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "http.status_code",
			Value: intValueFromInt(val),
		})
	}
	if val, ok := spanData["http_route"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "http.route",
			Value: stringValue(val),
		})
	}
	if val, ok := spanData["http_target"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "http.target",
			Value: stringValue(val),
		})
	}

	// DB attributes
	if val, ok := spanData["db_system"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "db.system",
			Value: stringValue(val),
		})
	}
	if val, ok := spanData["db_statement"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "db.statement",
			Value: stringValue(val),
		})
	}

	// Messaging attributes
	if val, ok := spanData["messaging_system"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "messaging.system",
			Value: stringValue(val),
		})
	}
	if val, ok := spanData["messaging_destination"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "messaging.destination",
			Value: stringValue(val),
		})
	}

	// RPC attributes
	if val, ok := spanData["rpc_service"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "rpc.service",
			Value: stringValue(val),
		})
	}
	if val, ok := spanData["rpc_method"].(string); ok && isValidAttributeValue(val) {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "rpc.method",
			Value: stringValue(val),
		})
	}

	// Error flag
	if val, ok := spanData["error"].(bool); ok {
		span.Attributes = append(span.Attributes, TempoAttribute{
			Key:   "error",
			Value: boolValue(val),
		})
	}
}
