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
		result, err := s.executeQuery(r.Context(), sql)
		if err != nil {
			s.sendErrorResponse(w, fmt.Sprintf("failed to execute query: %v", err))
			return
		}
		results = append(results, result)
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

// sendErrorResponse sends an error response
func (s *Server) sendErrorResponse(w http.ResponseWriter, errMsg string) {
	response := QueryResponse{
		Error: errMsg,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}
