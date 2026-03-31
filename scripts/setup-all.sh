#!/bin/bash
set -e

echo "🚀 OTEL-OQL Complete Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Check for podman compose
if ! podman compose version > /dev/null 2>&1; then
    echo "❌ podman-compose not found"
    echo "Install with: pip3 install podman-compose"
    echo "Or use: brew install podman-compose"
    exit 1
fi

# Step 1: Build OTEL-OQL and CLI
echo "📦 Step 1: Building OTEL-OQL and CLI..."
go build -o otel-oql ./cmd/otel-oql
go build -o oql-cli ./cmd/oql-cli
echo "✅ Build complete"
echo ""

# Step 2: Start infrastructure with compose
echo "🐳 Step 2: Starting Kafka and Pinot with compose..."
podman compose up -d
echo "⏳ Waiting for services to be healthy (40 seconds)..."
sleep 40
echo ""

# Step 3: Create Kafka topics
echo "📝 Step 3: Creating Kafka topics..."
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

# Step 4: Initialize schemas
echo "📊 Step 4: Creating Pinot schemas..."
./otel-oql setup-schema --pinot-url=http://localhost:9000
echo "✅ Schemas created"
echo ""

# Step 5: Verify setup
echo "🔍 Step 5: Verifying setup..."
./scripts/verify-setup.sh
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🎉 Setup complete!"
echo ""
echo "Infrastructure running (podman compose):"
echo "  • Kafka:  localhost:9092"
echo "  • Pinot:  localhost:9000"
echo ""
echo "Start the service with:"
echo "  ./otel-oql --test-mode"
echo ""
echo "Or use the config file:"
echo "  ./otel-oql --config=otel-oql.yaml"
echo ""
echo "Query the service with the CLI:"
echo "  ./oql-cli --tenant-id=0 \"signal=spans limit 10\""
echo ""
echo "Stop infrastructure:"
echo "  podman compose down"
echo ""
