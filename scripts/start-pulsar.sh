#!/bin/bash
set -e

echo "🚀 Starting Apache Pulsar for OTEL-OQL..."

# Check if Podman is available
if ! command -v podman &> /dev/null; then
    echo "❌ Podman is not installed. Please install Podman first."
    exit 1
fi

# Check if Podman is running
if ! podman info > /dev/null 2>&1; then
    echo "❌ Podman is not running. Try: podman machine start"
    exit 1
fi

# Check if Pulsar container already exists
if podman ps -a --format '{{.Names}}' | grep -q '^pulsar-standalone$'; then
    echo "📦 Found existing Pulsar container"

    if podman ps --format '{{.Names}}' | grep -q '^pulsar-standalone$'; then
        echo "✅ Pulsar is already running"
    else
        echo "▶️  Starting existing Pulsar container..."
        podman start pulsar-standalone
        echo "⏳ Waiting for Pulsar to be ready..."
        sleep 10
    fi
else
    echo "📥 Pulling Apache Pulsar image..."
    podman pull docker.io/apachepulsar/pulsar:4.1.3

    echo "🐳 Starting Pulsar container..."
    podman run -d \
      --name pulsar-standalone \
      -p 6650:6650 \
      -p 8081:8080 \
      docker.io/apachepulsar/pulsar:4.1.3 \
      bin/pulsar standalone

    echo "⏳ Waiting for Pulsar to initialize (30 seconds)..."
    sleep 30
fi

# Health check
echo "🏥 Checking Pulsar health..."
MAX_RETRIES=10
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -s http://localhost:8081/admin/v2/brokers/health | grep -q "ok"; then
        echo "✅ Pulsar is healthy and ready!"
        break
    else
        RETRY_COUNT=$((RETRY_COUNT + 1))
        echo "⏳ Waiting for Pulsar... ($RETRY_COUNT/$MAX_RETRIES)"
        sleep 3
    fi
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "❌ Pulsar failed to start properly"
    echo "📋 Check logs with: podman logs pulsar-standalone"
    exit 1
fi

echo ""
echo "🎉 Pulsar is running!"
echo ""
echo "📊 Pulsar Admin UI:  http://localhost:8081"
echo "🔌 Broker URL:       pulsar://localhost:6650"
echo ""
echo "Next steps:"
echo "  1. Start Pinot:              ./scripts/start-pinot.sh"
echo "  2. Create REALTIME schemas:  ./otel-oql setup-schema --pinot-url=http://localhost:9000"
echo "  3. Start OTEL-OQL:           ./otel-oql --test-mode --pinot-url=http://localhost:9000 --pulsar-url=pulsar://localhost:6650"
echo ""
