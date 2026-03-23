#!/bin/bash
set -e

echo "🚀 OTEL-OQL Complete Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Step 1: Build OTEL-OQL
echo "📦 Step 1: Building OTEL-OQL..."
go build -o otel-oql ./cmd/otel-oql
echo "✅ Build complete"
echo ""

# Step 2: Start Pulsar
echo "🐳 Step 2: Starting Pulsar..."
./scripts/start-pulsar.sh
echo ""

# Step 3: Start Pinot
echo "🐳 Step 3: Starting Pinot..."
./scripts/start-pinot.sh
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
echo "Start the service with:"
echo "  ./otel-oql --test-mode --pinot-url=http://localhost:9000 --pulsar-url=pulsar://localhost:6650"
echo ""
echo "Or in production mode (requires tenant-id):"
echo "  ./otel-oql --pinot-url=http://localhost:9000 --pulsar-url=pulsar://localhost:6650"
echo ""
