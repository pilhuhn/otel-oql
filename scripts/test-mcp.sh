#!/bin/bash
set -e

echo "Testing MCP Server..."
echo ""

BASE_URL="http://localhost:8090"

# Test 1: List tools
echo "1. Testing /mcp/v1/tools/list"
echo "================================"
curl -s "$BASE_URL/mcp/v1/tools/list" | jq '.' || echo "Error: Failed to list tools"
echo ""
echo ""

# Test 2: Call oql_help (all topics)
echo "2. Testing oql_help tool (all topics)"
echo "======================================"
curl -s "$BASE_URL/mcp/v1/tools/call" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "oql_help",
    "arguments": {
      "topic": "all"
    }
  }' | jq -r '.content[0].text' | head -50
echo ""
echo "[... truncated ...]"
echo ""
echo ""

# Test 3: Call oql_help (operators only)
echo "3. Testing oql_help tool (operators topic)"
echo "=========================================="
curl -s "$BASE_URL/mcp/v1/tools/call" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "oql_help",
    "arguments": {
      "topic": "operators"
    }
  }' | jq -r '.content[0].text' | head -30
echo ""
echo "[... truncated ...]"
echo ""
echo ""

# Test 4: Call oql_query
echo "4. Testing oql_query tool"
echo "========================="
curl -s "$BASE_URL/mcp/v1/tools/call" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "oql_query",
    "arguments": {
      "tenant_id": 0,
      "query": "signal=spans | where duration > 500ms | limit 5"
    }
  }' | jq '.'
echo ""
echo ""

# Test 5: Test invalid query
echo "5. Testing error handling (invalid query)"
echo "=========================================="
curl -s "$BASE_URL/mcp/v1/tools/call" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "oql_query",
    "arguments": {
      "tenant_id": 0,
      "query": "signal=spans | where duration > 5.5.5s"
    }
  }' | jq '.'
echo ""
echo ""

echo "MCP Server tests completed!"
