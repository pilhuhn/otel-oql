package pinot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a Pinot client
type Client struct {
	brokerURL  string
	httpClient *http.Client
}

// NewClient creates a new Pinot client
func NewClient(brokerURL string) *Client {
	return &Client{
		brokerURL: brokerURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Query executes a SQL query against Pinot
func (c *Client) Query(ctx context.Context, sql string) (*QueryResponse, error) {
	reqBody := map[string]interface{}{
		"sql": sql,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.brokerURL+"/query/sql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query failed with status %d: %s", resp.StatusCode, string(body))
	}

	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &queryResp, nil
}

// Insert inserts data into a Pinot table
func (c *Client) Insert(ctx context.Context, tableName string, records []map[string]interface{}) error {
	jsonData, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("failed to marshal records: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.brokerURL+"/ingest?table="+tableName, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to insert data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("insert failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateTable creates a table in Pinot
func (c *Client) CreateTable(ctx context.Context, schema *TableSchema) error {
	jsonData, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.brokerURL+"/tables", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create table failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// QueryResponse represents a Pinot query response
type QueryResponse struct {
	ResultTable struct {
		DataSchema struct {
			ColumnNames     []string `json:"columnNames"`
			ColumnDataTypes []string `json:"columnDataTypes"`
		} `json:"dataSchema"`
		Rows [][]interface{} `json:"rows"`
	} `json:"resultTable"`
	Exceptions []interface{} `json:"exceptions"`
	NumDocsScanned int64 `json:"numDocsScanned"`
	TotalDocs      int64 `json:"totalDocs"`
	TimeUsedMs     int64 `json:"timeUsedMs"`
}
