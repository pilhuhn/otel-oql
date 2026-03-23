package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pilhuhn/otel-oql/pkg/oql"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"github.com/pilhuhn/otel-oql/pkg/translator"
)

// Server is the OQL query API server
type Server struct {
	port         int
	validator    *tenant.Validator
	pinotClient  *pinot.Client
	httpServer   *http.Server
}

// NewServer creates a new query API server
func NewServer(port int, validator *tenant.Validator, pinotClient *pinot.Client) *Server {
	return &Server{
		port:        port,
		validator:   validator,
		pinotClient: pinotClient,
	}
}

// Start starts the query API server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Query endpoint with tenant validation
	mux.Handle("/query", s.validator.HTTPMiddleware(http.HandlerFunc(s.handleQuery)))

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Query API server error: %v\n", err)
		}
	}()

	fmt.Printf("Query API server listening on port %d\n", s.port)
	return nil
}

// Stop stops the query API server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// QueryRequest represents a query request
type QueryRequest struct {
	Query string `json:"query"`
}

// QueryResponse represents a query response
type QueryResponse struct {
	Results []QueryResult `json:"results"`
	Error   string        `json:"error,omitempty"`
}

// QueryResult represents a single query result
type QueryResult struct {
	SQL     string          `json:"sql"`
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Stats   QueryStats      `json:"stats"`
}

// QueryStats represents query statistics
type QueryStats struct {
	NumDocsScanned int64 `json:"numDocsScanned"`
	TotalDocs      int64 `json:"totalDocs"`
	TimeUsedMs     int64 `json:"timeUsedMs"`
}

