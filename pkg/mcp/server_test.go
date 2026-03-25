package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPinotClient is a mock Pinot client for testing
type mockPinotClient struct {
	queryFunc func(ctx context.Context, sql string) (*pinot.QueryResponse, error)
}

func (m *mockPinotClient) Query(ctx context.Context, sql string) (*pinot.QueryResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql)
	}
	// Default response
	resp := &pinot.QueryResponse{
		NumDocsScanned: 100,
		TotalDocs:      1000,
		TimeUsedMs:     5,
	}
	resp.ResultTable.DataSchema.ColumnNames = []string{"trace_id", "duration", "name"}
	resp.ResultTable.Rows = [][]interface{}{
		{"trace-123", 1500000000, "test-span"},
		{"trace-456", 2000000000, "another-span"},
	}
	return resp, nil
}

func TestMCP_ListTools(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleListTools)))
	defer ts.Close()

	// Make request
	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var toolsResp ToolsListResponse
	err = json.NewDecoder(resp.Body).Decode(&toolsResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, toolsResp.Tools, 2, "Should have exactly 2 tools")

	// Check oql_query tool
	var queryTool *ToolDefinition
	var helpTool *ToolDefinition
	for i := range toolsResp.Tools {
		if toolsResp.Tools[i].Name == "oql_query" {
			queryTool = &toolsResp.Tools[i]
		}
		if toolsResp.Tools[i].Name == "oql_help" {
			helpTool = &toolsResp.Tools[i]
		}
	}

	assert.NotNil(t, queryTool, "oql_query tool should exist")
	assert.NotNil(t, helpTool, "oql_help tool should exist")

	// Verify oql_query schema
	assert.Contains(t, queryTool.Description, "Execute OQL queries")
	assert.NotNil(t, queryTool.InputSchema)
	schema := queryTool.InputSchema
	properties, ok := schema["properties"].(map[string]interface{})
	assert.True(t, ok, "Should have properties")
	assert.Contains(t, properties, "tenant_id")
	assert.Contains(t, properties, "query")

	// Verify oql_help schema
	assert.Contains(t, helpTool.Description, "OQL documentation")
	assert.NotNil(t, helpTool.InputSchema)
}

