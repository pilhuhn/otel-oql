package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"github.com/pilhuhn/otel-oql/pkg/traceql"
)

// TempoTagsResponse represents the response for /api/v2/search/tags
type TempoTagsResponse struct {
	TagNames []string `json:"tagNames"`
}

// TempoTagValuesResponse represents the response for /api/v2/search/tag/{tag}/values
type TempoTagValuesResponse struct {
	TagValues []string `json:"tagValues"`
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
	values := []string{}
	if len(results) > 0 && len(results[0].Rows) > 0 {
		for _, row := range results[0].Rows {
			if len(row) > 0 {
				if val, ok := row[0].(string); ok && val != "" {
					values = append(values, val)
				}
			}
		}
	}

	response := TempoTagValuesResponse{
		TagValues: values,
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
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(attributes, '$.%s', 'STRING')", attrName)
}

// mapResourceAttributeToColumn maps a resource attribute to Pinot column
func (s *Server) mapResourceAttributeToColumn(attrName string) string {
	// Map OTel resource semantic conventions to native columns
	if attrName == "service.name" {
		return "service_name"
	}

	// Not a native column - use JSON extraction
	return fmt.Sprintf("JSON_EXTRACT_SCALAR(resource_attributes, '$.%s', 'STRING')", attrName)
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

// TempoSearchResponse represents the response for /api/v2/search
type TempoSearchResponse struct {
	Traces []TempoTrace `json:"traces"`
}

// TempoTrace represents a single trace in the search results
type TempoTrace struct {
	TraceID           string                 `json:"traceID"`
	RootServiceName   string                 `json:"rootServiceName,omitempty"`
	RootTraceName     string                 `json:"rootTraceName,omitempty"`
	StartTimeUnixNano int64                  `json:"startTimeUnixNano"`
	DurationMs        int                    `json:"durationMs"`
	SpanSets          []TempoSpanSet         `json:"spanSets,omitempty"`
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

		for _, span := range spans {
			// Extract span name for root
			if name, ok := span["name"].(string); ok && rootTraceName == "" {
				rootTraceName = name
			}

			// Extract service name for root
			if service, ok := span["service_name"].(string); ok && rootServiceName == "" {
				rootServiceName = service
			}

			// Calculate trace time bounds
			if ts, ok := span["timestamp"].(int64); ok {
				if minStartTime == 0 || ts < minStartTime {
					minStartTime = ts
				}
			}

			if duration, ok := span["duration"].(int64); ok {
				if ts, ok := span["timestamp"].(int64); ok {
					endTime := ts + (duration / 1000000) // Convert nanos to millis
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
		})
	}

	return traces
}
