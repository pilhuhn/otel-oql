#!/bin/bash
# PromQL Examples for OTEL-OQL
# These examples demonstrate how to query metrics using PromQL syntax

# Set the API endpoint and tenant ID
API_URL="http://localhost:8080/query"
TENANT_ID="0"

echo "PromQL Examples for OTEL-OQL"
echo "=============================="
echo ""

# Example 1: Simple metric query
echo "1. Simple metric query:"
echo "   http_requests_total"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "http_requests_total",
    "language": "promql"
  }' | jq .
echo ""

# Example 2: Metric with label matchers
echo "2. Metric with label matchers:"
echo "   http_requests_total{job=\"api\", status=\"200\"}"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "http_requests_total{job=\"api\", status=\"200\"}",
    "language": "promql"
  }' | jq .
echo ""

# Example 3: Range vector selector
echo "3. Range vector selector (last 5 minutes):"
echo "   http_requests_total[5m]"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "http_requests_total[5m]",
    "language": "promql"
  }' | jq .
echo ""

# Example 4: Aggregation
echo "4. Sum aggregation:"
echo "   sum(http_requests_total)"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "sum(http_requests_total)",
    "language": "promql"
  }' | jq .
echo ""

# Example 5: Aggregation with grouping
echo "5. Sum with GROUP BY:"
echo "   sum by (job) (http_requests_total)"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "sum by (job) (http_requests_total)",
    "language": "promql"
  }' | jq .
echo ""

# Example 6: Rate function
echo "6. Rate function:"
echo "   rate(http_requests_total[5m])"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "rate(http_requests_total[5m])",
    "language": "promql"
  }' | jq .
echo ""

# Example 7: Value comparison
echo "7. Value comparison:"
echo "   cpu_usage > 80"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "cpu_usage > 80",
    "language": "promql"
  }' | jq .
echo ""

# Example 8: Regex label matching
echo "8. Regex label matching:"
echo "   http_requests_total{job=~\"api.*\"}"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "http_requests_total{job=~\"api.*\"}",
    "language": "promql"
  }' | jq .
echo ""

# Example 9: Negative label matching
echo "9. Negative label matching:"
echo "   http_requests_total{status!=\"500\"}"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "http_requests_total{status!=\"500\"}",
    "language": "promql"
  }' | jq .
echo ""

# Example 10: Multiple aggregations
echo "10. Average CPU usage:"
echo "    avg(cpu_usage)"
curl -s -X POST "$API_URL" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "avg(cpu_usage)",
    "language": "promql"
  }' | jq .
echo ""

echo "=============================="
echo "All PromQL examples completed!"
echo ""
echo "Compare with OQL syntax:"
echo "  PromQL: http_requests_total{job=\"api\"}"
echo "  OQL:    signal=metrics | where metric_name == \"http_requests_total\" | where attributes.job == \"api\""
