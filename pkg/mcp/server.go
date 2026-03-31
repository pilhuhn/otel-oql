package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pilhuhn/otel-oql/pkg/oql"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/translator"
)

//go:embed OQL_REFERENCE.md
var oqlReferenceContent string

// PinotQuerier defines the interface for executing Pinot queries
type PinotQuerier interface {
	Query(ctx context.Context, sql string) (*pinot.QueryResponse, error)
}

// Server wraps the MCP SDK server with our Pinot client
type Server struct {
	port        int
	pinotClient PinotQuerier
	mcpServer   *mcp.Server
	httpServer  *http.Server
}

// NewServer creates a new MCP server using the official SDK
func NewServer(port int, pinotClient PinotQuerier) *Server {
	// Create the MCP server using the SDK
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "otel-oql-mcp",
		Version: "1.0.0",
	}, nil)

	s := &Server{
		port:        port,
		pinotClient: pinotClient,
		mcpServer:   mcpServer,
	}

	// Register tools
	s.registerTools()

	return s
}

// OQLQueryArgs defines the parameters for the oql_query tool
type OQLQueryArgs struct {
	TenantID int    `json:"tenant_id" jsonschema:"Tenant ID for multi-tenant isolation"`
	Query    string `json:"query" jsonschema:"OQL query to execute"`
}

// OQLHelpArgs defines the parameters for the oql_help tool
type OQLHelpArgs struct {
	Topic string `json:"topic,omitempty" jsonschema:"Specific help topic (operators, examples, functions, etc.)"`
}

// registerTools adds the OQL tools to the MCP server
func (s *Server) registerTools() {
	// Add oql_query tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "oql_query",
		Description: `Execute OQL queries for observability data.

OQL (Observability Query Language) enables powerful cross-signal correlation:
- Query spans, logs, and metrics with a unified syntax
- Use 'expand trace' to reconstruct full trace waterfalls
- Use 'correlate' to find related logs and metrics
- Use 'get_exemplars()' to jump from aggregated metrics to specific traces

Examples:
  signal=spans | where duration > 500ms | limit 10
  signal=spans | where name == "checkout" | expand trace
  signal=metrics | where value > 2s | get_exemplars() | expand trace | correlate logs`,
	}, s.handleOQLQuery)

	// Add oql_help tool
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "oql_help",
		Description: "Get comprehensive OQL documentation including syntax, operators, examples, and best practices",
	}, s.handleOQLHelp)
}

// handleOQLQuery implements the oql_query tool handler
func (s *Server) handleOQLQuery(ctx context.Context, req *mcp.CallToolRequest, args OQLQueryArgs) (*mcp.CallToolResult, any, error) {
	// Validate tenant ID
	if args.TenantID < 0 {
		return nil, nil, fmt.Errorf("invalid tenant_id: must be >= 0")
	}

	// Validate query
	if args.Query == "" {
		return nil, nil, fmt.Errorf("query is required")
	}

	// Parse OQL query
	parser := oql.NewParser(args.Query)
	ast, err := parser.Parse()
	if err != nil {
		return nil, nil, fmt.Errorf("parse error: %w", err)
	}

	// Translate to SQL
	trans := translator.NewTranslator(args.TenantID)
	sqlQueries, err := trans.TranslateQuery(ast)
	if err != nil {
		return nil, nil, fmt.Errorf("translation error: %w", err)
	}

	// Execute queries
	var results []string
	for _, sql := range sqlQueries {
		resp, err := s.pinotClient.Query(ctx, sql)
		if err != nil {
			return nil, nil, fmt.Errorf("query error: %w", err)
		}
		results = append(results, formatQueryResponse(sql, resp))
	}

	// Return result
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: strings.Join(results, "\n\n")},
		},
	}, nil, nil
}

// handleOQLHelp implements the oql_help tool handler
func (s *Server) handleOQLHelp(ctx context.Context, req *mcp.CallToolRequest, args OQLHelpArgs) (*mcp.CallToolResult, any, error) {
	// Use embedded OQL reference content
	helpText := oqlReferenceContent

	// If a specific topic is requested, try to extract that section
	if args.Topic != "" {
		section := extractSection(helpText, args.Topic)
		if section != "" {
			helpText = section
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: helpText},
		},
	}, nil, nil
}

// Start starts the MCP HTTP server
func (s *Server) Start() error {
	// Create the Streamable HTTP handler using the SDK
	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return s.mcpServer
	}, nil)

	addr := fmt.Sprintf(":%d", s.port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	fmt.Printf("MCP server listening on port %d (Streamable HTTP transport)\n", s.port)
	fmt.Printf("MCP endpoints:\n")
	fmt.Printf("  - POST /mcp (Streamable HTTP)\n")
	fmt.Printf("\n")
	fmt.Printf("Available tools:\n")
	fmt.Printf("  - oql_query: Execute OQL queries\n")
	fmt.Printf("  - oql_help: Get OQL documentation\n")

	return s.httpServer.ListenAndServe()
}

// Stop stops the MCP server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// formatQueryResponse formats a Pinot query response as text
func formatQueryResponse(sql string, resp *pinot.QueryResponse) string {
	var sb strings.Builder

	if len(resp.ResultTable.Rows) == 0 {
		sb.WriteString("No results\n")
		return sb.String()
	}

	// Column headers
	headers := make([]string, len(resp.ResultTable.DataSchema.ColumnNames))
	copy(headers, resp.ResultTable.DataSchema.ColumnNames)
	sb.WriteString(strings.Join(headers, "\t"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("-", 80))
	sb.WriteString("\n")

	// Rows
	for _, row := range resp.ResultTable.Rows {
		rowStrs := make([]string, len(row))
		for i, val := range row {
			rowStrs[i] = fmt.Sprintf("%v", val)
		}
		sb.WriteString(strings.Join(rowStrs, "\t"))
		sb.WriteString("\n")
	}

	// Summary
	sb.WriteString(fmt.Sprintf("\n%d row(s) returned\n", len(resp.ResultTable.Rows)))
	if resp.TimeUsedMs > 0 {
		sb.WriteString(fmt.Sprintf("Query time: %dms\n", resp.TimeUsedMs))
	}

	return sb.String()
}

// extractSection extracts a section from the help text based on topic
func extractSection(content, topic string) string {
	// Simple implementation: look for a heading that matches the topic
	lines := strings.Split(content, "\n")
	var section []string
	inSection := false
	topicLower := strings.ToLower(topic)

	for _, line := range lines {
		// Check if this is a heading that matches our topic
		if strings.HasPrefix(line, "#") {
			headingText := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if strings.Contains(strings.ToLower(headingText), topicLower) {
				inSection = true
				section = append(section, line)
				continue
			} else if inSection && strings.HasPrefix(line, "#") {
				// We hit another heading, stop
				break
			}
		}

		if inSection {
			section = append(section, line)
		}
	}

	if len(section) > 0 {
		return strings.Join(section, "\n")
	}

	return "" // Topic not found
}
