#!/bin/bash
set -e

CLICKHOUSE_URL="${CLICKHOUSE_URL:-http://localhost:8123}"
OTEL_OQL_ARGS="${OTEL_OQL_ARGS:---test-mode}"

cd "$(dirname "$0")/.."

# Build if binary is missing or source is newer
if [ ! -f "./otel-oql" ] || find cmd/otel-oql -name '*.go' -newer otel-oql | grep -q .; then
    echo "Building otel-oql..."
    go build -o otel-oql ./cmd/otel-oql
fi

# Start Clickhouse if not already running
if ! curl -sf "${CLICKHOUSE_URL}/ping" > /dev/null 2>&1; then
    echo "Starting Clickhouse..."
    podman compose up -d clickhouse

    echo -n "Waiting for Clickhouse"
    for i in $(seq 1 30); do
        if curl -sf "${CLICKHOUSE_URL}/ping" > /dev/null 2>&1; then
            echo " ready"
            break
        fi
        echo -n "."
        sleep 1
        if [ $i -eq 30 ]; then
            echo ""
            echo "❌ Clickhouse did not start in time"
            exit 1
        fi
    done

    echo "Setting up schema..."
    ./otel-oql setup-schema --clickhouse-url="${CLICKHOUSE_URL}"
else
    echo "Clickhouse already running at ${CLICKHOUSE_URL}"
fi

echo "Starting otel-oql ${OTEL_OQL_ARGS}..."
exec ./otel-oql ${OTEL_OQL_ARGS}
