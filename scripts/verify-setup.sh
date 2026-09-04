#!/bin/bash

echo "🔍 Verifying OTEL-OQL Setup..."
echo ""

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

ERRORS=0

# Check Podman
echo -n "🐳 Podman running: "
if podman info > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC} Podman is not running"
    echo "   Try: podman machine start"
    ERRORS=$((ERRORS + 1))
fi

# Check Clickhouse container
echo -n "📦 Clickhouse container exists: "
if podman ps -a --format '{{.Names}}' | grep -q 'clickhouse'; then
    echo -e "${GREEN}✓${NC}"

    echo -n "▶️  Clickhouse container running: "
    if podman ps --format '{{.Names}}' | grep -q 'clickhouse'; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC} Container exists but is stopped"
        echo "   Run: podman compose up -d"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo -e "${RED}✗${NC} Clickhouse container not found"
    echo "   Run: podman compose up -d"
    ERRORS=$((ERRORS + 1))
fi

# Check Clickhouse connectivity
echo -n "🏥 Clickhouse health: "
if curl -s http://localhost:8123/ping 2>/dev/null | grep -q "Ok"; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC} Cannot connect to Clickhouse on localhost:8123"
    echo "   Ensure Clickhouse is running: podman compose up -d"
    ERRORS=$((ERRORS + 1))
fi

# Check for otel-oql binary
echo -n "🔨 OTEL-OQL binary built: "
if [ -f "./otel-oql" ]; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${YELLOW}⚠${NC} Binary not found"
    echo "   Run: go build -o otel-oql ./cmd/otel-oql"
fi

# Check for oql-cli binary
echo -n "🔨 OQL CLI binary built: "
if [ -f "./oql-cli" ]; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${YELLOW}⚠${NC} Binary not found"
    echo "   Run: go build -o oql-cli ./cmd/oql-cli"
fi

# Check Clickhouse tables
echo ""
echo "📊 Checking Clickhouse tables..."

TABLES_OK=0
if curl -s http://localhost:8123/ping 2>/dev/null | grep -q "Ok"; then
    for table in otel_spans otel_metrics otel_logs; do
        echo -n "   $table: "
        result=$(curl -s "http://localhost:8123/?query=SELECT+count()+FROM+${table}" 2>/dev/null)
        if [ $? -eq 0 ] && echo "$result" | grep -qE '^[0-9]+$'; then
            echo -e "${GREEN}✓${NC}"
            TABLES_OK=$((TABLES_OK + 1))
        else
            echo -e "${RED}✗${NC} Not found or not queryable"
        fi
    done

    if [ $TABLES_OK -eq 0 ]; then
        echo ""
        echo "   ${YELLOW}No tables found${NC}"
        echo "   Run: ./otel-oql setup-schema --clickhouse-url=http://localhost:8123"
    fi
fi

# Check OTEL-OQL service (if running)
echo ""
echo "🔌 Checking OTEL-OQL service..."

check_port() {
    PORT=$1
    NAME=$2
    echo -n "   $NAME (port $PORT): "
    if lsof -i :$PORT > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
        return 0
    else
        echo -e "${YELLOW}○${NC} Not running"
        return 1
    fi
}

SERVICE_RUNNING=0
check_port 4317 "OTLP gRPC" && SERVICE_RUNNING=$((SERVICE_RUNNING + 1))
check_port 4318 "OTLP HTTP" && SERVICE_RUNNING=$((SERVICE_RUNNING + 1))
check_port 8080 "Query API" && SERVICE_RUNNING=$((SERVICE_RUNNING + 1))

if [ $SERVICE_RUNNING -eq 0 ]; then
    echo ""
    echo "   ${YELLOW}Service not running${NC}"
    echo "   Run: ./otel-oql --test-mode"
    echo "   Or:  ./otel-oql --config=otel-oql.yaml"
fi

# Summary
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $ERRORS -eq 0 ] && [ $TABLES_OK -eq 3 ]; then
    echo -e "${GREEN}✓ All checks passed!${NC}"
    echo ""
    echo "🎉 OTEL-OQL is ready to use"
    echo ""
    echo "Next steps:"
    echo "  • Clickhouse UI: http://localhost:8123/play"
    if [ $SERVICE_RUNNING -eq 0 ]; then
        echo "  • Start service: ./otel-oql --test-mode"
        echo "                or ./otel-oql --config=otel-oql.yaml"
    fi
    echo "  • Query with CLI: ./oql-cli --tenant-id=0 \"signal=spans limit 10\""
    echo "  • Send test data: ./scripts/insert-test-data.sh"
elif [ $ERRORS -eq 0 ] && [ $TABLES_OK -eq 0 ]; then
    echo -e "${YELLOW}⚠ Setup incomplete${NC}"
    echo ""
    echo "Clickhouse is running but tables not created."
    echo ""
    echo "Run schema setup:"
    echo "  ./otel-oql setup-schema --clickhouse-url=http://localhost:8123"
else
    echo -e "${RED}✗ Issues found (${ERRORS} errors)${NC}"
    echo ""
    echo "Please fix the errors above and try again."
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

exit $ERRORS
