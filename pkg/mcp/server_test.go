package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPinotClient implements PinotQuerier for testing
type MockPinotClient struct {
	QueryFunc func(ctx context.Context, sql string) (*pinot.QueryResponse, error)
}

func (m *MockPinotClient) Query(ctx context.Context, sql string) (*pinot.QueryResponse, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, sql)
	}
	resp := &pinot.QueryResponse{
		TimeUsedMs: 5,
	}
	resp.ResultTable.DataSchema.ColumnNames = []string{"trace_id", "duration", "name"}
	resp.ResultTable.DataSchema.ColumnDataTypes = []string{"STRING", "LONG", "STRING"}
	resp.ResultTable.Rows = [][]interface{}{
		{"trace-123", int64(1500000000), "test-span"},
	}
	return resp, nil
}

func TestMCP_SDK_Initialize(t *testing.T) {
	mockClient := &MockPinotClient{}
	server := NewServer(8090, mockClient)

	// Create a test HTTP server using the SDK's handler
	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Create an MCP client
	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	// Connect to the server (this automatically does the initialize handshake)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer session.Close()

	assert.NotEmpty(t, session.ID(), "session should have an ID")
}

func TestMCP_SDK_ToolsList(t *testing.T) {
	mockClient := &MockPinotClient{}
	server := NewServer(8090, mockClient)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer session.Close()

	// List tools
	toolsResult, err := session.ListTools(ctx, nil)
	require.NoError(t, err)

	// Should have 2 tools: oql_query and oql_help
	require.Len(t, toolsResult.Tools, 2, "should have 2 tools")

	toolNames := make([]string, len(toolsResult.Tools))
	for i, tool := range toolsResult.Tools {
		toolNames[i] = tool.Name
	}
	assert.Contains(t, toolNames, "oql_query")
	assert.Contains(t, toolNames, "oql_help")
}

func TestMCP_SDK_OQLQuery(t *testing.T) {
	mockClient := &MockPinotClient{
		QueryFunc: func(ctx context.Context, sql string) (*pinot.QueryResponse, error) {
			resp := &pinot.QueryResponse{
				TimeUsedMs: 5,
			}
			resp.ResultTable.DataSchema.ColumnNames = []string{"trace_id", "duration", "name"}
			resp.ResultTable.DataSchema.ColumnDataTypes = []string{"STRING", "LONG", "STRING"}
			resp.ResultTable.Rows = [][]interface{}{
				{"trace-123", int64(1500000000), "test-span"},
				{"trace-456", int64(2000000000), "another-span"},
			}
			return resp, nil
		},
	}
	server := NewServer(8090, mockClient)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call oql_query tool
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "oql_query",
		Arguments: map[string]any{
			"tenant_id": 0,
			"query":     "signal=spans | where duration > 500ms | limit 10",
		},
	})
	require.NoError(t, err)

	// Check that we got content back
	require.Greater(t, len(result.Content), 0, "should have content")

	// Check that it's text content
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "content should be text")
	assert.Contains(t, textContent.Text, "trace-123", "should contain trace data")
	assert.Contains(t, textContent.Text, "test-span", "should contain span name")
}

func TestMCP_SDK_InvalidQuery(t *testing.T) {
	mockClient := &MockPinotClient{}
	server := NewServer(8090, mockClient)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call oql_query with invalid query
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "oql_query",
		Arguments: map[string]any{
			"tenant_id": 0,
			"query":     "invalid syntax here",
		},
	})
	require.NoError(t, err, "should not have transport error")
	require.True(t, result.IsError, "result should be marked as error")
	require.Greater(t, len(result.Content), 0, "should have error content")

	// Error is returned as TextContent
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "error content should be text")
	assert.Contains(t, strings.ToLower(textContent.Text), "parse", "error should mention parse error")
}

func TestMCP_SDK_MissingArguments(t *testing.T) {
	mockClient := &MockPinotClient{}
	server := NewServer(8090, mockClient)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call oql_query without query argument
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "oql_query",
		Arguments: map[string]any{
			"tenant_id": 0,
			// Missing "query" field
		},
	})
	require.Error(t, err, "should return error for missing query")
}

func TestMCP_SDK_OQLHelp(t *testing.T) {
	mockClient := &MockPinotClient{}
	server := NewServer(8090, mockClient)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call oql_help tool
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "oql_help",
		Arguments: map[string]any{},
	})
	require.NoError(t, err, "should not have transport error")

	// Check the content
	require.Greater(t, len(result.Content), 0, "should have content")
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "content should be text")
	assert.NotEmpty(t, textContent.Text, "help text should not be empty")
	assert.Contains(t, textContent.Text, "OQL", "help text should mention OQL")
}

func TestMCP_SDK_NegativeTenantID(t *testing.T) {
	mockClient := &MockPinotClient{}
	server := NewServer(8090, mockClient)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server.mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	require.NoError(t, err)
	defer session.Close()

	// Call oql_query with negative tenant_id
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "oql_query",
		Arguments: map[string]any{
			"tenant_id": -1,
			"query":     "signal=spans | limit 10",
		},
	})
	require.NoError(t, err, "should not have transport error")
	require.True(t, result.IsError, "result should be marked as error")
	require.Greater(t, len(result.Content), 0, "should have error content")

	// Error is returned as TextContent
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "error content should be text")
	assert.Contains(t, strings.ToLower(textContent.Text), "tenant", "error should mention tenant")
}
