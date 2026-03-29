package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/api/formats"
	"github.com/pilhuhn/otel-oql/pkg/logql"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
)

// handleLokiQuery handles Loki instant query endpoint: GET/POST /loki/api/v1/query
func (s *Server) handleLokiQuery(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.loki.query")
	defer span.End()

	// Parse form parameters
	params, err := ParseLokiQueryParams(r)
	if err != nil {
		s.obs.RecordError(ctx, "invalid_params", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.LokiError("bad_data", err.Error()))
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query", time.Since(start), http.StatusUnauthorized)
		writeJSON(w, http.StatusUnauthorized, formats.LokiError("bad_data", "tenant-id not found"))
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Loki instant query (tenant_id=%d, limit=%d): %s\n", tenantID, params.Limit, params.Query)
	}

	// Translate LogQL to SQL
	translator := logql.NewTranslator(tenantID)
	sqlQueries, err := translator.TranslateQuery(params.Query)
	if err != nil {
		if s.debugTranslation {
			fmt.Printf("[DEBUG TRANSLATION] LogQL translation error: %v\n", err)
		}
		s.obs.RecordError(ctx, "translation_error", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.LokiError("bad_data", fmt.Sprintf("query parse error: %v", err)))
		return
	}

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] LogQL query translated to %d SQL statements:\n", len(sqlQueries))
		for i, sql := range sqlQueries {
			fmt.Printf("[DEBUG TRANSLATION]   [%d] %s\n", i+1, sql)
		}
	}

	// Execute queries against Pinot
	results, err := s.executeQueries(ctx, sqlQueries)
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query", time.Since(start), http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, formats.LokiError("execution", fmt.Sprintf("query execution failed: %v", err)))
		return
	}

	// Transform results to Loki format
	pinotResults := make([]formats.PinotResult, 0, len(results))
	for _, r := range results {
		pinotResults = append(pinotResults, formats.PinotResult{
			SQL:     r.SQL,
			Columns: r.Columns,
			Rows:    r.Rows,
		})
	}

	// Detect if this is a metric query or log stream query
	var lokiResponse formats.LokiResponse
	if isMetricQuery(pinotResults) {
		lokiResponse = formats.TransformToLokiMatrix(pinotResults)
	} else {
		lokiResponse = formats.TransformToLokiStreams(pinotResults, params.Limit, params.Direction)
	}

	// Return response
	s.obs.RecordRequest(ctx, "/loki/api/v1/query", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, lokiResponse)
}

// handleLokiQueryRange handles Loki range query endpoint: GET/POST /loki/api/v1/query_range
func (s *Server) handleLokiQueryRange(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.loki.query_range")
	defer span.End()

	// Parse form parameters
	params, err := ParseLokiRangeParams(r)
	if err != nil {
		s.obs.RecordError(ctx, "invalid_params", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query_range", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.LokiError("bad_data", err.Error()))
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query_range", time.Since(start), http.StatusUnauthorized)
		writeJSON(w, http.StatusUnauthorized, formats.LokiError("bad_data", "tenant-id not found"))
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Loki range query (tenant_id=%d, start=%s, end=%s, limit=%d): %s\n",
			tenantID, params.Start, params.End, params.Limit, params.Query)
	}

	// Translate LogQL to SQL with time range
	translator := logql.NewTranslator(tenantID)
	sqlQueries, err := translator.TranslateQueryWithTimeRange(params.Query, &params.Start, &params.End)
	if err != nil {
		if s.debugTranslation {
			fmt.Printf("[DEBUG TRANSLATION] LogQL translation error: %v\n", err)
		}
		s.obs.RecordError(ctx, "translation_error", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query_range", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.LokiError("bad_data", fmt.Sprintf("query parse error: %v", err)))
		return
	}

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] LogQL range query translated to %d SQL statements:\n", len(sqlQueries))
		for i, sql := range sqlQueries {
			fmt.Printf("[DEBUG TRANSLATION]   [%d] %s\n", i+1, sql)
		}
	}

	// Execute queries against Pinot
	results, err := s.executeQueries(ctx, sqlQueries)
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/query_range", time.Since(start), http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, formats.LokiError("execution", fmt.Sprintf("query execution failed: %v", err)))
		return
	}

	// Transform results to Loki format
	pinotResults := make([]formats.PinotResult, 0, len(results))
	for _, r := range results {
		pinotResults = append(pinotResults, formats.PinotResult{
			SQL:     r.SQL,
			Columns: r.Columns,
			Rows:    r.Rows,
		})
	}

	// Detect if this is a metric query or log stream query
	var lokiResponse formats.LokiResponse
	if isMetricQuery(pinotResults) {
		lokiResponse = formats.TransformToLokiMatrix(pinotResults)
	} else {
		lokiResponse = formats.TransformToLokiStreams(pinotResults, params.Limit, params.Direction)
	}

	// Return response
	s.obs.RecordRequest(ctx, "/loki/api/v1/query_range", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, lokiResponse)
}

// isMetricQuery detects if query results represent a metric query (has value column)
// or a log stream query (has body column)
func isMetricQuery(results []formats.PinotResult) bool {
	if len(results) == 0 {
		return false
	}

	// Check for presence of value/cnt column (metric) vs body column (log stream)
	result := results[0]
	for _, col := range result.Columns {
		colLower := strings.ToLower(col)
		if colLower == "value" || colLower == "cnt" {
			return true
		}
	}

	return false
}