// handleQuery handles OQL query requests
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Parse request
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Parse OQL query
	parser := oql.NewParser(req.Query)
	query, err := parser.Parse()
	if err != nil {
		s.sendErrorResponse(w, fmt.Sprintf("failed to parse query: %v", err))
		return
	}

	// Translate to SQL
	trans := translator.NewTranslator(tenantID)
	sqlQueries, err := trans.TranslateQuery(query)
	if err != nil {
		s.sendErrorResponse(w, fmt.Sprintf("failed to translate query: %v", err))
		return
	}

	// Execute queries
	results := make([]QueryResult, 0)
	for _, sql := range sqlQueries {
		// Check if this is an expand operation (marker format)
		if result, err := s.executeExpandQuery(r.Context(), sql, tenantID); err == nil {
			results = append(results, result)
		} else if err.Error() == "not an expand query" {
			// Regular query
			result, err := s.executeQuery(r.Context(), sql)
			if err != nil {
				s.sendErrorResponse(w, fmt.Sprintf("failed to execute query: %v", err))
				return
			}
			results = append(results, result)
		} else {
			s.sendErrorResponse(w, fmt.Sprintf("failed to execute expand query: %v", err))
			return
		}
	}

	// Send response
	response := QueryResponse{
		Results: results,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// executeQuery executes a SQL query against Pinot
func (s *Server) executeQuery(ctx context.Context, sql string) (QueryResult, error) {
	resp, err := s.pinotClient.Query(ctx, sql)
	if err != nil {
		return QueryResult{}, err
	}

	result := QueryResult{
		SQL:     sql,
		Columns: resp.ResultTable.DataSchema.ColumnNames,
		Rows:    resp.ResultTable.Rows,
		Stats: QueryStats{
			NumDocsScanned: resp.NumDocsScanned,
			TotalDocs:      resp.TotalDocs,
			TimeUsedMs:     resp.TimeUsedMs,
		},
	}

	return result, nil
}

// executeExpandQuery executes an expand operation in two steps
// Returns error "not an expand query" if the SQL is not an expand marker
func (s *Server) executeExpandQuery(ctx context.Context, sql string, tenantID int) (QueryResult, error) {
	// Check if this is an expand marker
	const expandPrefix = "__EXPAND_TRACE__"
	const tableSuffix = "__TABLE__"
	const expandSuffix = "__END_EXPAND__"

	if len(sql) < len(expandPrefix)+len(expandSuffix) {
		return QueryResult{}, fmt.Errorf("not an expand query")
	}

	if sql[:len(expandPrefix)] != expandPrefix {
		return QueryResult{}, fmt.Errorf("not an expand query")
	}

	// Extract base SQL and table name
	rest := sql[len(expandPrefix):]
	tableIdx := len(rest) - len(tableSuffix) - len(expandSuffix)
	if tableIdx < 0 {
		return QueryResult{}, fmt.Errorf("invalid expand marker format")
	}

	// Find __TABLE__ marker
	tableMarkerIdx := -1
	for i := 0; i < len(rest)-len(tableSuffix)-len(expandSuffix); i++ {
		if rest[i:i+len(tableSuffix)] == tableSuffix {
			tableMarkerIdx = i
			break
		}
	}

	if tableMarkerIdx == -1 {
		return QueryResult{}, fmt.Errorf("table marker not found in expand query")
	}

	baseSQL := rest[:tableMarkerIdx]
	tableAndEnd := rest[tableMarkerIdx+len(tableSuffix):]
	tableName := tableAndEnd[:len(tableAndEnd)-len(expandSuffix)]

	// Step 1: Execute base query to get trace_ids
	fmt.Printf("DEBUG EXPAND: Executing base query to get trace_ids\n")
	resp1, err := s.pinotClient.Query(ctx, baseSQL)
	if err != nil {
		return QueryResult{}, fmt.Errorf("failed to execute base query for expand: %w", err)
	}

	// Step 2: Extract unique trace_ids from results
	traceIDSet := make(map[string]bool)
	traceIDColIdx := -1

	// Find trace_id column index
	for i, colName := range resp1.ResultTable.DataSchema.ColumnNames {
		if colName == "trace_id" {
			traceIDColIdx = i
			break
		}
	}

	if traceIDColIdx == -1 {
		return QueryResult{}, fmt.Errorf("trace_id column not found in base query results")
	}

	// Collect unique trace_ids (filter out empty strings)
	for _, row := range resp1.ResultTable.Rows {
		if traceIDColIdx < len(row) {
			if traceID, ok := row[traceIDColIdx].(string); ok && traceID != "" {
				traceIDSet[traceID] = true
			}
		}
	}

	if len(traceIDSet) == 0 {
		// No trace_ids found, return empty result
		return QueryResult{
			SQL:     fmt.Sprintf("-- Expand query found no trace_ids in base query: %s", baseSQL),
			Columns: []string{},
			Rows:    [][]interface{}{},
			Stats: QueryStats{
				NumDocsScanned: 0,
				TotalDocs:      0,
				TimeUsedMs:     0,
			},
		}, nil
	}

	// Step 3: Build IN clause with trace_ids
	traceIDs := make([]string, 0, len(traceIDSet))
	for traceID := range traceIDSet {
		traceIDs = append(traceIDs, fmt.Sprintf("'%s'", traceID))
	}

	expandSQL := fmt.Sprintf(
		"SELECT * FROM %s WHERE tenant_id = %d AND trace_id IN (%s)",
		tableName,
		tenantID,
		join(traceIDs, ", "),
	)

	fmt.Printf("DEBUG EXPAND: Executing expand query with %d trace_ids\n", len(traceIDs))

	// Step 4: Execute the expand query
	resp2, err := s.pinotClient.Query(ctx, expandSQL)
	if err != nil {
		return QueryResult{}, fmt.Errorf("failed to execute expand query: %w", err)
	}

	result := QueryResult{
		SQL:     expandSQL,
		Columns: resp2.ResultTable.DataSchema.ColumnNames,
		Rows:    resp2.ResultTable.Rows,
		Stats: QueryStats{
			NumDocsScanned: resp2.NumDocsScanned,
			TotalDocs:      resp2.TotalDocs,
			TimeUsedMs:     resp2.TimeUsedMs,
		},
	}

	return result, nil
}

// join is a helper function to join strings
func join(strings []string, sep string) string {
	if len(strings) == 0 {
		return ""
	}
	result := strings[0]
	for i := 1; i < len(strings); i++ {
		result += sep + strings[i]
	}
	return result
}

// sendErrorResponse sends an error response
func (s *Server) sendErrorResponse(w http.ResponseWriter, errMsg string) {
	response := QueryResponse{
		Error: errMsg,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}