func TestMCP_OQLQuery_Success(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request
	reqBody := ToolCallRequest{
		Name: "oql_query",
		Arguments: map[string]interface{}{
			"tenant_id": float64(0),
			"query":     "signal=spans | where duration > 500ms | limit 2",
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, callResp.Error, "Should not have error")
	assert.NotEmpty(t, callResp.Content, "Should have content")
	assert.Equal(t, "text", callResp.Content[0].Type)

	// Check that result contains SQL and data
	resultText := callResp.Content[0].Text
	assert.Contains(t, resultText, "SQL:")
	assert.Contains(t, resultText, "SELECT * FROM otel_spans")
	assert.Contains(t, resultText, "WHERE tenant_id = 0")
	assert.Contains(t, resultText, "duration > 500000000") // 500ms in nanoseconds
	assert.Contains(t, resultText, "trace-123")
	assert.Contains(t, resultText, "2 row(s) returned")
}

func TestMCP_OQLQuery_ParseError(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request with invalid query
	reqBody := ToolCallRequest{
		Name: "oql_query",
		Arguments: map[string]interface{}{
			"tenant_id": float64(0),
			"query":     "invalid query syntax",
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotNil(t, callResp.Error, "Should have error")
	assert.Equal(t, "parse_error", callResp.Error.Code)
	assert.Contains(t, callResp.Error.Message, "failed to parse query")
}

func TestMCP_OQLQuery_MalformedDuration(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request with malformed duration
	reqBody := ToolCallRequest{
		Name: "oql_query",
		Arguments: map[string]interface{}{
			"tenant_id": float64(0),
			"query":     "signal=spans | where duration > 5.5.5s",
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotNil(t, callResp.Error, "Should have error")
	assert.Equal(t, "translation_error", callResp.Error.Code)
	assert.Contains(t, callResp.Error.Message, "invalid duration format")
	assert.Contains(t, callResp.Error.Message, "5.5.5s")
}

func TestMCP_OQLQuery_MissingTenantID(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request without tenant_id
	reqBody := ToolCallRequest{
		Name: "oql_query",
		Arguments: map[string]interface{}{
			"query": "signal=spans",
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotNil(t, callResp.Error, "Should have error")
	assert.Equal(t, "invalid_argument", callResp.Error.Code)
	assert.Contains(t, callResp.Error.Message, "tenant_id is required")
}

func TestMCP_OQLQuery_MissingQuery(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request without query
	reqBody := ToolCallRequest{
		Name: "oql_query",
		Arguments: map[string]interface{}{
			"tenant_id": float64(0),
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotNil(t, callResp.Error, "Should have error")
	assert.Equal(t, "invalid_argument", callResp.Error.Code)
	assert.Contains(t, callResp.Error.Message, "query is required")
}

func TestMCP_OQLHelp_AllTopics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires OQL_REFERENCE.md file")
	}

	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request
	reqBody := ToolCallRequest{
		Name: "oql_help",
		Arguments: map[string]interface{}{
			"topic": "all",
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// If file doesn't exist, check error response
	if callResp.Error != nil {
		assert.Contains(t, callResp.Error.Message, "OQL reference")
		t.Skip("OQL_REFERENCE.md not found - skipping help validation")
	}

	// Assertions
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, callResp.Error, "Should not have error")
	assert.NotEmpty(t, callResp.Content, "Should have content")

	helpText := callResp.Content[0].Text
	assert.Contains(t, helpText, "# OQL (Observability Query Language) Reference")
	assert.Contains(t, helpText, "## Core Operations")
	assert.Contains(t, helpText, "## Complete Examples")
	assert.Contains(t, helpText, "## Signal-Specific Fields")
}

func TestMCP_OQLHelp_OperatorsTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires OQL_REFERENCE.md file")
	}

	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request
	reqBody := ToolCallRequest{
		Name: "oql_help",
		Arguments: map[string]interface{}{
			"topic": "operators",
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// If file doesn't exist, skip
	if callResp.Error != nil {
		t.Skip("OQL_REFERENCE.md not found")
	}

	// Assertions
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, callResp.Error, "Should not have error")

	helpText := callResp.Content[0].Text
	assert.Contains(t, helpText, "## Core Operations")
	assert.Contains(t, helpText, "where")
	assert.Contains(t, helpText, "expand trace")
	assert.Contains(t, helpText, "correlate")
	// Should not contain examples section
	assert.NotContains(t, helpText, "## Complete Examples")
}

func TestMCP_OQLHelp_ExamplesTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires OQL_REFERENCE.md file")
	}

	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request
	reqBody := ToolCallRequest{
		Name: "oql_help",
		Arguments: map[string]interface{}{
			"topic": "examples",
		},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// If file doesn't exist, skip
	if callResp.Error != nil {
		t.Skip("OQL_REFERENCE.md not found")
	}

	// Assertions
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, callResp.Error, "Should not have error")

	helpText := callResp.Content[0].Text
	assert.Contains(t, helpText, "## Complete Examples")
	assert.Contains(t, helpText, "Example 1:")
	assert.Contains(t, helpText, "Example 2:")
}

func TestMCP_OQLHelp_NoTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires OQL_REFERENCE.md file")
	}

	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request without topic (defaults to "all")
	reqBody := ToolCallRequest{
		Name:      "oql_help",
		Arguments: map[string]interface{}{},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// If file doesn't exist, skip
	if callResp.Error != nil {
		t.Skip("OQL_REFERENCE.md not found")
	}

	// Assertions - should return full documentation
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, callResp.Error, "Should not have error")

	helpText := callResp.Content[0].Text
	assert.Contains(t, helpText, "# OQL (Observability Query Language) Reference")
}

func TestMCP_UnknownTool(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Prepare request with unknown tool
	reqBody := ToolCallRequest{
		Name:      "unknown_tool",
		Arguments: map[string]interface{}{},
	}
	reqJSON, _ := json.Marshal(reqBody)

	// Make request
	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotNil(t, callResp.Error, "Should have error")
	assert.Equal(t, "tool_not_found", callResp.Error.Code)
	assert.Contains(t, callResp.Error.Message, "unknown tool")
}

func TestMCP_InvalidJSON(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleCallTool)))
	defer ts.Close()

	// Make request with invalid JSON
	resp, err := http.Post(ts.URL, "application/json", strings.NewReader("invalid json"))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Parse response
	var callResp ToolCallResponse
	err = json.NewDecoder(resp.Body).Decode(&callResp)
	require.NoError(t, err)

	// Assertions
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotNil(t, callResp.Error, "Should have error")
	assert.Equal(t, "invalid_request", callResp.Error.Code)
}

func TestMCP_CORS(t *testing.T) {
	// Setup
	mockPinot := &mockPinotClient{}
	server := NewServer(0, mockPinot)

	// Create test server
	ts := httptest.NewServer(server.enableCORS(http.HandlerFunc(server.handleListTools)))
	defer ts.Close()

	// Make OPTIONS request
	req, _ := http.NewRequest("OPTIONS", ts.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Assertions
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type")
}
