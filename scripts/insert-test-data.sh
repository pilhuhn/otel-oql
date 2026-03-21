#!/bin/bash

echo "📝 Inserting test data into Pinot..."
echo ""

# Check if Pinot is running
if ! curl -s http://localhost:9000/health | grep -q "OK"; then
    echo "❌ Pinot is not running or not healthy"
    echo "Run: ./scripts/start-pinot.sh"
    exit 1
fi

echo "Creating test spans..."

# Insert test spans via Pinot SQL
curl -X POST http://localhost:9000/query/sql \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "INSERT INTO otel_spans (tenant_id, trace_id, span_id, parent_span_id, name, kind, service_name, http_method, http_status_code, http_route, error, timestamp, duration, status_code) VALUES (0, '\'trace-001\'', '\'span-001\'', '\'span-000\'', '\'checkout\'', '\'SERVER\'', '\'payment-service\'', '\'POST\'', 200, \''/api/checkout\'', false, 1710000000000, 150000000, '\'OK\'')"
  }' 2>/dev/null

echo ""

curl -X POST http://localhost:9000/query/sql \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "INSERT INTO otel_spans (tenant_id, trace_id, span_id, parent_span_id, name, kind, service_name, http_method, http_status_code, error, timestamp, duration, status_code) VALUES (0, '\'trace-001\'', '\'span-002\'', '\'span-001\'', '\'process_payment\'', '\'INTERNAL\'', '\'payment-service\'', NULL, NULL, false, 1710000000100, 50000000, '\'OK\'')"
  }' 2>/dev/null

echo ""

curl -X POST http://localhost:9000/query/sql \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "INSERT INTO otel_spans (tenant_id, trace_id, span_id, parent_span_id, name, kind, service_name, http_method, http_status_code, error, timestamp, duration, status_code) VALUES (0, '\'trace-002\'', '\'span-003\'', '\'span-000\'', '\'api_call\'', '\'SERVER\'', '\'api-gateway\'', '\'GET\'', 500, true, 1710000001000, 250000000, '\'ERROR\'')"
  }' 2>/dev/null

echo ""
echo "Creating test metrics..."

curl -X POST http://localhost:9000/query/sql \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "INSERT INTO otel_metrics (tenant_id, metric_name, metric_type, service_name, value, timestamp) VALUES (0, '\'http.server.duration\'', '\'histogram\'', '\'payment-service\'', 150.5, 1710000000000)"
  }' 2>/dev/null

echo ""

curl -X POST http://localhost:9000/query/sql \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "INSERT INTO otel_metrics (tenant_id, metric_name, metric_type, service_name, value, exemplar_trace_id, timestamp) VALUES (0, '\'http.server.duration\'', '\'histogram\'', '\'api-gateway\'', 6500.0, '\'trace-002\'', 1710000001000)"
  }' 2>/dev/null

echo ""
echo "Creating test logs..."

curl -X POST http://localhost:9000/query/sql \
  -H "Content-Type: application/json" \
  -d '{
    "sql": "INSERT INTO otel_logs (tenant_id, trace_id, span_id, severity_text, severity_number, body, service_name, timestamp) VALUES (0, '\'trace-002\'', '\'span-003\'', '\'ERROR\'', 17, '\'Payment processing failed: timeout\'', '\'payment-service\'', 1710000001000)"
  }' 2>/dev/null

echo ""
echo "✅ Test data inserted!"
echo ""
echo "Verify with queries:"
echo ""
echo "  # Via Pinot UI:"
echo "  open http://localhost:9000/#/query"
echo ""
echo "  # Via SQL:"
echo "  SELECT * FROM otel_spans WHERE tenant_id = 0;"
echo "  SELECT * FROM otel_metrics WHERE tenant_id = 0;"
echo "  SELECT * FROM otel_logs WHERE tenant_id = 0;"
echo ""
echo "  # Via OQL (requires OTEL-OQL service running):"
echo "  curl -X POST http://localhost:8080/query \\"
echo "    -H 'X-Tenant-ID: 0' -H 'Content-Type: application/json' \\"
echo "    -d '{\"query\": \"signal=spans | where tenant_id == 0 | limit 10\"}'"
echo ""
