package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pilhuhn/otel-oql/pkg/oql"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/translator"
)

// PinotQuerier defines the interface for executing Pinot queries
type PinotQuerier interface {
	Query(ctx context.Context, sql string) (*pinot.QueryResponse, error)
}

// Server implements an MCP (Model Context Protocol) server for OQL queries
type Server struct {
	port        int
	pinotClient PinotQuerier
	httpServer  *http.Server
}

// NewServer creates a new MCP server
func NewServer(port int, pinotClient PinotQuerier) *Server {
	return &Server{
		port:        port,
		pinotClient: pinotClient,
	}
}

// Start starts the MCP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// MCP endpoints
	mux.HandleFunc("/mcp/v1/tools/list", s.handleListTools)
	mux.HandleFunc("/mcp/v1/tools/call", s.handleCallTool)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.enableCORS(mux),
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("MCP server error: %v\n", err)
		}
	}()

	fmt.Printf("MCP server listening on port %d\n", s.port)
	return nil
}

// Stop stops the MCP server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// enableCORS adds CORS headers for MCP clients
func (s *Server) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ToolDefinition defines an MCP tool
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolsListResponse is the response for /tools/list
type ToolsListResponse struct {
	Tools []ToolDefinition `json:"tools"`
}

// ToolCallRequest is the request for /tools/call
type ToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResponse is the response for /tools/call
type ToolCallResponse struct {
	Content []ContentBlock `json:"content,omitempty"`
	Error   *ErrorDetail   `json:"error,omitempty"`
}

// ContentBlock represents a content block in the response
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ErrorDetail represents an error in the response
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// handleListTools returns the list of available MCP tools
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tools := []ToolDefinition{
		{
			Name: "oql_query",
			Description: `Execute OQL queries for observability data. Supports spans, metrics, and logs with:
• Filtering: where, filter
• Cross-signal correlation: correlate, expand trace, get_exemplars()
• Aggregation: avg, min, max, sum, count, group by
• Time ranges: since, between
• Limits: limit
Use oql_help tool for full documentation and examples.`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"tenant_id": map[string]interface{}{
						"type":        "integer",
						"description": "Tenant ID for multi-tenant isolation (use 0 for test mode)",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "OQL query to execute (e.g., 'signal=spans | where duration > 500ms | limit 10')",
					},
				},
				"required": []string{"tenant_id", "query"},
			},
		},
		{
			Name:        "oql_help",
			Description: "Get comprehensive OQL documentation with examples. Optional topic filtering for specific sections.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"topic": map[string]interface{}{
						"type":        "string",
						"description": "Filter help by topic: 'operators', 'examples', 'syntax', 'signals', 'all' (default: 'all')",
						"enum":        []string{"operators", "examples", "syntax", "signals", "all"},
					},
				},
				"required": []string{},
			},
		},
	}

	response := ToolsListResponse{Tools: tools}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCallTool executes an MCP tool
func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, "invalid_request", fmt.Sprintf("failed to parse request: %v", err))
		return
	}

	switch req.Name {
	case "oql_query":
		s.handleOQLQuery(w, req.Arguments)
	case "oql_help":
		s.handleOQLHelp(w, req.Arguments)
	default:
		s.sendErrorResponse(w, "tool_not_found", fmt.Sprintf("unknown tool: %s", req.Name))
	}
}

