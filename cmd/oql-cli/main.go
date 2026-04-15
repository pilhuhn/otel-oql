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
	"time"
)

const version = "1.0.0"

const banner = `
  ___ _____ ___ _        ___   ___  _
 / _ \_   _| __| |      / _ \ / _ \| |
| (_) || | | _|| |_____| (_) | (_) | |___
 \___/ |_| |___|_____|  \___/ \__\_\_____|

  Observability Query Language
`

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

// SessionState holds interactive session state
type SessionState struct {
	lastResponse   *QueryResponse // Last query results
	lastQuery      string          // Last query string
	focusedTraceID string          // Currently focused trace ID
	focusedSpanID  string          // Currently focused span ID (for span-level focus)
	displayRows    [][]interface{} // Displayed rows (sorted/filtered) matching table output
}

func main() {
	// Define flags
	endpoint := flag.String("endpoint", "http://localhost:8080", "OTEL-OQL query API endpoint")
	tenantID := flag.String("tenant-id", "0", "Tenant ID for query isolation")
	verbose := flag.Bool("verbose", false, "Show verbose output including SQL and stats")
	jsonOutput := flag.Bool("json", false, "Output raw JSON response")
	showVersion := flag.Bool("version", false, "Show version and exit")
	allFields := flag.Bool("all-fields", false, "Show all fields (default: only interesting fields)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "oql-cli - OTEL-OQL Query Client v%s\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  oql-cli [flags] [query]\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Query from command line\n")
		fmt.Fprintf(os.Stderr, "  oql-cli -tenant-id=0 \"signal=spans limit 10\"\n\n")
		fmt.Fprintf(os.Stderr, "  # Query from stdin\n")
		fmt.Fprintf(os.Stderr, "  echo \"signal=spans where duration > 100\" | oql-cli -tenant-id=0\n\n")
		fmt.Fprintf(os.Stderr, "  # Interactive mode (multi-line input, Ctrl+D to submit)\n")
		fmt.Fprintf(os.Stderr, "  oql-cli -tenant-id=0\n\n")
		fmt.Fprintf(os.Stderr, "  # Verbose output with SQL and stats\n")
		fmt.Fprintf(os.Stderr, "  oql-cli -tenant-id=0 -verbose \"signal=spans limit 5\"\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		printBanner()
		os.Exit(0)
	}

	// Check if running in interactive mode
	isInteractive := flag.NArg() == 0

	if isInteractive {
		// Check if stdin is actually a terminal
		stat, _ := os.Stdin.Stat()
		isInteractive = (stat.Mode() & os.ModeCharDevice) != 0
	}

	if isInteractive {
		// Interactive REPL mode
		runInteractiveMode(*endpoint, *tenantID, *verbose, *jsonOutput, *allFields)
	} else {
		// Single query mode
		runSingleQuery(*endpoint, *tenantID, *verbose, *jsonOutput, *allFields)
	}
}

// runInteractiveMode runs the CLI in interactive REPL mode
func runInteractiveMode(endpoint, tenantID string, verbose, jsonOutput, allFields bool) {
	printBanner()
	fmt.Fprintf(os.Stderr, "Interactive Shell\n")
	fmt.Fprintf(os.Stderr, "  Type 'help' for query examples\n")
	fmt.Fprintf(os.Stderr, "  Type 'print #N' to show details of row N\n")
	fmt.Fprintf(os.Stderr, "  Type 'focus #N' to focus on a trace or span\n")
	fmt.Fprintf(os.Stderr, "  Type 'unfocus' to return to previous view\n")
	fmt.Fprintf(os.Stderr, "  Type 'list metrics' to see available metrics\n")
	fmt.Fprintf(os.Stderr, "  Type 'undo' to remove last refinement\n")
	fmt.Fprintf(os.Stderr, "  Type 'exit' or Ctrl+D to quit\n\n")

	scanner := bufio.NewScanner(os.Stdin)
	var queryHistory []string // Stack of queries for undo
	session := &SessionState{} // Session state for interactive commands

	for {
		fmt.Fprintf(os.Stderr, "oql> ")

		if !scanner.Scan() {
			// EOF (Ctrl+D)
			fmt.Fprintf(os.Stderr, "\nGoodbye!\n")
			break
		}

		input := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if input == "" {
			continue
		}

		// Check for exit command
		if strings.ToLower(input) == "exit" || strings.ToLower(input) == "quit" {
			fmt.Fprintf(os.Stderr, "Goodbye!\n")
			break
		}

		// Check for help command
		if strings.ToLower(input) == "help" {
			showHelp()
			continue
		}

		// Check for list commands
		if strings.HasPrefix(strings.ToLower(input), "list ") {
			handleListCommand(endpoint, tenantID, input)
			continue
		}

		// Check for print command (with or without arguments)
		lowerInput := strings.ToLower(input)
		if lowerInput == "print" || strings.HasPrefix(lowerInput, "print ") {
			handlePrintCommand(session, input, allFields)
			continue
		}

		// Check for focus command
		if strings.HasPrefix(strings.ToLower(input), "focus ") {
			handleFocusCommand(session, input)
			continue
		}

		// Check for unfocus command
		if strings.ToLower(input) == "unfocus" || strings.ToLower(input) == "clear" {
			if session.focusedTraceID == "" && session.focusedSpanID == "" {
				fmt.Fprintf(os.Stderr, "No trace or span is currently focused.\n\n")
				continue
			}
			if session.focusedSpanID != "" {
				fmt.Fprintf(os.Stderr, "✓ Cleared focus on span: %s\n", session.focusedSpanID)
			} else {
				fmt.Fprintf(os.Stderr, "✓ Cleared focus on trace: %s\n", session.focusedTraceID)
			}
			session.focusedTraceID = ""
			session.focusedSpanID = ""
			// Re-display last query results
			if session.lastResponse != nil {
				printResultsNumbered(session.lastResponse, session.lastQuery, verbose, allFields, session)
			}
			fmt.Fprintf(os.Stderr, "\n")
			continue
		}

		// Check for undo command
		if strings.ToLower(input) == "undo" || strings.ToLower(input) == "back" {
			if len(queryHistory) <= 1 {
				fmt.Fprintf(os.Stderr, "Nothing to undo. Already at the base query.\n\n")
				continue
			}
			// Pop the last query
			queryHistory = queryHistory[:len(queryHistory)-1]
			lastQuery := queryHistory[len(queryHistory)-1]
			fmt.Fprintf(os.Stderr, "Undid last refinement. Current query:\n→ %s\n\n", lastQuery)

			// Re-execute the previous query to show results
			resp, err := executeQuery(endpoint, tenantID, lastQuery)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
				continue
			}
			if resp.Error != "" {
				fmt.Fprintf(os.Stderr, "Error: %s\n\n", resp.Error)
				continue
			}
			if jsonOutput {
				jsonBytes, _ := json.MarshalIndent(resp, "", "  ")
				fmt.Println(string(jsonBytes))
			} else {
				printResults(resp, lastQuery, verbose, allFields)
			}
			session.lastResponse = resp
			session.lastQuery = lastQuery
			fmt.Fprintf(os.Stderr, "\n")
			continue
		}

		// Determine if this is a refinement operation or a new query
		query := input
		isRefinement := isRefinementOperation(input)

		// Auto-expand abbreviations (may convert to a full query if focused)
		expandedQuery, isFullQuery := expandAbbreviations(query, session)
		query = expandedQuery

		// If expandAbbreviations returned a full query (e.g., focused trace expansion),
		// treat it as a new base query, not a refinement
		if isFullQuery {
			isRefinement = false
		}

		// Also check if it's a bare condition (auto-add filter)
		if !isRefinement && !isFullQuery && !strings.HasPrefix(strings.ToLower(query), "signal=") {
			if looksLikeCondition(query) {
				// Auto-add filter prefix
				query = "filter " + query
				isRefinement = true
				fmt.Fprintf(os.Stderr, "(auto-adding 'filter' prefix)\n")
			}
		}

		if isRefinement {
			if len(queryHistory) == 0 {
				fmt.Fprintf(os.Stderr, "Error: No previous query to refine. Start with a signal= query first.\n\n")
				continue
			}
			// Append refinement to last query
			lastQuery := queryHistory[len(queryHistory)-1]
			query = lastQuery + " | " + query
			fmt.Fprintf(os.Stderr, "→ %s\n", query)
		}

		// Execute query (no retry in interactive mode to avoid stdin conflicts)
		resp, err := executeQuery(endpoint, tenantID, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			continue
		}

		// Handle error response
		if resp.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n\n", resp.Error)
			continue
		}

		// Query succeeded - save it to history for potential refinement/undo
		if !isRefinement {
			// New base query - clear history and start fresh
			queryHistory = []string{query}
		} else {
			// Refinement - add to history
			queryHistory = append(queryHistory, query)
		}

		// Save results to session
		session.lastResponse = resp
		session.lastQuery = query

		// Output results
		if jsonOutput {
			jsonBytes, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n\n", err)
				continue
			}
			fmt.Println(string(jsonBytes))
		} else {
			printResultsNumbered(resp, query, verbose, allFields, session)
		}

		fmt.Fprintf(os.Stderr, "\n") // Add spacing between queries
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}

