#!/bin/bash
set -e

echo "🚀 Starting Apache Pinot for OTEL-OQL..."

# Check if Podman is available
if ! command -v podman &> /dev/null; then
    echo "❌ Podman is not installed. Please install Podman first."
    exit 1
fi

# Check if Podman is running (for podman machine on Mac/Windows)
if ! podman info > /dev/null 2>&1; then
    echo "❌ Podman is not running. Try: podman machine start"
    exit 1
fi

# Check if Pinot container already exists
if podman ps -a --format '{{.Names}}' | grep -q '^pinot-quickstart$'; then
    echo "📦 Found existing Pinot container"

    # Check if it's running
    if podman ps --format '{{.Names}}' | grep -q '^pinot-quickstart$'; then
        echo "✅ Pinot is already running"
    else
        echo "▶️  Starting existing Pinot container..."
        podman start pinot-quickstart
        echo "⏳ Waiting for Pinot to be ready..."
        sleep 5
    fi
else
    echo "📥 Pulling Apache Pinot image..."
    podman pull docker.io/apachepinot/pinot:latest

    echo "🐳 Starting Pinot container..."
    podman run \
      --name pinot-quickstart \
      -p 9000:9000 \
      -p 8099:8099 \
      -d \
      docker.io/apachepinot/pinot:latest QuickStart -type batch

    echo "⏳ Waiting for Pinot to initialize (30 seconds)..."
    sleep 30
fi

# Check Pinot health
echo "🏥 Checking Pinot health..."
MAX_RETRIES=10
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -s http://localhost:9000/health | grep -q "OK"; then
        echo "✅ Pinot is healthy and ready!"
        break
    else
        RETRY_COUNT=$((RETRY_COUNT + 1))
        echo "⏳ Waiting for Pinot to be ready... ($RETRY_COUNT/$MAX_RETRIES)"
        sleep 3
    fi
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "❌ Pinot failed to start properly"
    echo "📋 Check logs with: podman logs pinot-quickstart"
    exit 1
fi

echo ""
echo "🎉 Pinot is running!"
echo ""
echo "📊 Pinot UI:        http://localhost:9000"
echo "🔌 Broker API:      http://localhost:9000/query/sql"
echo "⚙️  Controller API:  http://localhost:8099"
echo ""
echo "Next steps:"
echo "  1. Build OTEL-OQL:        go build -o otel-oql ./cmd/otel-oql"
echo "  2. Initialize schemas:    ./otel-oql setup-schema --pinot-url=http://localhost:9000"
echo "  3. Start service:         ./otel-oql --test-mode --pinot-url=http://localhost:9000"
echo ""