// handleOQLQuery executes an OQL query
func (s *Server) handleOQLQuery(w http.ResponseWriter, args map[string]interface{}) {
	// Extract tenant_id
	tenantID, ok := args["tenant_id"].(float64)
	if !ok {
		s.sendErrorResponse(w, "invalid_argument", "tenant_id is required and must be an integer")
		return
	}

	// Extract query
	queryStr, ok := args["query"].(string)
	if !ok || queryStr == "" {
		s.sendErrorResponse(w, "invalid_argument", "query is required and must be a non-empty string")
		return
	}

	// Parse OQL query
	parser := oql.NewParser(queryStr)
	query, err := parser.Parse()
	if err != nil {
		s.sendErrorResponse(w, "parse_error", fmt.Sprintf("failed to parse query: %v", err))
		return
	}

	// Translate to SQL
	trans := translator.NewTranslator(int(tenantID))
	sqlQueries, err := trans.TranslateQuery(query)
	if err != nil {
		s.sendErrorResponse(w, "translation_error", fmt.Sprintf("failed to translate query: %v", err))
		return
	}

	// Execute queries
	results := make([]string, 0)
	for _, sql := range sqlQueries {
		// For now, just execute the SQL directly (no expand/correlate special handling)
		resp, err := s.pinotClient.Query(context.Background(), sql)
		if err != nil {
			s.sendErrorResponse(w, "query_error", err.Error())
			return
		}

		// Format results as text
		resultText := s.formatQueryResult(resp, sql)
		results = append(results, resultText)
	}

	// Send success response
	response := ToolCallResponse{
		Content: []ContentBlock{
			{
				Type: "text",
				Text: strings.Join(results, "\n\n---\n\n"),
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleOQLHelp returns OQL documentation
func (s *Server) handleOQLHelp(w http.ResponseWriter, args map[string]interface{}) {
	topic := "all"
	if t, ok := args["topic"].(string); ok && t != "" {
		topic = t
	}

	// Read OQL_REFERENCE.md
	content, err := os.ReadFile("OQL_REFERENCE.md")
	if err != nil {
		s.sendErrorResponse(w, "file_error", fmt.Sprintf("failed to read OQL reference: %v", err))
		return
	}

	helpText := string(content)

	// Filter by topic if requested
	if topic != "all" {
		helpText = s.filterHelpByTopic(helpText, topic)
	}

	response := ToolCallResponse{
		Content: []ContentBlock{
			{
				Type: "text",
				Text: helpText,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// filterHelpByTopic filters the help text by topic
func (s *Server) filterHelpByTopic(content string, topic string) string {
	switch topic {
	case "operators":
		// Extract "Core Operations" section
		return s.extractSection(content, "## Core Operations", "## Aggregation Operations")
	case "examples":
		// Extract "Complete Examples" section
		return s.extractSection(content, "## Complete Examples", "## Query Syntax Notes")
	case "syntax":
		// Extract "Query Syntax Notes" section
		return s.extractSection(content, "## Query Syntax Notes", "## Operator Precedence")
	case "signals":
		// Extract "Signal-Specific Fields" section
		return s.extractSection(content, "## Signal-Specific Fields", "## Best Practices")
	default:
		return content
	}
}

// extractSection extracts a section from the markdown content
func (s *Server) extractSection(content, startMarker, endMarker string) string {
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return "Section not found"
	}

	endIdx := strings.Index(content[startIdx:], endMarker)
	if endIdx == -1 {
		// Return from start to end of document
		return content[startIdx:]
	}

	return content[startIdx : startIdx+endIdx]
}

// formatQueryResult formats Pinot query results as text
func (s *Server) formatQueryResult(resp *pinot.QueryResponse, sql string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("SQL: %s\n\n", sql))

	// Write column headers
	if len(resp.ResultTable.DataSchema.ColumnNames) > 0 {
		sb.WriteString(strings.Join(resp.ResultTable.DataSchema.ColumnNames, "\t"))
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat("-", 80))
		sb.WriteString("\n")
	}

	// Write rows
	for _, row := range resp.ResultTable.Rows {
		values := make([]string, len(row))
		for i, val := range row {
			values[i] = fmt.Sprintf("%v", val)
		}
		sb.WriteString(strings.Join(values, "\t"))
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("\n%d row(s) returned\n", len(resp.ResultTable.Rows)))
	sb.WriteString(fmt.Sprintf("Query time: %dms\n", resp.TimeUsedMs))

	return sb.String()
}

// sendErrorResponse sends an error response
func (s *Server) sendErrorResponse(w http.ResponseWriter, code string, message string) {
	response := ToolCallResponse{
		Error: &ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}
