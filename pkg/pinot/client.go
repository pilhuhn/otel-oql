package pinot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.brokerURL+"/sql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Check if it's a connection error
		if isConnectionError(err) {
			return nil, fmt.Errorf("Pinot is not reachable at %s (connection refused). Ensure Pinot is running.", c.brokerURL)
		}
		// Check if it's a timeout
		if isTimeoutError(err) {
			return nil, fmt.Errorf("Pinot query timeout at %s. The query took too long or Pinot is unresponsive.", c.brokerURL)
		}
		// Other network errors
		return nil, fmt.Errorf("failed to connect to Pinot at %s: %w", c.brokerURL, err)
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

// CreateSchema creates a schema in Pinot
func (c *Client) CreateSchema(ctx context.Context, schema interface{}) error {
	jsonData, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.brokerURL+"/schemas", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create schema failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateTable creates a table in Pinot with retry logic for Kafka metadata propagation
func (c *Client) CreateTable(ctx context.Context, tableConfig interface{}) error {
	jsonData, err := json.Marshal(tableConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal table config: %w", err)
	}

	// Retry configuration for Kafka metadata propagation race condition
	maxRetries := 5
	baseDelay := 2 * time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s, 16s, 32s
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			fmt.Printf("⏳ Retrying table creation in %v (attempt %d/%d)...\n", delay, attempt+1, maxRetries+1)
			time.Sleep(delay)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.brokerURL+"/tables", bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to create table: %w", err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			return nil
		}

		// Check if this is a Kafka partition metadata error (transient)
		bodyStr := string(body)
		if resp.StatusCode == http.StatusInternalServerError &&
		   strings.Contains(bodyStr, "Failed to fetch partition information for topic") {
			lastErr = fmt.Errorf("kafka topic metadata not ready (this is normal on first setup)")
			continue // Retry this specific error
		}

		// Non-retryable error
		return fmt.Errorf("create table failed with status %d: %s", resp.StatusCode, bodyStr)
	}

	return fmt.Errorf("create table failed after %d retries: %w", maxRetries+1, lastErr)
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

// isConnectionError checks if the error is a connection refused error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connect: connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable")
}

// isTimeoutError checks if the error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if urlErr, ok := err.(*url.Error); ok {
		return urlErr.Timeout()
	}
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded")
}
