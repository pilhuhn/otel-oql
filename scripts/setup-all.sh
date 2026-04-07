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
echo "⏳ Waiting for Kafka metadata to propagate (3 seconds)..."
sleep 3
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

# Step 6: Optional Perses datasource setup
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 Step 6 (Optional): Configure Perses Datasources"
echo ""
echo "Would you like to configure Perses datasources now? (y/n)"
read -r configure_perses

if [[ "$configure_perses" =~ ^[Yy]$ ]]; then
    ./scripts/setup-perses.sh
else
    echo "⏭️  Skipping Perses configuration"
    echo "You can run './scripts/setup-perses.sh' later to configure datasources"
    echo ""
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🎉 Setup complete!"
echo ""
echo "Infrastructure running (podman compose):"
echo "  • Kafka:  localhost:9092"
echo "  • Pinot:  localhost:9000"
echo "  • Perses: localhost:8082"
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
echo "Access Perses UI:"
echo "  http://localhost:8082"
echo ""
echo "Stop infrastructure:"
echo "  podman compose down"
echo ""
