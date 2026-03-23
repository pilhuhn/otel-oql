#!/bin/bash
set -e

echo "🚀 OTEL-OQL Simple Setup (Kafka + Pinot)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Step 1: Build binaries
echo "📦 Step 1: Building binaries..."
go build -o otel-oql ./cmd/otel-oql
go build -o oql-cli ./cmd/oql-cli
echo "✅ Build complete"
echo ""

# Step 2: Start infrastructure
echo "🐳 Step 2: Starting Kafka and Pinot..."
podman compose -f compose-simple.yaml up -d
echo ""

# Step 3: Wait for services
echo "⏳ Step 3: Waiting for services to be ready (40 seconds)..."
sleep 40
echo ""

# Step 4: Verify Kafka
echo "🔍 Step 4: Verifying Kafka..."
if nc -z localhost 9092 2>/dev/null; then
    echo "  ✓ Kafka is reachable on localhost:9092"
else
    echo "  ✗ Kafka not reachable"
    exit 1
fi
echo ""

# Step 5: Verify Pinot
echo "🔍 Step 5: Verifying Pinot..."
if curl -sf http://localhost:9000/health > /dev/null; then
    echo "  ✓ Pinot is healthy on localhost:9000"
else
    echo "  ✗ Pinot not healthy"
    exit 1
fi
echo ""

# Step 6: Create Kafka topics
echo "📝 Step 6: Creating Kafka topics..."
for topic in otel-spans otel-metrics otel-logs; do
    podman exec kafka /opt/kafka/bin/kafka-topics.sh \
        --create \
        --topic $topic \
        --bootstrap-server localhost:9092 \
        --partitions 3 \
        --replication-factor 1 2>&1 || echo "  Topic $topic may already exist"
done
echo "✅ Kafka topics created"
echo ""

# Step 7: Create Pinot schemas
echo "📊 Step 7: Creating Pinot schemas and tables..."
./otel-oql setup-schema --pinot-url=http://localhost:9000
echo "✅ Schemas created"
echo ""

# Step 8: Verify tables
echo "🔍 Step 8: Verifying tables..."
curl -s http://localhost:9000/tables | jq -r '.tables[]' | grep otel || echo "Warning: No otel tables found"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🎉 Setup complete!"
echo ""
echo "Infrastructure running:"
echo "  • Kafka:  localhost:9092"
echo "  • Pinot:  localhost:9000"
echo ""
echo "Tables created:"
echo "  • otel_spans"
echo "  • otel_metrics"
echo "  • otel_logs"
echo ""
echo "Start OTEL-OQL service:"
echo "  ./otel-oql --test-mode"
echo ""
echo "Query with CLI:"
echo "  ./oql-cli --tenant-id=0 \"signal=spans limit 10\""
echo ""
echo "Stop infrastructure:"
echo "  podman compose -f compose-simple.yaml down"
echo ""
