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
	"strconv"
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

		// Check for help command
		if strings.ToLower(strings.TrimSpace(query)) == "help" {
			showHelp()
			os.Exit(0)
		}
	} else {
		// Read from stdin (interactive mode)
		query = readQueryInteractive()
	}

	query = strings.TrimSpace(query)
	if query == "" {
		fmt.Fprintf(os.Stderr, "Error: query cannot be empty\n")
		flag.Usage()
		os.Exit(1)
	}

	// Execute query with retry on error
	resp, err := executeQueryWithRetry(*endpoint, *tenantID, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Handle error response (should not happen if executeQueryWithRetry works correctly)
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

// readQueryInteractive reads a query from stdin, handling help command
func readQueryInteractive() string {
	// Check if stdin is a terminal (interactive) or pipe
	stat, _ := os.Stdin.Stat()
	isInteractive := (stat.Mode() & os.ModeCharDevice) != 0

	scanner := bufio.NewScanner(os.Stdin)

	for {
		if isInteractive {
			fmt.Fprintf(os.Stderr, "Enter OQL query (or 'help' for examples, Ctrl+D to exit):\n> ")
		}

		// For interactive: read until Ctrl+D
		// For piped: read one line at a time
		var lines []string

		if !isInteractive {
			// Piped input: read one line, check for help, process immediately
			if scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.ToLower(line) == "help" {
					showHelp()
					// Continue to next line
					continue
				}
				return line
			}
			// EOF or error
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
				os.Exit(1)
			}
			return "" // EOF
		}

		// Interactive mode: read multiple lines until Ctrl+D
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			fmt.Fprintf(os.Stderr, "> ")
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}

		query := strings.Join(lines, " ")
		query = strings.TrimSpace(query)

		// Check for help command in interactive mode
		if strings.ToLower(query) == "help" {
			showHelp()
			scanner = bufio.NewScanner(os.Stdin)
			continue
		}

		return query
	}
}

// showHelp displays OQL syntax help and examples
func showHelp() {
	help := `
OQL (Observability Query Language) Help
======================================

BASIC SYNTAX:
  signal=<type> [operations...]

SIGNAL TYPES:
  - spans, span, s, traces, trace, t   (trace data)
  - metrics, metric, m                 (metrics data)
  - logs, log, l                       (log data)

COMMON OPERATIONS:
  where <condition>              Filter by condition
  limit <n>                      Limit results to n rows
  expand trace                   Get all spans in the same trace
  correlate <signals>            Find related logs/metrics/spans
  get_exemplars()                Extract trace IDs from metrics
  since <duration>               Time range (e.g., "1h", "30m")
  aggregate <func>(<field>)      Aggregations (avg, min, max, sum, count)
  group by <fields>              Group results

CONDITIONS:
  field == "value"               String equality (use == not =)
  field == 123                   Number equality
  field > 100                    Numeric comparison (>, <, >=, <=, !=)
  cond1 and cond2                Logical AND
  cond1 or cond2                 Logical OR

COMMON FIELDS:
  Spans:    trace_id, span_id, name, service_name, duration,
            http_method, http_status_code, error, status_code
  Metrics:  metric_name, value, service_name, timestamp
  Logs:     body, severity_text, service_name, trace_id

EXAMPLES:

  # Find slow spans
  signal=spans where duration > 1000000000 limit 10

  # Find errors from a service
  signal=spans where service_name == "payment" and error == true limit 5

  # Get full trace for a slow request
  signal=spans where duration > 5000000000 limit 1 | expand trace

  # Find logs correlated with error spans
  signal=spans where error == true limit 10 | correlate logs

  # Recent errors (last hour)
  signal=spans where error == true since 1h limit 20

  # Metrics over time
  signal=metrics where metric_name == "http.server.duration" since 30m

  # Aggregate query
  signal=spans group by service_name | aggregate avg(duration)

TIPS:
  - Use == for equality (not =)
  - Strings need quotes: service_name == "payment"
  - Numbers don't: duration > 1000
  - Duration is in nanoseconds (1s = 1000000000ns)
  - Press Ctrl+D to exit
`
	fmt.Println(help)
}

