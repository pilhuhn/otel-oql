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
echo "🐳 Step 2: Starting Clickhouse with compose..."
podman compose up -d
echo "⏳ Waiting for Clickhouse to be healthy (15 seconds)..."
sleep 15
echo ""

# Step 3: Initialize schemas
echo "📊 Step 3: Creating Clickhouse schemas..."
./otel-oql setup-schema --clickhouse-url=http://localhost:8123
echo "✅ Schemas created"
echo ""

# Step 4: Verify setup
echo "🔍 Step 4: Verifying setup..."
./scripts/verify-setup.sh
echo ""

# Step 5: Optional Perses datasource setup
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 Step 5 (Optional): Configure Perses Datasources"
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
echo "  • Clickhouse: localhost:8123 (HTTP), localhost:9000 (native)"
echo "  • Perses:     localhost:8082"
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
