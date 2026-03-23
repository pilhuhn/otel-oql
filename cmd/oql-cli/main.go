package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const version = "1.0.0"

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

func main() {
	// Define flags
	endpoint := flag.String("endpoint", "http://localhost:8080", "OTEL-OQL query API endpoint")
	tenantID := flag.String("tenant-id", "0", "Tenant ID for query isolation")
	verbose := flag.Bool("verbose", false, "Show verbose output including SQL and stats")
	jsonOutput := flag.Bool("json", false, "Output raw JSON response")
	showVersion := flag.Bool("version", false, "Show version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "oql-cli - OTEL-OQL Query Client v%s\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  oql-cli [flags] [query]\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Query from command line\n")
		fmt.Fprintf(os.Stderr, "  oql-cli --tenant-id=0 \"signal=spans limit 10\"\n\n")
		fmt.Fprintf(os.Stderr, "  # Query from stdin\n")
		fmt.Fprintf(os.Stderr, "  echo \"signal=spans where duration > 100\" | oql-cli --tenant-id=0\n\n")
		fmt.Fprintf(os.Stderr, "  # Interactive mode (multi-line input, Ctrl+D to submit)\n")
		fmt.Fprintf(os.Stderr, "  oql-cli --tenant-id=0\n\n")
		fmt.Fprintf(os.Stderr, "  # Verbose output with SQL and stats\n")
		fmt.Fprintf(os.Stderr, "  oql-cli --tenant-id=0 --verbose \"signal=spans limit 5\"\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("oql-cli version %s\n", version)
		os.Exit(0)
	}

	// Get query from args or stdin
	var query string
	if flag.NArg() > 0 {
		// Query provided as command line arguments
		query = strings.Join(flag.Args(), " ")
	} else {
		// Read from stdin
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string

		// Check if stdin is a terminal (interactive) or pipe
		stat, _ := os.Stdin.Stat()
		isInteractive := (stat.Mode() & os.ModeCharDevice) != 0

		if isInteractive {
			fmt.Fprintf(os.Stderr, "Enter OQL query (Ctrl+D to submit):\n> ")
		}

		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			if isInteractive {
				fmt.Fprintf(os.Stderr, "> ")
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}

		query = strings.Join(lines, " ")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		fmt.Fprintf(os.Stderr, "Error: query cannot be empty\n")
		flag.Usage()
		os.Exit(1)
	}

	// Execute query
	resp, err := executeQuery(*endpoint, *tenantID, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Handle error response
	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		os.Exit(1)
	}

	// Output results
	if *jsonOutput {
		// Raw JSON output
		jsonBytes, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonBytes))
	} else {
		// Pretty-printed table output
		printResults(resp, *verbose)
	}
}

// executeQuery sends the query to the OTEL-OQL API
func executeQuery(endpoint, tenantID, query string) (*QueryResponse, error) {
	// Build request
	reqBody := QueryRequest{Query: query}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := endpoint + "/query"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("tenant-id", tenantID)

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to parse error from JSON response
		var errorResp QueryResponse
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != "" {
			return nil, fmt.Errorf("%s", errorResp.Error)
		}
		// Fallback to raw body
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &queryResp, nil
}

// printResults prints query results in a formatted table
func printResults(resp *QueryResponse, verbose bool) {
	if len(resp.Results) == 0 {
		fmt.Println("No results")
		return
	}

	for i, result := range resp.Results {
		if len(resp.Results) > 1 {
			fmt.Printf("\n=== Result Set %d ===\n", i+1)
		}

		if verbose {
			fmt.Printf("\nSQL: %s\n", result.SQL)
			fmt.Printf("Stats: %d/%d docs scanned, %dms\n\n",
				result.Stats.NumDocsScanned,
				result.Stats.TotalDocs,
				result.Stats.TimeUsedMs)
		}

		if len(result.Rows) == 0 {
			fmt.Println("No rows returned")
			continue
		}

		// Print table header
		printTableHeader(result.Columns)

		// Print table rows
		for _, row := range result.Rows {
			printTableRow(result.Columns, row)
		}

		fmt.Printf("\n%d row(s) returned\n", len(result.Rows))
	}
}

// printTableHeader prints the table header
func printTableHeader(columns []string) {
	// Calculate column widths
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
		if widths[i] < 10 {
			widths[i] = 10
		}
	}

	// Print header
	for i, col := range columns {
		fmt.Printf("%-*s", widths[i]+2, col)
	}
	fmt.Println()

	// Print separator
	for _, width := range widths {
		fmt.Print(strings.Repeat("-", width+2))
	}
	fmt.Println()
}

// printTableRow prints a single table row
func printTableRow(columns []string, row []interface{}) {
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
		if widths[i] < 10 {
			widths[i] = 10
		}
	}

	for i, val := range row {
		var strVal string
		if val == nil {
			strVal = "NULL"
		} else {
			strVal = fmt.Sprintf("%v", val)
		}

		// Truncate long values
		if len(strVal) > widths[i] {
			strVal = strVal[:widths[i]-3] + "..."
		}

		fmt.Printf("%-*s", widths[i]+2, strVal)
	}
	fmt.Println()
}
