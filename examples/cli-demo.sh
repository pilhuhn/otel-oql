#!/bin/bash
# Demo script for oql-cli usage examples
# This script shows various ways to use the CLI tool

echo "=== OTEL-OQL CLI Demo ==="
echo ""

# Check if oql-cli is built
if [ ! -f "./oql-cli" ]; then
    echo "Building oql-cli..."
    go build -o oql-cli ./cmd/oql-cli
    echo ""
fi

echo "Prerequisites:"
echo "  - OTEL-OQL service running on localhost:8080"
echo "  - Test data ingested (use: go run cmd/send-test-data/main.go)"
echo ""
echo "Press Enter to continue..."
read

# Example 1: Simple query
echo "=== Example 1: Simple Query ==="
echo "Command: ./oql-cli --tenant-id=0 \"signal=spans limit 5\""
echo ""
echo "This executes a basic OQL query to get 5 recent spans."
echo ""

# Example 2: Verbose output
echo "=== Example 2: Verbose Output ==="
echo "Command: ./oql-cli --tenant-id=0 --verbose \"signal=spans limit 3\""
echo ""
echo "Verbose mode shows:"
echo "  - Generated Pinot SQL"
echo "  - Query statistics (docs scanned, execution time)"
echo "  - Result data"
echo ""

# Example 3: Piping from stdin
echo "=== Example 3: Query from stdin ==="
echo "Command: echo \"signal=metrics where metric_name == \\\"cpu.usage\\\"\" | ./oql-cli --tenant-id=0"
echo ""
echo "Useful for scripting and automation."
echo ""

# Example 4: Multi-line interactive
echo "=== Example 4: Interactive Multi-line Mode ==="
echo "Command: ./oql-cli --tenant-id=0"
echo ""
echo "Then type:"
echo "  signal=spans"
echo "  where duration > 100"
echo "  since 1h"
echo "  limit 10"
echo "  <Ctrl+D to submit>"
echo ""
echo "Great for composing complex queries with better readability."
echo ""

# Example 5: JSON output for scripting
echo "=== Example 5: JSON Output for Scripting ==="
echo "Command: ./oql-cli --tenant-id=0 --json \"signal=spans limit 2\" | jq '.results[0].rows'"
echo ""
echo "Perfect for:"
echo "  - Integration with other tools"
echo "  - Parsing with jq, python, etc."
echo "  - Automated testing"
echo ""

# Example 6: Complex OQL queries
echo "=== Example 6: Advanced OQL Query ==="
echo "Command: ./oql-cli --tenant-id=0 \"signal=spans where http_status_code >= 500 group by service_name count()\""
echo ""
echo "This query:"
echo "  - Filters spans with HTTP 5xx errors"
echo "  - Groups by service name"
echo "  - Counts errors per service"
echo ""

# Example 7: Time-based queries
echo "=== Example 7: Time Range Queries ==="
echo "Command: ./oql-cli --tenant-id=0 \"signal=spans since 1h where duration > 1000\""
echo ""
echo "Queries from the last hour where duration > 1 second."
echo ""

# Example 8: Trace expansion
echo "=== Example 8: Trace Expansion ==="
echo "Command: ./oql-cli --tenant-id=0 \"signal=spans where name == \\\"checkout\\\" limit 1 expand trace\""
echo ""
echo "This reconstructs the full trace waterfall for debugging."
echo ""

# Example 9: Self-observability
echo "=== Example 9: Query Self-Observability Data ==="
echo "Command: ./oql-cli --tenant-id=0 \"signal=spans where service_name == \\\"otel-oql\\\" since 5m\""
echo ""
echo "If observability is enabled, you can query the service's own telemetry!"
echo ""

echo "=== End of Demo ==="
echo ""
echo "Try running these commands yourself!"
echo "Full documentation: ./cmd/oql-cli/README.md"