// executeQueryWithRetry executes a query and offers suggestions on errors
func executeQueryWithRetry(endpoint, tenantID, originalQuery string) (*QueryResponse, error) {
	query := originalQuery

	for {
		resp, err := executeQuery(endpoint, tenantID, query)
		if err != nil {
			return nil, err
		}

		// If no error, return the response
		if resp.Error == "" {
			return resp, nil
		}

		// We have an error - try to suggest a fix
		suggestion := suggestQueryFix(query, resp.Error)
		if suggestion == "" {
			// No suggestion available, just return the error
			return resp, nil
		}

		// Print the error and suggestion
		fmt.Fprintf(os.Stderr, "\nError: %s\n\n", resp.Error)
		fmt.Fprintf(os.Stderr, "Suggestion: %s\n\n", suggestion)
		fmt.Fprintf(os.Stderr, "Try this instead? (y/n/edit): ")

		// Read user input
		var choice string
		fmt.Scanln(&choice)
		choice = strings.ToLower(strings.TrimSpace(choice))

		switch choice {
		case "y", "yes":
			// Use the suggested query
			query = suggestion
			fmt.Fprintf(os.Stderr, "\nRetrying with: %s\n\n", query)
			continue
		case "e", "edit":
			// Let user edit the query
			fmt.Fprintf(os.Stderr, "Enter corrected query: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				query = scanner.Text()
				if query == "" {
					return resp, nil // User gave up
				}
				fmt.Fprintf(os.Stderr, "\nRetrying with: %s\n\n", query)
				continue
			}
			return resp, nil
		default:
			// User declined, return the error
			return resp, nil
		}
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
			// Return the response with error for better error handling
			return &errorResp, nil
		}
		// Fallback to raw body as error
		return &QueryResponse{Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))}, nil
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

// suggestQueryFix analyzes an error and suggests a corrected query
func suggestQueryFix(query, errorMsg string) string {
	// Common error patterns and their fixes

	// Pattern 1: "invalid condition: X=Y" (missing quotes or wrong operator)
	// Example: "where service=replicator" -> "where service_name == \"replicator\""
	if strings.Contains(errorMsg, "invalid condition:") {
		// Extract the problematic condition from error message
		parts := strings.Split(errorMsg, "invalid condition: ")
		if len(parts) >= 2 {
			badCondition := strings.TrimSpace(parts[1])

			// Check if it's using = instead of ==
			if strings.Contains(badCondition, "=") && !strings.Contains(badCondition, "==") {
				// Try to fix it
				eqIdx := strings.Index(badCondition, "=")
				if eqIdx > 0 && eqIdx < len(badCondition)-1 {
					left := strings.TrimSpace(badCondition[:eqIdx])
					right := strings.TrimSpace(badCondition[eqIdx+1:])

					// Common field name mappings
					fieldMap := map[string]string{
						"service":  "service_name",
						"name":     "name",
						"trace":    "trace_id",
						"span":     "span_id",
						"status":   "status_code",
						"duration": "duration",
					}

					// Map field name if needed
					if mapped, ok := fieldMap[left]; ok {
						left = mapped
					}

					// Add quotes if right side doesn't have them and isn't a number
					if !strings.HasPrefix(right, "\"") && !strings.HasPrefix(right, "'") {
						if _, err := strconv.Atoi(right); err != nil {
							// Not a number, add quotes
							right = "\"" + right + "\""
						}
					}

					// Build the corrected condition
					fixedCondition := left + " == " + right

					// Replace in the original query
					return strings.Replace(query, badCondition, fixedCondition, 1)
				}
			}
		}
	}

	// Pattern 2: "query must start with 'signal='"
	if strings.Contains(errorMsg, "query must start with 'signal='") {
		// Add signal=spans as default
		return "signal=spans " + query
	}

	// Pattern 3: "invalid signal type: X"
	if strings.Contains(errorMsg, "invalid signal type:") {
		parts := strings.Split(errorMsg, "invalid signal type: ")
		if len(parts) >= 2 {
			invalidSignal := strings.TrimSpace(parts[1])
			// Remove trailing text like " (expected: ..."
			if idx := strings.Index(invalidSignal, " ("); idx > 0 {
				invalidSignal = invalidSignal[:idx]
			}

			// Try to guess the intended signal
			signalMap := map[string]string{
				"span":    "spans",
				"trace":   "traces",
				"metric":  "metrics",
				"log":     "logs",
				"tracing": "traces",
				"logging": "logs",
			}

			if corrected, ok := signalMap[strings.ToLower(invalidSignal)]; ok {
				return strings.Replace(query, "signal="+invalidSignal, "signal="+corrected, 1)
			}
		}
	}

	// Pattern 4: Missing quotes around string values
	// This is already handled by Pattern 1, but we could enhance it

	return "" // No suggestion available
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