// handleLokiLabels handles GET/POST /loki/api/v1/labels
func (s *Server) handleLokiLabels(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.loki.labels")
	defer span.End()

	// Parse form parameters
	params, err := ParseLokiLabelsParams(r)
	if err != nil {
		s.obs.RecordError(ctx, "invalid_params", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/labels", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.LokiError("bad_data", err.Error()))
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/labels", time.Since(start), http.StatusUnauthorized)
		writeJSON(w, http.StatusUnauthorized, formats.LokiError("bad_data", "tenant-id not found"))
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Loki labels query (tenant_id=%d, start=%v, end=%v)\n", 
			tenantID, params.Start, params.End)
	}

	// Build SQL to get distinct label names from logs table
	sql := s.buildLokiLabelsSQL(tenantID, params)

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] Loki labels query SQL: %s\n", sql)
	}

	// Execute query against Pinot
	results, err := s.executeQueries(ctx, []string{sql})
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/labels", time.Since(start), http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, formats.LokiError("execution", fmt.Sprintf("query execution failed: %v", err)))
		return
	}

	// Transform results to Loki format (reuse Prometheus transformer structure)
	pinotResults := make([]formats.PinotResult, 0, len(results))
	for _, r := range results {
		pinotResults = append(pinotResults, formats.PinotResult{
			SQL:     r.SQL,
			Columns: r.Columns,
			Rows:    r.Rows,
		})
	}
	
	response := formats.TransformToPrometheusLabels(pinotResults)

	// Return response
	s.obs.RecordRequest(ctx, "/loki/api/v1/labels", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, response)
}

// handleLokiLabelValues handles GET/POST /loki/api/v1/label/{name}/values
func (s *Server) handleLokiLabelValues(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.loki.label_values")
	defer span.End()

	// Extract label name from URL path
	// Path is /loki/api/v1/label/{name}/values
	path := r.URL.Path
	// Remove prefix /loki/api/v1/label/ and suffix /values
	labelName := path[len("/loki/api/v1/label/") : len(path)-len("/values")]

	// Parse form parameters
	params, err := ParseLokiLabelValuesParams(r, labelName)
	if err != nil {
		s.obs.RecordError(ctx, "invalid_params", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/label/values", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.LokiError("bad_data", err.Error()))
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/label/values", time.Since(start), http.StatusUnauthorized)
		writeJSON(w, http.StatusUnauthorized, formats.LokiError("bad_data", "tenant-id not found"))
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Loki label values query (tenant_id=%d, label=%s, start=%v, end=%v)\n",
			tenantID, labelName, params.Start, params.End)
	}

	// Build SQL to get distinct label values
	sql := s.buildLokiLabelValuesSQL(tenantID, params)

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] Loki label values query SQL: %s\n", sql)
	}

	// Execute query against Pinot
	results, err := s.executeQueries(ctx, []string{sql})
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "loki_api")
		s.obs.RecordRequest(ctx, "/loki/api/v1/label/values", time.Since(start), http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, formats.LokiError("execution", fmt.Sprintf("query execution failed: %v", err)))
		return
	}

	// Transform results to Loki format (reuse Prometheus transformer structure)
	pinotResults := make([]formats.PinotResult, 0, len(results))
	for _, r := range results {
		pinotResults = append(pinotResults, formats.PinotResult{
			SQL:     r.SQL,
			Columns: r.Columns,
			Rows:    r.Rows,
		})
	}
	response := formats.TransformToPrometheusLabelValues(pinotResults)

	// Return response
	s.obs.RecordRequest(ctx, "/loki/api/v1/label/values", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, response)
}

// buildLokiLabelsSQL constructs SQL to get distinct label names from logs
func (s *Server) buildLokiLabelsSQL(tenantID int, params *LokiLabelsParams) string {
	// Return common log label columns
	// This is a simplified implementation - could query schema dynamically
	sql := fmt.Sprintf("SELECT DISTINCT service_name FROM otel_logs WHERE tenant_id = %d", tenantID)

	// Add time range if provided (Loki timestamps are in nanoseconds, convert to milliseconds)
	if !params.Start.IsZero() && !params.End.IsZero() {
		sql += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d",
			params.Start.UnixMilli(), params.End.UnixMilli())
	}

	if params.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", params.Limit)
	}

	return sql
}

// buildLokiLabelValuesSQL constructs SQL to get distinct values for a specific label
func (s *Server) buildLokiLabelValuesSQL(tenantID int, params *LokiLabelValuesParams) string {
	var column string

	// Map common label names to native columns
	switch params.LabelName {
	case "service_name", "job":
		column = "service_name"
	case "host_name", "instance":
		column = "host_name"
	case "log_level", "level":
		column = "log_level"
	case "log_source":
		column = "log_source"
	case "environment":
		column = "environment"
	case "trace_id":
		column = "trace_id"
	case "span_id":
		column = "span_id"
	default:
		// For unknown labels, use the label name directly
		column = params.LabelName
	}

	sql := fmt.Sprintf("SELECT DISTINCT %s FROM otel_logs WHERE tenant_id = %d AND %s IS NOT NULL",
		column, tenantID, column)

	// Add time range if provided (Loki timestamps are in nanoseconds, convert to milliseconds)
	if !params.Start.IsZero() && !params.End.IsZero() {
		sql += fmt.Sprintf(" AND \"timestamp\" >= %d AND \"timestamp\" <= %d",
			params.Start.UnixMilli(), params.End.UnixMilli())
	}

	if params.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", params.Limit)
	}

	return sql
}
