package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a Clickhouse HTTP client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Clickhouse client connecting to the given URL (e.g. http://localhost:8123)
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// QueryResponse mirrors pinot.QueryResponse for drop-in compatibility with the API server.
type QueryResponse struct {
	ResultTable struct {
		DataSchema struct {
			ColumnNames     []string `json:"columnNames"`
			ColumnDataTypes []string `json:"columnDataTypes"`
		} `json:"dataSchema"`
		Rows [][]interface{} `json:"rows"`
	} `json:"resultTable"`
	Exceptions     []interface{} `json:"exceptions"`
	NumDocsScanned int64         `json:"numDocsScanned"`
	TotalDocs      int64         `json:"totalDocs"`
	TimeUsedMs     int64         `json:"timeUsedMs"`
}

// clickhouseJSONResponse is the raw Clickhouse FORMAT JSON response shape.
type clickhouseJSONResponse struct {
	Meta []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"meta"`
	Data       []map[string]interface{} `json:"data"`
	Rows       int64                    `json:"rows"`
	Statistics struct {
		Elapsed   float64 `json:"elapsed"`
		RowsRead  int64   `json:"rows_read"`
		BytesRead int64   `json:"bytes_read"`
	} `json:"statistics"`
}

// Query executes a SQL query against Clickhouse and returns a Pinot-compatible response.
func (c *Client) Query(ctx context.Context, sql string) (*QueryResponse, error) {
	// Append FORMAT JSON unless the query already specifies a format
	trimmed := strings.TrimRight(sql, " \t\n\r")
	upperTrimmed := strings.ToUpper(trimmed)
	if !strings.Contains(upperTrimmed, "FORMAT ") {
		sql = trimmed + " FORMAT JSON"
	}

	start := time.Now()
	body, status, err := c.post(ctx, "/", sql, nil)
	if err != nil {
		return nil, err
	}
	elapsed := time.Since(start)

	if status != http.StatusOK {
		return nil, fmt.Errorf("query failed with status %d: %s", status, string(body))
	}

	var chResp clickhouseJSONResponse
	if err := json.Unmarshal(body, &chResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Clickhouse response: %w", err)
	}

	return convertResponse(&chResp, elapsed), nil
}

// Insert inserts records into a Clickhouse table using JSONEachRow format.
func (c *Client) Insert(ctx context.Context, table string, records []map[string]interface{}) error {
	if len(records) == 0 {
		return nil
	}

	// Build newline-delimited JSON body
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("failed to encode record: %w", err)
		}
	}

	query := fmt.Sprintf("INSERT INTO %s FORMAT JSONEachRow", table)
	params := url.Values{"query": []string{query}}
	reqURL := c.baseURL + "/?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, &buf)
	if err != nil {
		return fmt.Errorf("failed to create insert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return wrapNetErr("insert", c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("insert into %s failed with status %d: %s", table, resp.StatusCode, string(respBody))
	}
	return nil
}

// CreateTable executes a DDL statement (e.g. CREATE TABLE IF NOT EXISTS ...).
func (c *Client) CreateTable(ctx context.Context, ddl string) error {
	body, status, err := c.post(ctx, "/", ddl, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("DDL failed with status %d: %s", status, string(body))
	}
	return nil
}

// post sends a POST request to path with the given body and optional extra headers.
func (c *Client) post(ctx context.Context, path, body string, extraHeaders map[string]string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, strings.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, wrapNetErr("query", c.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// convertResponse maps a Clickhouse JSON response to the Pinot-compatible QueryResponse.
func convertResponse(ch *clickhouseJSONResponse, elapsed time.Duration) *QueryResponse {
	colNames := make([]string, len(ch.Meta))
	colTypes := make([]string, len(ch.Meta))
	for i, m := range ch.Meta {
		colNames[i] = m.Name
		colTypes[i] = m.Type
	}

	rows := make([][]interface{}, len(ch.Data))
	for i, dataRow := range ch.Data {
		row := make([]interface{}, len(colNames))
		for j, name := range colNames {
			row[j] = dataRow[name]
		}
		rows[i] = row
	}

	var resp QueryResponse
	resp.ResultTable.DataSchema.ColumnNames = colNames
	resp.ResultTable.DataSchema.ColumnDataTypes = colTypes
	resp.ResultTable.Rows = rows
	resp.NumDocsScanned = ch.Statistics.RowsRead
	resp.TotalDocs = ch.Rows
	resp.TimeUsedMs = int64(math.Round(ch.Statistics.Elapsed * 1000))
	_ = elapsed
	return &resp
}

// wrapNetErr wraps a network error with a user-friendly message.
func wrapNetErr(op, baseURL string, err error) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	if strings.Contains(s, "connection refused") || strings.Contains(s, "no such host") || strings.Contains(s, "network is unreachable") {
		return fmt.Errorf("Clickhouse is not reachable at %s (connection refused). Ensure Clickhouse is running.", baseURL)
	}
	if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
		return fmt.Errorf("Clickhouse %s timeout at %s. The operation took too long or Clickhouse is unresponsive.", op, baseURL)
	}
	if strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded") {
		return fmt.Errorf("Clickhouse %s timeout at %s.", op, baseURL)
	}
	return fmt.Errorf("failed to connect to Clickhouse at %s: %w", baseURL, err)
}
