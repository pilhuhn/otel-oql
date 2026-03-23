#!/bin/bash

echo "📝 Sending test data via OTLP..."
echo ""

# Check if OTEL-OQL service is running
if ! lsof -i :4318 > /dev/null 2>&1; then
    echo "❌ OTEL-OQL service is not running on port 4318"
    echo "Start it with: ./otel-oql --test-mode --pinot-url=http://localhost:9000"
    exit 1
fi

echo "Creating test span (trace-001, span-001)..."

# Send a test span via OTLP HTTP
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -H "tenant-id: 0" \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{
          "key": "service.name",
          "value": {"stringValue": "payment-service"}
        }]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "74726163652d303031000000000000",
          "spanId": "7370616e2d303031",
          "name": "checkout",
          "kind": 2,
          "startTimeUnixNano": "1710000000000000000",
          "endTimeUnixNano": "1710000150000000000",
          "attributes": [
            {
              "key": "http.method",
              "value": {"stringValue": "POST"}
            },
            {
              "key": "http.status_code",
              "value": {"intValue": "200"}
            },
            {
              "key": "http.route",
              "value": {"stringValue": "/api/checkout"}
            }
          ],
          "status": {
            "code": 0
          }
        }]
      }]
    }]
  }' 2>/dev/null

echo ""
echo "✅ Test span sent!"
echo ""

echo "Creating test metric with exemplar..."

# Send a test metric via OTLP HTTP
curl -X POST http://localhost:4318/v1/metrics \
  -H "Content-Type: application/json" \
  -H "tenant-id: 0" \
  -d '{
    "resourceMetrics": [{
      "resource": {
        "attributes": [{
          "key": "service.name",
          "value": {"stringValue": "api-gateway"}
        }]
      },
      "scopeMetrics": [{
        "metrics": [{
          "name": "http.server.duration",
          "gauge": {
            "dataPoints": [{
              "timeUnixNano": "1710000001000000000",
              "asDouble": 6500.0,
              "attributes": [{
                "key": "environment",
                "value": {"stringValue": "production"}
              }],
              "exemplars": [{
                "timeUnixNano": "1710000001000000000",
                "asDouble": 6500.0,
                "traceId": "74726163652d303032000000000000",
                "spanId": "7370616e2d303033"
              }]
            }]
          }
        }]
      }]
    }]
  }' 2>/dev/null

echo ""
echo "✅ Test metric sent!"
echo ""

echo "Creating test log..."

# Send a test log via OTLP HTTP
curl -X POST http://localhost:4318/v1/logs \
  -H "Content-Type: application/json" \
  -H "tenant-id: 0" \
  -d '{
    "resourceLogs": [{
      "resource": {
        "attributes": [{
          "key": "service.name",
          "value": {"stringValue": "payment-service"}
        }]
      },
      "scopeLogs": [{
        "logRecords": [{
          "timeUnixNano": "1710000001000000000",
          "severityNumber": 17,
          "severityText": "ERROR",
          "body": {
            "stringValue": "Payment processing failed: timeout"
          },
          "traceId": "74726163652d303032000000000000",
          "spanId": "7370616e2d303033",
          "attributes": [{
            "key": "log.level",
            "value": {"stringValue": "ERROR"}
          }]
        }]
      }]
    }]
  }' 2>/dev/null

echo ""
echo "✅ Test log sent!"
echo ""
echo "⏳ Waiting for Pinot to ingest data (5 seconds)..."
sleep 5
echo ""
echo "✅ Test data should now be in Pinot!"
echo ""
echo "Verify with queries:"
echo ""
echo "  # Via Pinot UI:"
echo "  open http://localhost:9000/#/query"
echo ""
echo "  # Via SQL in Pinot UI:"
echo "  SELECT * FROM otel_spans WHERE tenant_id = 0;"
echo "  SELECT * FROM otel_metrics WHERE tenant_id = 0;"
echo "  SELECT * FROM otel_logs WHERE tenant_id = 0;"
echo ""
echo "  # Via OQL:"
echo "  curl -X POST http://localhost:8080/query \\"
echo "    -H 'X-Tenant-ID: 0' -H 'Content-Type: application/json' \\"
echo "    -d '{\"query\": \"signal=spans | where tenant_id == 0 | limit 10\"}'"
echo ""