// isRefinementOperation checks if the input is a refinement operation
// rather than a new query
func isRefinementOperation(input string) bool {
	lowerInput := strings.ToLower(strings.TrimSpace(input))

	// Operations that refine existing results
	refinementOps := []string{
		"filter ",
		"limit ",
		"expand ",
		"et ", // Abbreviation for "expand trace"
		"et",  // Standalone "et"
		"correlate ",
		"get_exemplars",
		"switch_context ",
		"extract ",
		"group ",
		"aggregate ",
		"avg(",
		"min(",
		"max(",
		"count(",
		"sum(",
		"since ",
		"between ",
	}

	for _, op := range refinementOps {
		if strings.HasPrefix(lowerInput, op) {
			return true
		}
	}

	return false
}

// looksLikeCondition checks if input looks like a bare condition
// (has comparison operators but no keyword prefix)
func looksLikeCondition(input string) bool {
	trimmed := strings.TrimSpace(input)

	// Check for common operators
	operators := []string{"==", "!=", ">=", "<=", ">", "<", "=", " and ", " or "}

	for _, op := range operators {
		if strings.Contains(trimmed, op) {
			return true
		}
	}

	return false
}

// runSingleQuery runs a single query and exits
func runSingleQuery(endpoint, tenantID string, verbose, jsonOutput, allFields bool) {
	var query string

	if flag.NArg() > 0 {
		// Query from command line args
		query = strings.Join(flag.Args(), " ")

		// Check for help command
		if strings.ToLower(strings.TrimSpace(query)) == "help" {
			showHelp()
			os.Exit(0)
		}

		// Check for list commands
		if strings.HasPrefix(strings.ToLower(query), "list ") {
			handleListCommand(endpoint, tenantID, query)
			os.Exit(0)
		}
	} else {
		// Read from stdin (piped input)
		query = readQueryInteractive()
	}

	query = strings.TrimSpace(query)
	if query == "" {
		fmt.Fprintf(os.Stderr, "Error: query cannot be empty\n")
		flag.Usage()
		os.Exit(1)
	}

	// Check for list commands from stdin as well
	if strings.HasPrefix(strings.ToLower(query), "list ") {
		handleListCommand(endpoint, tenantID, query)
		os.Exit(0)
	}

	// Execute query with retry on error
	resp, err := executeQueryWithRetry(endpoint, tenantID, query)
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
	if jsonOutput {
		jsonBytes, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonBytes))
	} else {
		printResults(resp, query, verbose, allFields)
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

// printBanner prints the ASCII art banner and version
func printBanner() {
	fmt.Fprintf(os.Stderr, "%s", banner)
	fmt.Fprintf(os.Stderr, "  Version %s\n\n", version)
}

// showHelp displays OQL syntax help and examples
func showHelp() {
	help := `
OQL (Observability Query Language) Help
======================================

DISCOVERY COMMANDS:
  list metrics                   List all available metrics
  list labels                    List all available labels
  list values <label>            List values for a specific label

INTERACTIVE COMMANDS:
  print #N                       Show full details of row N from last results
  print <traceid>                Show rows matching trace ID (supports prefix)
  print                          Show details of focused trace/span (when focused)
  focus #N                       Set focus to trace/span in row N
  focus <traceid>                Set context trace for subsequent operations
  unfocus                        Clear focus and return to list view
  undo                           Remove last refinement

FOCUS MODES:
  Trace-level focus              From trace list: 'focus #N' focuses on entire trace
                                 Commands: 'print', 'et' (expand trace)
  Span-level focus               From expanded trace: 'focus #N' focuses on specific span
                                 Commands: 'print', 'correlate logs' (logs for this span)

BASIC SYNTAX:
  signal=<type> [operations...]

SIGNAL TYPES:
  - spans, span, s, traces, trace, t   (trace data)
  - metrics, metric, m                 (metrics data)
  - logs, log, l                       (log data)

COMMON OPERATIONS:
  where <condition>              Filter by condition
  limit <n>                      Limit results to n rows
  expand trace                   Get all spans in the same trace (or just: expand, et)
  correlate <signals>            Find related logs/metrics/spans
  get_exemplars()                Extract trace IDs from metrics
  since <duration>               Time range (e.g., "1h", "30m")
  aggregate <func>(<field>)      Aggregations (avg, min, max, sum, count)
  group by <fields>              Group results

CONDITIONS:
  field = "value" or ==          String equality (both = and == work)
  field = 123                    Number equality
  field > 100                    Numeric comparison (>, <, >=, <=, !=)
  cond1 and cond2                Logical AND
  cond1 or cond2                 Logical OR

COMMON FIELDS:
  Spans:    trace_id, span_id, name, service_name, duration,
            http_method, http_status_code, error, status_code
  Metrics:  metric_name, value, service_name, timestamp
  Logs:     body, severity_text, service_name, trace_id

EXAMPLES:

  # Find slow spans (using time units)
  signal=spans where duration > 1s limit 10

  # Find errors from a service
  signal=spans where service_name = "payment" and error = true limit 5

  # Get full trace for a slow request
  signal=spans where duration > 5s limit 1 | expand trace

  # Find logs correlated with error spans
  signal=spans where error == true limit 10 | correlate logs

  # Recent errors (last hour)
  signal=spans where error == true since 1h limit 20

  # Metrics over time
  signal=metrics where metric_name == "http.server.duration" since 30m

  # Aggregate query
  signal=spans group by service_name | aggregate avg(duration)

  # Interactive workflow - investigate a specific trace
  signal=t | filter name="GET /tea"  # Get list of traces
  focus #3                            # Focus on trace from row 3
  print                               # Show details (no args needed when focused!)
  et                                  # Expand the focused trace (super short!)
  focus #5                            # Focus on span #5 (error span!)
  print                               # Show span details
  correlate logs                      # Get logs for THIS SPECIFIC SPAN
  unfocus                             # Return to trace view

TIPS:
  - Both = and == work for equality
  - Strings need quotes: service_name = "payment"
  - Time units: duration > 5s, duration < 100ms (auto-converts to ns)
  - Supported units: s (seconds), ms (milliseconds), us (microseconds), ns (nanoseconds)
  - Abbreviations: 'expand' or 'et' instead of 'expand trace'
  - Interactive: Use 'print #N' to inspect rows, 'focus <traceid>' to set context
  - In REPL: type 'undo' to remove last refinement
  - Type 'exit' or Ctrl+D to quit interactive mode
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

// handleListCommand handles "list" commands for discovering metrics and labels
func handleListCommand(endpoint, tenantID, input string) {
	parts := strings.Fields(input)
	if len(parts) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: list <what>\n")
		fmt.Fprintf(os.Stderr, "  list metrics              - List all available metrics\n")
		fmt.Fprintf(os.Stderr, "  list labels               - List all available labels\n")
		fmt.Fprintf(os.Stderr, "  list values <label>       - List values for a specific label\n")
		return
	}

	command := strings.ToLower(parts[1])

	switch command {
	case "metrics":
		// List metric names (special case of label values for __name__)
		values, err := fetchLabelValues(endpoint, tenantID, "__name__", 1000)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Println("\nAvailable metrics:")
		for _, value := range values {
			fmt.Printf("  %s\n", value)
		}
		fmt.Printf("\nTotal: %d metrics\n", len(values))

	case "labels":
		// List all labels
		labels, err := fetchLabels(endpoint, tenantID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Println("\nAvailable labels:")
		for _, label := range labels {
			fmt.Printf("  %s\n", label)
		}
		fmt.Printf("\nTotal: %d labels\n", len(labels))

	case "values":
		// List values for a specific label
		if len(parts) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: list values <label>\n")
			fmt.Fprintf(os.Stderr, "Example: list values service_name\n")
			return
		}
		labelName := parts[2]
		values, err := fetchLabelValues(endpoint, tenantID, labelName, 1000)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Printf("\nValues for label '%s':\n", labelName)
		for _, value := range values {
			fmt.Printf("  %s\n", value)
		}
		fmt.Printf("\nTotal: %d values\n", len(values))

	default:
		fmt.Fprintf(os.Stderr, "Unknown list command: %s\n", command)
		fmt.Fprintf(os.Stderr, "Available commands: metrics, labels, values\n")
	}
}

// PrometheusLabelsResponse represents the response from /api/v1/labels
type PrometheusLabelsResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
	Error  string   `json:"error,omitempty"`
}

// fetchLabels fetches the list of available labels from the API
func fetchLabels(endpoint, tenantID string) ([]string, error) {
	url := endpoint + "/api/v1/labels"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Tenant-ID", tenantID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var labelsResp PrometheusLabelsResponse
	if err := json.Unmarshal(body, &labelsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if labelsResp.Error != "" {
		return nil, fmt.Errorf("API error: %s", labelsResp.Error)
	}

	return labelsResp.Data, nil
}

// fetchLabelValues fetches the list of values for a specific label
func fetchLabelValues(endpoint, tenantID, labelName string, limit int) ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/label/%s/values?limit=%d", endpoint, labelName, limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Tenant-ID", tenantID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var valuesResp PrometheusLabelsResponse
	if err := json.Unmarshal(body, &valuesResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if valuesResp.Error != "" {
		return nil, fmt.Errorf("API error: %s", valuesResp.Error)
	}

	return valuesResp.Data, nil
}

// handlePrintCommand handles "print #N" or "print <traceid>" or "print" (when focused) commands
func handlePrintCommand(session *SessionState, input string, allFields bool) {
	if session.lastResponse == nil || len(session.lastResponse.Results) == 0 {
		fmt.Fprintf(os.Stderr, "No previous results. Run a query first.\n\n")
		return
	}

	parts := strings.Fields(input)

	// Handle "print" without arguments when focused
	if len(parts) == 1 {
		if session.focusedSpanID != "" {
			// Span-level focus: print the specific span
			printRowBySpanID(session.lastResponse, session.focusedSpanID, allFields)
			return
		} else if session.focusedTraceID != "" {
			// Trace-level focus: print the trace
			printRowByTraceID(session.lastResponse, session.focusedTraceID, allFields)
			return
		} else {
			fmt.Fprintf(os.Stderr, "Usage: print #N (row number) or print <traceid>\n")
			fmt.Fprintf(os.Stderr, "Example: print #3\n")
			fmt.Fprintf(os.Stderr, "Example: print 0c7c63b1\n")
			fmt.Fprintf(os.Stderr, "Tip: Use 'focus #N' to focus on a trace/span, then 'print' to see its details\n\n")
			return
		}
	}

	arg := parts[1]

	// Check if it's a row number (#N)
	if strings.HasPrefix(arg, "#") {
		rowNum, err := strconv.Atoi(arg[1:])
		if err != nil || rowNum < 1 {
			fmt.Fprintf(os.Stderr, "Invalid row number: %s\n\n", arg)
			return
		}
		printRowDetails(session.lastResponse, rowNum-1, allFields, session.displayRows)
		return
	}

	// Otherwise treat as trace ID (or prefix)
	printRowByTraceID(session.lastResponse, arg, allFields)
}

// handleFocusCommand handles "focus <traceid>" or "focus on trace <traceid>" or "focus #N" commands
func handleFocusCommand(session *SessionState, input string) {
	if session.lastResponse == nil || len(session.lastResponse.Results) == 0 {
		fmt.Fprintf(os.Stderr, "No previous results. Run a query first.\n\n")
		return
	}

	parts := strings.Fields(input)

	// Handle "focus on trace <id>" or "focus <id>" or "focus #N"
	var arg string
	if len(parts) >= 4 && strings.ToLower(parts[1]) == "on" && strings.ToLower(parts[2]) == "trace" {
		arg = parts[3]
	} else if len(parts) >= 2 {
		arg = parts[1]
	} else {
		fmt.Fprintf(os.Stderr, "Usage: focus #N (row number) or focus <traceid>\n")
		fmt.Fprintf(os.Stderr, "Example: focus #3\n")
		fmt.Fprintf(os.Stderr, "Example: focus 0c7c63b1\n")
		fmt.Fprintf(os.Stderr, "Example: focus on trace 0c7c63b13f9bcbfc\n\n")
		return
	}

	var traceID string
	var spanID string

	// Check if it's a row number (#N)
	if strings.HasPrefix(arg, "#") {
		rowNum, err := strconv.Atoi(arg[1:])
		if err != nil || rowNum < 1 {
			fmt.Fprintf(os.Stderr, "Invalid row number: %s\n\n", arg)
			return
		}

		// Extract trace_id and span_id from row
		result := session.lastResponse.Results[0]
		if rowNum-1 >= len(result.Rows) {
			fmt.Fprintf(os.Stderr, "Row #%d not found. Results contain %d rows.\n\n", rowNum, len(result.Rows))
			return
		}

		// Find trace_id and span_id column indices
		traceIDIdx := -1
		spanIDIdx := -1
		for i, col := range result.Columns {
			if col == "trace_id" {
				traceIDIdx = i
			} else if col == "span_id" {
				spanIDIdx = i
			}
		}

		if traceIDIdx == -1 {
			fmt.Fprintf(os.Stderr, "No trace_id column in results.\n\n")
			return
		}

		row := result.Rows[rowNum-1]
		if traceIDIdx < len(row) && row[traceIDIdx] != nil {
			traceID = fmt.Sprintf("%v", row[traceIDIdx])
		} else {
			fmt.Fprintf(os.Stderr, "Row #%d has no trace_id.\n\n", rowNum)
			return
		}

		// If span_id column exists, extract it (span-level focus)
		if spanIDIdx != -1 && spanIDIdx < len(row) && row[spanIDIdx] != nil {
			spanID = fmt.Sprintf("%v", row[spanIDIdx])
		}
	} else {
		// It's a trace ID (or prefix)
		traceID = arg
	}

	// Determine focus type based on context
	isExpandTraceView := strings.Contains(strings.ToLower(session.lastQuery), "expand trace")

	if isExpandTraceView && spanID != "" {
		// Span-level focus (when in expand trace view)
		session.focusedTraceID = traceID
		session.focusedSpanID = spanID
		fmt.Fprintf(os.Stderr, "✓ Focused on span: %s (trace: %s)\n", spanID, traceID)
		fmt.Fprintf(os.Stderr, "  Commands: 'print' (span details), 'correlate logs' (logs for this span), 'unfocus'\n\n")
	} else {
		// Trace-level focus
		session.focusedTraceID = traceID
		session.focusedSpanID = "" // Clear span focus
		fmt.Fprintf(os.Stderr, "✓ Focused on trace: %s\n", traceID)
		fmt.Fprintf(os.Stderr, "  Commands: 'print' (details), 'et' (expand), 'unfocus' (return to list)\n\n")
	}
}

// expandAbbreviations expands shorthand commands
// Returns (expandedQuery, isFullQuery) where isFullQuery=true means it generated a complete signal=... query
func expandAbbreviations(query string, session *SessionState) (string, bool) {
	trimmed := strings.TrimSpace(query)
	lowerQuery := strings.ToLower(trimmed)

	// Expand "et" to "expand trace"
	if lowerQuery == "et" {
		query = "expand trace"
	} else if strings.HasPrefix(lowerQuery, "et ") {
		query = "expand trace" + trimmed[2:]
	}

	// Expand "cl" to "correlate logs"
	if lowerQuery == "cl" {
		query = "correlate logs"
	} else if strings.HasPrefix(lowerQuery, "cl ") {
		query = "correlate logs" + trimmed[2:]
	}

	// Expand standalone "expand" to "expand trace"
	if lowerQuery == "expand" {
		query = "expand trace"
	} else if strings.HasPrefix(lowerQuery, "expand ") && !strings.HasPrefix(lowerQuery, "expand trace") {
		// It's "expand" followed by something that's not "trace"
		// This is likely a typo, so just convert "expand" -> "expand trace"
		parts := strings.Fields(trimmed)
		if len(parts) == 1 {
			query = "expand trace"
		}
	}

	// Update lowerQuery after expansions
	lowerQuery = strings.ToLower(query)

	// If focused on a span and user types "correlate logs", auto-inject span_id filter
	if session.focusedSpanID != "" {
		if lowerQuery == "correlate logs" || lowerQuery == "correlate log" {
			// Find logs for this specific span
			query = fmt.Sprintf("signal=logs | where span_id == \"%s\"", session.focusedSpanID)
			fmt.Fprintf(os.Stderr, "(finding logs for focused span: %s)\n", session.focusedSpanID)
			return query, true // This is a full query
		}
	}

	// If focused on a trace and user types "expand" or "expand trace" without specifying trace_id,
	// auto-inject the focused trace as a FULL NEW QUERY
	if session.focusedTraceID != "" && session.focusedSpanID == "" {
		if lowerQuery == "expand trace" || lowerQuery == "expand" || lowerQuery == "et" {
			// Auto-inject focused trace as a complete new query
			query = fmt.Sprintf("signal=spans | where trace_id == \"%s\" | expand trace", session.focusedTraceID)
			fmt.Fprintf(os.Stderr, "(expanding focused trace: %s)\n", session.focusedTraceID)
			return query, true // This is a full query, not a refinement
		}
	}

	return query, false // Normal expansion, not a full query
}

// printRowDetails prints full details of a specific row
func printRowDetails(resp *QueryResponse, rowIdx int, allFields bool, displayRows [][]interface{}) {
	if len(resp.Results) == 0 {
		fmt.Fprintf(os.Stderr, "No results to display.\n\n")
		return
	}

	// For now, print from first result set
	result := resp.Results[0]

	// Use displayRows if provided (for sorted/filtered views), otherwise use result.Rows
	rows := result.Rows
	if displayRows != nil {
		rows = displayRows
	}

	if rowIdx < 0 || rowIdx >= len(rows) {
		fmt.Fprintf(os.Stderr, "Row #%d not found. Results contain %d rows.\n\n", rowIdx+1, len(rows))
		return
	}

	row := rows[rowIdx]

	fmt.Printf("\n=== Row #%d Details ===\n\n", rowIdx+1)

	// Find max column name length for alignment
	maxLen := 0
	for _, col := range result.Columns {
		if len(col) > maxLen {
			maxLen = len(col)
		}
	}

	// Print all fields (with FULL values, not truncated)
	for i, col := range result.Columns {
		if i < len(row) {
			value := row[i]

			// Skip nil values or string "null" unless --all-fields
			if !allFields {
				if value == nil {
					continue
				}
				// Also skip string "null" (some Pinot responses use this)
				if strVal, ok := value.(string); ok && strVal == "null" {
					continue
				}
			}

			// For details view, show raw value without table truncation
			var formattedValue string
			if value == nil {
				formattedValue = "null"
			} else {
				// Apply formatting but not truncation
				switch col {
				case "duration":
					if numVal, ok := toInt64(value); ok {
						formattedValue = formatDuration(numVal)
					} else {
						formattedValue = fmt.Sprintf("%v", value)
					}
				case "timestamp", "start_time", "end_time":
					if numVal, ok := toInt64(value); ok {
						formattedValue = formatTimestamp(numVal)
					} else {
						formattedValue = fmt.Sprintf("%v", value)
					}
				case "http_status_code", "status_code":
					if numVal, ok := toInt64(value); ok && numVal == -1 {
						formattedValue = ""
					} else {
						formattedValue = fmt.Sprintf("%v", value)
					}
				case "error":
					if boolVal, ok := value.(bool); ok {
						if boolVal {
							formattedValue = "true"
						} else {
							formattedValue = ""
						}
					} else {
						formattedValue = fmt.Sprintf("%v", value)
					}
				default:
					// Show full value (no truncation for trace_id, span_id, etc.)
					formattedValue = fmt.Sprintf("%v", value)
				}
			}

			if formattedValue == "" && !allFields {
				continue // Skip empty values unless --all-fields
			}
			fmt.Printf("%-*s: %s\n", maxLen, col, formattedValue)
		}
	}

	fmt.Println()
}

// printRowByTraceID finds and prints row(s) matching a trace ID (or prefix)
func printRowByTraceID(resp *QueryResponse, traceIDPrefix string, allFields bool) {
	if len(resp.Results) == 0 {
		fmt.Fprintf(os.Stderr, "No results to display.\n\n")
		return
	}

	result := resp.Results[0]

	// Find trace_id column
	traceIDIdx := -1
	for i, col := range result.Columns {
		if col == "trace_id" {
			traceIDIdx = i
			break
		}
	}

	if traceIDIdx == -1 {
		fmt.Fprintf(os.Stderr, "No trace_id column in results.\n\n")
		return
	}

	// Find matching rows
	matchedRows := make([]int, 0)
	for rowIdx, row := range result.Rows {
		if traceIDIdx < len(row) && row[traceIDIdx] != nil {
			traceID := fmt.Sprintf("%v", row[traceIDIdx])
			if strings.HasPrefix(traceID, traceIDPrefix) {
				matchedRows = append(matchedRows, rowIdx)
			}
		}
	}

	if len(matchedRows) == 0 {
		fmt.Fprintf(os.Stderr, "No rows found with trace_id starting with: %s\n\n", traceIDPrefix)
		return
	}

	// Print all matching rows
	for _, rowIdx := range matchedRows {
		printRowDetails(resp, rowIdx, allFields, nil)
	}
}

// printRowBySpanID finds and prints row(s) matching a span ID (or prefix)
func printRowBySpanID(resp *QueryResponse, spanIDPrefix string, allFields bool) {
	if len(resp.Results) == 0 {
		fmt.Fprintf(os.Stderr, "No results to display.\n\n")
		return
	}

	result := resp.Results[0]

	// Find span_id column
	spanIDIdx := -1
	for i, col := range result.Columns {
		if col == "span_id" {
			spanIDIdx = i
			break
		}
	}

	if spanIDIdx == -1 {
		fmt.Fprintf(os.Stderr, "No span_id column in results.\n\n")
		return
	}

	// Find matching rows
	matchedRows := make([]int, 0)
	for rowIdx, row := range result.Rows {
		if spanIDIdx < len(row) && row[spanIDIdx] != nil {
			spanID := fmt.Sprintf("%v", row[spanIDIdx])
			if strings.HasPrefix(spanID, spanIDPrefix) {
				matchedRows = append(matchedRows, rowIdx)
			}
		}
	}

	if len(matchedRows) == 0 {
		fmt.Fprintf(os.Stderr, "No rows found with span_id starting with: %s\n\n", spanIDPrefix)
		return
	}

	// Print all matching rows
	for _, rowIdx := range matchedRows {
		printRowDetails(resp, rowIdx, allFields, nil)
	}
}

// printResultsNumbered prints query results with row numbers
func printResultsNumbered(resp *QueryResponse, query string, verbose, allFields bool, session *SessionState) {
	if len(resp.Results) == 0 {
		fmt.Println("No results")
		return
	}

	// Detect query context
	isCorrelateQuery := strings.Contains(strings.ToLower(query), "correlate")
	isExpandTrace := strings.Contains(strings.ToLower(query), "expand trace")
	signalType := detectSignalType(query)

	for i, result := range resp.Results {
		if len(resp.Results) > 1 {
			if isCorrelateQuery {
				// For correlate queries, show signal type header
				fmt.Printf("\n=== %s ===\n", detectSignalTypeFromSQL(result.SQL))
			} else {
				fmt.Printf("\n=== Result Set %d ===\n", i+1)
			}
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

		// Sort rows by trace hierarchy if this is an expand trace query
		displayRows := result.Rows
		if isExpandTrace {
			displayRows = sortRowsByTraceHierarchy(result.Columns, result.Rows)
		} else if isCorrelateQuery {
			// For correlate queries, detect signal type and sort logs by timestamp
			sqlType := detectSignalTypeFromSQL(result.SQL)
			if strings.ToLower(sqlType) == "logs" {
				displayRows = sortRowsByTimestamp(result.Columns, result.Rows)
			}
		}

		// Store displayRows in session for print command to use
		if session != nil {
			session.displayRows = displayRows
		}

		// Filter columns unless --all-fields is specified
		displayColumns := result.Columns
		columnIndices := make([]int, len(result.Columns))
		for i := range columnIndices {
			columnIndices[i] = i
		}
		var hiddenFilters map[string]string

		if !allFields {
			// For correlate queries, detect signal type per result set from SQL
			resultSignalType := signalType
			if isCorrelateQuery {
				sqlType := detectSignalTypeFromSQL(result.SQL)
				resultSignalType = strings.ToLower(sqlType)
			}
			displayColumns, columnIndices, hiddenFilters = filterColumns(result.Columns, resultSignalType, displayRows, query)
		}

		if len(displayColumns) == 0 {
			fmt.Println("No displayable columns")
			continue
		}

		// Print filter summary if we hid any filtered columns
		if len(hiddenFilters) > 0 {
			filterParts := make([]string, 0)
			for field, value := range hiddenFilters {
				filterParts = append(filterParts, fmt.Sprintf("%s=%s", field, value))
			}
			fmt.Printf("[Filtered by: %s]\n", strings.Join(filterParts, ", "))
		}

		// Print table header with row number column
		printTableHeaderNumbered(displayColumns)

		// Build indentation map for expand trace queries (after sorting!)
		indentMap := make(map[int]int) // row index -> indent level
		if isExpandTrace {
			indentMap = buildTraceIndentation(result.Columns, displayRows)
		}

		// Print table rows with row numbers
		for rowIdx, row := range displayRows {
			indent := indentMap[rowIdx]
			printFormattedRowNumbered(rowIdx+1, displayColumns, row, columnIndices, indent)
		}

		fmt.Printf("\n%d row(s) returned\n", len(displayRows))
		fmt.Fprintf(os.Stderr, "Tip: Use 'print #N' to see full details (IDs shown truncated)\n")
	}
}

// printTableHeaderNumbered prints table header with row number column
func printTableHeaderNumbered(columns []string) {
	widths := getColumnWidths(columns)

	// Print #
	fmt.Printf("%-4s", "#")

	// Print header with display names
	for i, col := range columns {
		displayName := getDisplayColumnName(col)
		fmt.Printf("%-*s", widths[i]+2, displayName)
	}
	fmt.Println()

	// Print separator
	fmt.Print("----") // For row number
	for _, width := range widths {
		fmt.Print(strings.Repeat("-", width+2))
	}
	fmt.Println()
}

// printFormattedRowNumbered prints a row with row number prefix
func printFormattedRowNumbered(rowNum int, columns []string, row []interface{}, indices []int, indent int) {
	widths := getColumnWidths(columns)

	// Print row number
	fmt.Printf("%-4d", rowNum)

	for i, idx := range indices {
		if idx >= len(row) {
			fmt.Printf("%-*s", widths[i]+2, "N/A")
			continue
		}

		// Format the value
		strVal := formatValue(columns[i], row[idx])

		// Add indentation to the "name" column for trace hierarchy
		if columns[i] == "name" && indent > 0 {
			// Use tree characters for better visibility
			indentStr := strings.Repeat("  ", indent-1) + "└─"
			strVal = indentStr + strVal
		}

		// Truncate long values
		if len(strVal) > widths[i] {
			strVal = strVal[:widths[i]-3] + "..."
		}

		fmt.Printf("%-*s", widths[i]+2, strVal)
	}
	fmt.Println()
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

// detectSignalType detects the signal type from the query
func detectSignalType(query string) string {
	lowerQuery := strings.ToLower(query)

	if strings.Contains(lowerQuery, "signal=spans") || strings.Contains(lowerQuery, "signal=span") ||
		strings.Contains(lowerQuery, "signal=traces") || strings.Contains(lowerQuery, "signal=trace") ||
		strings.Contains(lowerQuery, "signal=s") || strings.Contains(lowerQuery, "signal=t") {
		return "spans"
	}
	if strings.Contains(lowerQuery, "signal=metrics") || strings.Contains(lowerQuery, "signal=metric") ||
		strings.Contains(lowerQuery, "signal=m") {
		return "metrics"
	}
	if strings.Contains(lowerQuery, "signal=logs") || strings.Contains(lowerQuery, "signal=log") ||
		strings.Contains(lowerQuery, "signal=l") {
		return "logs"
	}
	return "spans" // default
}

// detectSignalTypeFromSQL detects signal type from SQL table name
func detectSignalTypeFromSQL(sql string) string {
	lowerSQL := strings.ToLower(sql)
	if strings.Contains(lowerSQL, "otel_spans") {
		return "Spans"
	}
	if strings.Contains(lowerSQL, "otel_metrics") {
		return "Metrics"
	}
	if strings.Contains(lowerSQL, "otel_logs") {
		return "Logs"
	}
	return "Results"
}

// getPotentiallyInterestingColumns returns a broader set of columns to consider based on signal type and query
func getPotentiallyInterestingColumns(signalType string, query string) []string {
	isExpandTrace := strings.Contains(strings.ToLower(query), "expand trace")

	switch signalType {
	case "spans":
		columns := []string{
			"duration", "service_name", "error",
			"http_method", "http_route", "http_status_code",
			"db_system", "db_statement",
			"messaging_system", "messaging_destination",
			"rpc_service", "rpc_method",
			"trace_id",
		}
		// Add span hierarchy columns if doing expand trace
		if isExpandTrace {
			columns = append(columns, "span_id", "parent_span_id")
		}
		// Note: "name" is last to make indentation more visible in expand trace
		columns = append(columns, "name")
		return columns

	case "metrics":
		return []string{
			"metric_name", "value", "service_name", "timestamp",
			"exemplar_trace_id", "job", "instance", "environment",
		}
	case "logs":
		return []string{
			"timestamp", "severity_text", "service_name", "span_id",
			"trace_id", "log_level", "log_source", "body",
		}
	default:
		return []string{
			"duration", "service_name", "error", "http_status_code",
			"trace_id", "name",
		}
	}
}

// extractFilteredFields extracts field names from where/filter conditions in the query
func extractFilteredFields(query string) map[string]string {
	filtered := make(map[string]string)

	// Split query by pipes to process each operation
	parts := strings.Split(query, "|")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		lowerPart := strings.ToLower(part)

		// Check if this part contains a where or filter clause
		var conditionPart string
		if idx := strings.Index(lowerPart, " where "); idx != -1 {
			conditionPart = part[idx+7:] // Skip " where "
		} else if strings.HasPrefix(lowerPart, "where ") {
			conditionPart = part[6:] // Skip "where "
		} else if strings.HasPrefix(lowerPart, "filter ") {
			conditionPart = part[7:] // Skip "filter "
		} else {
			continue
		}

		conditionPart = strings.TrimSpace(conditionPart)

		// Split by "and" and "or" to handle multiple conditions
		conditions := []string{conditionPart}
		for _, separator := range []string{" and ", " or "} {
			newConditions := make([]string, 0)
			for _, cond := range conditions {
				newConditions = append(newConditions, strings.Split(cond, separator)...)
			}
			conditions = newConditions
		}

		// Process each condition
		for _, condition := range conditions {
			condition = strings.TrimSpace(condition)

			// Try to extract field=value or field==value (equality only)
			var field, value string
			if strings.Contains(condition, "==") {
				parts := strings.SplitN(condition, "==", 2)
				if len(parts) == 2 {
					field = strings.TrimSpace(parts[0])
					value = strings.TrimSpace(parts[1])
				}
			} else if strings.Contains(condition, "=") && !strings.Contains(condition, ">") && !strings.Contains(condition, "<") && !strings.Contains(condition, "!") {
				parts := strings.SplitN(condition, "=", 2)
				if len(parts) == 2 {
					field = strings.TrimSpace(parts[0])
					value = strings.TrimSpace(parts[1])
				}
			}

			if field != "" && value != "" {
				// Remove quotes from value and any trailing content after closing quote
				if strings.HasPrefix(value, "\"") {
					// Find the closing quote
					if endIdx := strings.Index(value[1:], "\""); endIdx != -1 {
						value = value[1 : endIdx+1]
					} else {
						value = strings.Trim(value, "\"'")
					}
				} else if strings.HasPrefix(value, "'") {
					// Find the closing quote
					if endIdx := strings.Index(value[1:], "'"); endIdx != -1 {
						value = value[1 : endIdx+1]
					} else {
						value = strings.Trim(value, "\"'")
					}
				} else {
					// No quotes - take everything up to first space
					if spaceIdx := strings.Index(value, " "); spaceIdx != -1 {
						value = value[:spaceIdx]
					}
				}
				filtered[field] = value
			}
		}
	}

	return filtered
}

// filterColumns returns the columns to display based on signal type and actual data content
func filterColumns(allColumns []string, signalType string, rows [][]interface{}, query string) ([]string, []int, map[string]string) {
	isExpandTrace := strings.Contains(strings.ToLower(query), "expand trace")

	// Get potentially interesting columns for this signal type
	potentialColumns := getPotentiallyInterestingColumns(signalType, query)

	// Build map for quick lookup of column indices
	colToIndex := make(map[string]int)
	for i, col := range allColumns {
		colToIndex[col] = i
	}

	// Extract fields that were filtered on with equality
	filteredFields := extractFilteredFields(query)

	// Analyze each column in the order specified by potentialColumns
	displayColumns := make([]string, 0)
	columnIndices := make([]int, 0)
	hiddenFilters := make(map[string]string) // Columns hidden due to filtering

	for _, col := range potentialColumns {
		// Check if this column exists in the actual results
		i, exists := colToIndex[col]
		if !exists {
			continue
		}

		// Check if this column has any non-null/non-empty values
		hasData := false
		var firstNonNullValue interface{}
		allIdentical := true
		nonNullCount := 0

		for _, row := range rows {
			if i < len(row) && row[i] != nil {
				val := fmt.Sprintf("%v", row[i])
				if val != "" && val != "null" {
					hasData = true
					nonNullCount++
					if firstNonNullValue == nil {
						firstNonNullValue = row[i]
					} else if fmt.Sprintf("%v", firstNonNullValue) != val {
						allIdentical = false
					}
				}
			}
		}

		// Skip columns with no data
		if !hasData {
			continue
		}

		// For expand trace, hide columns that are mostly empty (less than 30% populated)
		if isExpandTrace && len(rows) > 0 {
			percentPopulated := float64(nonNullCount) / float64(len(rows)) * 100
			if percentPopulated < 30 {
				continue
			}
		}

		// If user filtered on this field with equality and all values are identical, hide it
		if allIdentical && len(filteredFields) > 0 {
			if filterValue, wasFiltered := filteredFields[col]; wasFiltered {
				hiddenFilters[col] = filterValue
				continue
			}
		}

		// This column has data and should be displayed
		displayColumns = append(displayColumns, col)
		columnIndices = append(columnIndices, i)
	}

	// If no columns would be shown, fall back to basic set
	if len(displayColumns) == 0 {
		basicColumns := []string{"duration", "service_name", "name", "trace_id"}
		for _, col := range basicColumns {
			for i, c := range allColumns {
				if c == col {
					displayColumns = append(displayColumns, col)
					columnIndices = append(columnIndices, i)
					break
				}
			}
		}
	}

	return displayColumns, columnIndices, hiddenFilters
}

// formatDuration converts nanoseconds to human-readable duration
func formatDuration(ns int64) string {
	if ns < 0 {
		return "N/A"
	}

	// Convert to appropriate unit
	if ns < 1000 {
		return fmt.Sprintf("%dns", ns)
	} else if ns < 1000000 {
		return fmt.Sprintf("%.1fus", float64(ns)/1000)
	} else if ns < 1000000000 {
		return fmt.Sprintf("%.1fms", float64(ns)/1000000)
	} else {
		return fmt.Sprintf("%.2fs", float64(ns)/1000000000)
	}
}

// formatTimestamp converts milliseconds since epoch to readable timestamp
func formatTimestamp(ms int64) string {
	if ms <= 0 {
		return "N/A"
	}

	// Convert to time.Time
	t := time.Unix(0, ms*1000000) // ms to nanoseconds

	// Format as ISO 8601 without timezone (more compact)
	return t.Format("2006-01-02 15:04:05")
}

// formatValue formats a value based on column name
func formatValue(colName string, value interface{}) string {
	if value == nil {
		return ""
	}

	// Format based on column name
	switch colName {
	case "duration":
		// Duration is in nanoseconds
		if numVal, ok := toInt64(value); ok {
			return formatDuration(numVal)
		}

	case "timestamp", "start_time", "end_time":
		// Timestamps are in milliseconds
		if numVal, ok := toInt64(value); ok {
			return formatTimestamp(numVal)
		}

	case "trace_id", "parent_trace_id":
		// Show shortened trace ID (first 12 chars) for table display
		strVal := fmt.Sprintf("%v", value)
		if len(strVal) > 12 {
			return strVal[:12]
		}
		return strVal

	case "span_id", "parent_span_id":
		// Show shortened span ID (first 8 chars) for table display
		strVal := fmt.Sprintf("%v", value)
		if len(strVal) > 8 {
			return strVal[:8]
		}
		return strVal

	case "http_status_code", "status_code":
		// Don't show -1 (null placeholder)
		if numVal, ok := toInt64(value); ok && numVal == -1 {
			return ""
		}

	case "error":
		// Show as true/false
		if boolVal, ok := value.(bool); ok {
			if boolVal {
				return "true"
			}
			return ""
		}
	}

	// Default formatting
	return fmt.Sprintf("%v", value)
}

// toInt64 converts various numeric types to int64
func toInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	default:
		return 0, false
	}
}

// sortRowsByTraceHierarchy sorts rows by parent-child relationships for waterfall display
func sortRowsByTraceHierarchy(columns []string, rows [][]interface{}) [][]interface{} {
	// Find span_id, parent_span_id, and timestamp column indices
	spanIDIdx := -1
	parentSpanIDIdx := -1
	timestampIdx := -1
	for i, col := range columns {
		if col == "span_id" {
			spanIDIdx = i
		} else if col == "parent_span_id" {
			parentSpanIDIdx = i
		} else if col == "timestamp" {
			timestampIdx = i
		}
	}

	// If we don't have both columns, return original order
	if spanIDIdx == -1 || parentSpanIDIdx == -1 {
		return rows
	}

	// Build span_id -> row map
	spanToRow := make(map[string][]interface{})
	for _, row := range rows {
		if spanIDIdx < len(row) && row[spanIDIdx] != nil {
			spanID := fmt.Sprintf("%v", row[spanIDIdx])
			spanToRow[spanID] = row
		}
	}

	// Helper to extract timestamp from row
	getTimestamp := func(spanID string) int64 {
		if row, ok := spanToRow[spanID]; ok {
			if timestampIdx != -1 && timestampIdx < len(row) && row[timestampIdx] != nil {
				if ts, ok := toInt64(row[timestampIdx]); ok {
					return ts
				}
			}
		}
		return 0
	}

	// Build parent -> children map
	children := make(map[string][]string)
	var roots []string

	for _, row := range rows {
		if spanIDIdx >= len(row) || row[spanIDIdx] == nil {
			continue
		}
		spanID := fmt.Sprintf("%v", row[spanIDIdx])

		// Check parent
		var parentSpanID string
		if parentSpanIDIdx < len(row) && row[parentSpanIDIdx] != nil {
			parentSpanID = fmt.Sprintf("%v", row[parentSpanIDIdx])
		}

		// Empty or null parent means root span
		if parentSpanID == "" || parentSpanID == "null" || parentSpanID == "0" || parentSpanID == "00000000000000000000000000000000" {
			roots = append(roots, spanID)
		} else {
			children[parentSpanID] = append(children[parentSpanID], spanID)
		}
	}

	// Sort roots by timestamp
	if timestampIdx != -1 {
		for i := 0; i < len(roots); i++ {
			for j := i + 1; j < len(roots); j++ {
				if getTimestamp(roots[i]) > getTimestamp(roots[j]) {
					roots[i], roots[j] = roots[j], roots[i]
				}
			}
		}
	}

	// Sort children by timestamp for each parent
	if timestampIdx != -1 {
		for parentID, childList := range children {
			// Simple bubble sort by timestamp
			sorted := make([]string, len(childList))
			copy(sorted, childList)
			for i := 0; i < len(sorted); i++ {
				for j := i + 1; j < len(sorted); j++ {
					if getTimestamp(sorted[i]) > getTimestamp(sorted[j]) {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			children[parentID] = sorted
		}
	}

	// DFS traversal to build sorted list
	sortedRows := make([][]interface{}, 0, len(rows))
	visited := make(map[string]bool)

	var dfs func(spanID string)
	dfs = func(spanID string) {
		if visited[spanID] {
			return
		}
		visited[spanID] = true

		// Add this span
		if row, ok := spanToRow[spanID]; ok {
			sortedRows = append(sortedRows, row)
		}

		// Add children in timestamp order (already sorted)
		for _, childID := range children[spanID] {
			dfs(childID)
		}
	}

	// Start from roots (already sorted by timestamp)
	for _, rootID := range roots {
		dfs(rootID)
	}

	// Add any orphaned spans that weren't reached
	for spanID, row := range spanToRow {
		if !visited[spanID] {
			sortedRows = append(sortedRows, row)
		}
	}

	return sortedRows
}

// sortRowsByTimestamp sorts rows by timestamp in ascending order
func sortRowsByTimestamp(columns []string, rows [][]interface{}) [][]interface{} {
	// Find timestamp column index
	timestampIdx := -1
	for i, col := range columns {
		if col == "timestamp" {
			timestampIdx = i
			break
		}
	}

	// If no timestamp column, return original order
	if timestampIdx == -1 {
		return rows
	}

	// Copy rows to avoid modifying original
	sorted := make([][]interface{}, len(rows))
	copy(sorted, rows)

	// Simple bubble sort by timestamp (ascending)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			ts1 := int64(0)
			ts2 := int64(0)

			if timestampIdx < len(sorted[i]) && sorted[i][timestampIdx] != nil {
				if t, ok := toInt64(sorted[i][timestampIdx]); ok {
					ts1 = t
				}
			}

			if timestampIdx < len(sorted[j]) && sorted[j][timestampIdx] != nil {
				if t, ok := toInt64(sorted[j][timestampIdx]); ok {
					ts2 = t
				}
			}

			// Sort ascending (earlier timestamps first)
			if ts1 > ts2 {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// buildTraceIndentation builds an indentation map for trace hierarchy
func buildTraceIndentation(columns []string, rows [][]interface{}) map[int]int {
	indentMap := make(map[int]int)

	// Find span_id and parent_span_id column indices
	spanIDIdx := -1
	parentSpanIDIdx := -1
	for i, col := range columns {
		if col == "span_id" {
			spanIDIdx = i
		} else if col == "parent_span_id" {
			parentSpanIDIdx = i
		}
	}

	// If we don't have both columns, no indentation
	if spanIDIdx == -1 || parentSpanIDIdx == -1 {
		return indentMap
	}

	// Build span_id -> row index map
	spanToRow := make(map[string]int)
	for rowIdx, row := range rows {
		if spanIDIdx < len(row) && row[spanIDIdx] != nil {
			spanID := fmt.Sprintf("%v", row[spanIDIdx])
			spanToRow[spanID] = rowIdx
		}
	}

	// Calculate indentation depth for each row
	var calculateDepth func(rowIdx int, visited map[int]bool) int
	calculateDepth = func(rowIdx int, visited map[int]bool) int {
		if visited[rowIdx] {
			return 0 // Circular reference, treat as root
		}
		visited[rowIdx] = true

		row := rows[rowIdx]
		if parentSpanIDIdx >= len(row) || row[parentSpanIDIdx] == nil {
			return 0 // Root span (no parent)
		}

		parentSpanID := fmt.Sprintf("%v", row[parentSpanIDIdx])
		if parentSpanID == "" || parentSpanID == "0" || parentSpanID == "null" || parentSpanID == "00000000000000000000000000000000" {
			return 0 // Empty parent = root
		}

		// Find parent row
		if parentRowIdx, ok := spanToRow[parentSpanID]; ok {
			return 1 + calculateDepth(parentRowIdx, visited)
		}

		return 0 // Parent not in result set
	}

	// Calculate depth for each row
	for rowIdx := range rows {
		visited := make(map[int]bool)
		indentMap[rowIdx] = calculateDepth(rowIdx, visited)
	}

	return indentMap
}

// printFormattedRow prints a single row with formatted values
func printFormattedRow(columns []string, row []interface{}, indices []int, indent int) {
	widths := getColumnWidths(columns)

	for i, idx := range indices {
		if idx >= len(row) {
			fmt.Printf("%-*s", widths[i]+2, "N/A")
			continue
		}

		// Format the value
		strVal := formatValue(columns[i], row[idx])

		// Add indentation to the "name" column for trace hierarchy
		if columns[i] == "name" && indent > 0 {
			// Use tree characters for better visibility
			indentStr := strings.Repeat("  ", indent-1) + "└─"
			strVal = indentStr + strVal
		}

		// Truncate long values
		if len(strVal) > widths[i] {
			strVal = strVal[:widths[i]-3] + "..."
		}

		fmt.Printf("%-*s", widths[i]+2, strVal)
	}
	fmt.Println()
}

// printResults prints query results in a formatted table
func printResults(resp *QueryResponse, query string, verbose, allFields bool) {
	if len(resp.Results) == 0 {
		fmt.Println("No results")
		return
	}

	// Detect query context
	isCorrelateQuery := strings.Contains(strings.ToLower(query), "correlate")
	isExpandTrace := strings.Contains(strings.ToLower(query), "expand trace")
	signalType := detectSignalType(query)

	for i, result := range resp.Results {
		if len(resp.Results) > 1 {
			if isCorrelateQuery {
				// For correlate queries, show signal type header
				fmt.Printf("\n=== %s ===\n", detectSignalTypeFromSQL(result.SQL))
			} else {
				fmt.Printf("\n=== Result Set %d ===\n", i+1)
			}
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

		// Sort rows by trace hierarchy if this is an expand trace query
		displayRows := result.Rows
		if isExpandTrace {
			displayRows = sortRowsByTraceHierarchy(result.Columns, result.Rows)
		}

		// Filter columns unless --all-fields is specified
		displayColumns := result.Columns
		columnIndices := make([]int, len(result.Columns))
		for i := range columnIndices {
			columnIndices[i] = i
		}
		var hiddenFilters map[string]string

		if !allFields {
			// For correlate queries, detect signal type per result set from SQL
			resultSignalType := signalType
			if isCorrelateQuery {
				sqlType := detectSignalTypeFromSQL(result.SQL)
				resultSignalType = strings.ToLower(sqlType)
			}
			displayColumns, columnIndices, hiddenFilters = filterColumns(result.Columns, resultSignalType, displayRows, query)
		}

		if len(displayColumns) == 0 {
			fmt.Println("No displayable columns")
			continue
		}

		// Print filter summary if we hid any filtered columns
		if len(hiddenFilters) > 0 {
			filterParts := make([]string, 0)
			for field, value := range hiddenFilters {
				filterParts = append(filterParts, fmt.Sprintf("%s=%s", field, value))
			}
			fmt.Printf("[Filtered by: %s]\n", strings.Join(filterParts, ", "))
		}

		// Print table header
		printTableHeader(displayColumns)

		// Build indentation map for expand trace queries (after sorting!)
		indentMap := make(map[int]int) // row index -> indent level
		if isExpandTrace {
			indentMap = buildTraceIndentation(result.Columns, displayRows)
		}

		// Print table rows with formatting
		for rowIdx, row := range displayRows {
			indent := indentMap[rowIdx]
			printFormattedRow(displayColumns, row, columnIndices, indent)
		}

		fmt.Printf("\n%d row(s) returned\n", len(displayRows))
	}
}

// getDisplayColumnName returns a shortened column name for display
func getDisplayColumnName(col string) string {
	switch col {
	case "http_status_code":
		return "status"
	case "http_method":
		return "method"
	case "http_route":
		return "route"
	case "service_name":
		return "service"
	case "severity_text":
		return "severity"
	case "severity_number":
		return "sev_num"
	case "parent_span_id":
		return "parent"
	default:
		return col
	}
}

// getColumnWidths calculates the display width for each column
func getColumnWidths(columns []string) []int {
	widths := make([]int, len(columns))
	for i, col := range columns {
		// Use display name for width calculation
		displayName := getDisplayColumnName(col)
		widths[i] = len(displayName)

		// Set appropriate minimum widths based on column type
		minWidth := 10 // default minimum
		switch col {
		case "metric_name":
			minWidth = 30 // Metric names can be long (e.g., "jvm.threads.daemon.count")
		case "timestamp":
			minWidth = 19 // Timestamp: "2026-03-28 10:30:45"
		case "trace_id", "parent_trace_id":
			minWidth = 12 // Shortened trace IDs (first 12 chars)
		case "span_id", "parent_span_id":
			minWidth = 8 // Shortened span IDs (first 8 chars)
		case "name":
			minWidth = 20 // Span/operation names
		case "body":
			minWidth = 40 // Log message bodies
		case "service_name":
			minWidth = 15 // Service names
		case "host_name":
			minWidth = 12 // Host names
		case "exemplar_trace_id":
			minWidth = 12 // Exemplar trace IDs (shortened)
		case "value":
			minWidth = 12 // Metric values (including scientific notation)
		case "duration":
			minWidth = 10 // Span durations
		case "http_method":
			minWidth = 6 // GET, POST, etc.
		case "http_status_code", "status_code":
			minWidth = 6 // 200, 404, etc.
		case "http_route":
			minWidth = 15 // Route patterns
		case "error":
			minWidth = 5 // true/false
		}

		if widths[i] < minWidth {
			widths[i] = minWidth
		}
	}
	return widths
}

// printTableHeader prints the table header
func printTableHeader(columns []string) {
	widths := getColumnWidths(columns)

	// Print header with display names
	for i, col := range columns {
		displayName := getDisplayColumnName(col)
		fmt.Printf("%-*s", widths[i]+2, displayName)
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

	// Pattern 1: "invalid condition: X=Y" (missing quotes or field name issue)
	// Example: "where service=replicator" -> "where service_name = \"replicator\""
	if strings.Contains(errorMsg, "invalid condition:") {
		// Extract the problematic condition from error message
		parts := strings.Split(errorMsg, "invalid condition: ")
		if len(parts) >= 2 {
			badCondition := strings.TrimSpace(parts[1])

			// Try to find an equals sign (either = or ==)
			var eqIdx int
			var opLen int
			if idx := strings.Index(badCondition, "=="); idx != -1 {
				eqIdx = idx
				opLen = 2
			} else if idx := strings.Index(badCondition, "="); idx != -1 {
				eqIdx = idx
				opLen = 1
			} else {
				// No equals sign, can't fix
				return ""
			}

			if eqIdx > 0 && eqIdx < len(badCondition)-opLen {
				left := strings.TrimSpace(badCondition[:eqIdx])
				right := strings.TrimSpace(badCondition[eqIdx+opLen:])

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

				// Build the corrected condition (keep operator simple)
				fixedCondition := left + " = " + right

				// Replace in the original query
				return strings.Replace(query, badCondition, fixedCondition, 1)
			}
		}
	}

	// Pattern 2: "query must start with 'signal='"
	if strings.Contains(errorMsg, "query must start with 'signal='") {
		// Check if query starts with "signal " (with space instead of =)
		if strings.HasPrefix(strings.ToLower(query), "signal ") {
			// Remove spaces around = in "signal = type"
			fixedQuery := strings.Replace(query, "signal ", "signal=", 1)
			// Also remove space after = if present
			if strings.HasPrefix(strings.ToLower(fixedQuery), "signal= ") {
				fixedQuery = strings.Replace(fixedQuery, "signal= ", "signal=", 1)
			}
			return fixedQuery
		}
		// Otherwise add signal=spans as default
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
