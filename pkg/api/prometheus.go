package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/api/formats"
	"github.com/pilhuhn/otel-oql/pkg/promql"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
)

// handlePrometheusQuery handles Prometheus instant query endpoint: GET/POST /api/v1/query
func (s *Server) handlePrometheusQuery(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.prometheus.query")
	defer span.End()

	// Parse form parameters
	params, err := ParsePrometheusQueryParams(r)
	if err != nil {
		s.obs.RecordError(ctx, "invalid_params", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.PrometheusError("bad_data", err.Error()))
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query", time.Since(start), http.StatusUnauthorized)
		writeJSON(w, http.StatusUnauthorized, formats.PrometheusError("bad_data", "tenant-id not found"))
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Prometheus instant query (tenant_id=%d): %s\n", tenantID, params.Query)
	}

	// Translate PromQL to SQL
	translator := promql.NewTranslator(tenantID)
	sqlQueries, err := translator.TranslateQuery(params.Query)
	if err != nil {
		if s.debugTranslation {
			fmt.Printf("[DEBUG TRANSLATION] PromQL translation error: %v\n", err)
		}
		s.obs.RecordError(ctx, "translation_error", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.PrometheusError("bad_data", fmt.Sprintf("query parse error: %v", err)))
		return
	}

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] PromQL query translated to %d SQL statements:\n", len(sqlQueries))
		for i, sql := range sqlQueries {
			fmt.Printf("[DEBUG TRANSLATION]   [%d] %s\n", i+1, sql)
		}
	}

	// Execute queries against Pinot
	results, err := s.executeQueries(ctx, sqlQueries)
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query", time.Since(start), http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, formats.PrometheusError("execution", fmt.Sprintf("query execution failed: %v", err)))
		return
	}

	// Transform results to Prometheus format
	pinotResults := make([]formats.PinotResult, 0, len(results))
	for _, r := range results {
		pinotResults = append(pinotResults, formats.PinotResult{
			SQL:     r.SQL,
			Columns: r.Columns,
			Rows:    r.Rows,
		})
	}

	promResponse := formats.TransformToPrometheusInstant(pinotResults, params.Time)

	// Return response
	s.obs.RecordRequest(ctx, "/api/v1/query", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, promResponse)
}

// handlePrometheusQueryRange handles Prometheus range query endpoint: GET/POST /api/v1/query_range
func (s *Server) handlePrometheusQueryRange(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := s.obs.Tracer().Start(r.Context(), "api.prometheus.query_range")
	defer span.End()

	// Parse form parameters
	params, err := ParsePrometheusRangeParams(r)
	if err != nil {
		s.obs.RecordError(ctx, "invalid_params", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query_range", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.PrometheusError("bad_data", err.Error()))
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		s.obs.RecordError(ctx, "missing_tenant_id", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query_range", time.Since(start), http.StatusUnauthorized)
		writeJSON(w, http.StatusUnauthorized, formats.PrometheusError("bad_data", "tenant-id not found"))
		return
	}

	if s.debugQuery {
		fmt.Printf("[DEBUG QUERY] Prometheus range query (tenant_id=%d, start=%s, end=%s, step=%s): %s\n",
			tenantID, params.Start, params.End, params.Step, params.Query)
	}

	// Translate PromQL to SQL with time range
	translator := promql.NewTranslator(tenantID)
	sqlQueries, err := translator.TranslateQueryWithTimeRange(params.Query, &params.Start, &params.End)
	if err != nil {
		if s.debugTranslation {
			fmt.Printf("[DEBUG TRANSLATION] PromQL translation error: %v\n", err)
		}
		s.obs.RecordError(ctx, "translation_error", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query_range", time.Since(start), http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, formats.PrometheusError("bad_data", fmt.Sprintf("query parse error: %v", err)))
		return
	}

	if s.debugTranslation {
		fmt.Printf("[DEBUG TRANSLATION] PromQL range query translated to %d SQL statements:\n", len(sqlQueries))
		for i, sql := range sqlQueries {
			fmt.Printf("[DEBUG TRANSLATION]   [%d] %s\n", i+1, sql)
		}
	}

	// Execute queries against Pinot
	results, err := s.executeQueries(ctx, sqlQueries)
	if err != nil {
		s.obs.RecordError(ctx, "execution_error", "prometheus_api")
		s.obs.RecordRequest(ctx, "/api/v1/query_range", time.Since(start), http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, formats.PrometheusError("execution", fmt.Sprintf("query execution failed: %v", err)))
		return
	}

	// Transform results to Prometheus format
	pinotResults := make([]formats.PinotResult, 0, len(results))
	for _, r := range results {
		pinotResults = append(pinotResults, formats.PinotResult{
			SQL:     r.SQL,
			Columns: r.Columns,
			Rows:    r.Rows,
		})
	}

	promResponse := formats.TransformToPrometheusRange(pinotResults)

	// Return response
	s.obs.RecordRequest(ctx, "/api/v1/query_range", time.Since(start), http.StatusOK)
	writeJSON(w, http.StatusOK, promResponse)
}

// executeQueries executes SQL queries against Pinot and returns results
func (s *Server) executeQueries(ctx context.Context, sqlQueries []string) ([]QueryResult, error) {
	results := make([]QueryResult, 0, len(sqlQueries))

	for _, sql := range sqlQueries {
		// Execute query
		resp, err := s.pinotClient.Query(ctx, sql)
		if err != nil {
			return nil, fmt.Errorf("pinot query failed: %w", err)
		}

		results = append(results, QueryResult{
			SQL:     sql,
			Columns: resp.ResultTable.DataSchema.ColumnNames,
			Rows:    resp.ResultTable.Rows,
			Stats: QueryStats{
				NumDocsScanned: resp.NumDocsScanned,
				TotalDocs:      resp.TotalDocs,
				TimeUsedMs:     resp.TimeUsedMs,
			},
		})
	}

	return results, nil
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
