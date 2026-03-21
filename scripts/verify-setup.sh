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

# Check Pinot container
echo -n "📦 Pinot container exists: "
if podman ps -a --format '{{.Names}}' | grep -q '^pinot-quickstart$'; then
    echo -e "${GREEN}✓${NC}"

    echo -n "▶️  Pinot container running: "
    if podman ps --format '{{.Names}}' | grep -q '^pinot-quickstart$'; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC} Container exists but is stopped"
        echo "   Run: podman start pinot-quickstart"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo -e "${RED}✗${NC} Pinot container not found"
    echo "   Run: ./scripts/start-pinot.sh"
    ERRORS=$((ERRORS + 1))
fi

# Check Pinot health
echo -n "🏥 Pinot health: "
if curl -s http://localhost:9000/health 2>/dev/null | grep -q "OK"; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC} Pinot not responding on http://localhost:9000"
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

# Check Pinot tables
echo ""
echo "📊 Checking Pinot tables..."

check_table() {
    TABLE_NAME=$1
    echo -n "   $TABLE_NAME: "
    if curl -s http://localhost:9000/tables 2>/dev/null | grep -q "\"$TABLE_NAME\""; then
        echo -e "${GREEN}✓${NC}"
        return 0
    else
        echo -e "${RED}✗${NC} Not found"
        return 1
    fi
}

TABLES_OK=0
if curl -s http://localhost:9000/health 2>/dev/null | grep -q "OK"; then
    check_table "otel_spans" && TABLES_OK=$((TABLES_OK + 1))
    check_table "otel_metrics" && TABLES_OK=$((TABLES_OK + 1))
    check_table "otel_logs" && TABLES_OK=$((TABLES_OK + 1))

    if [ $TABLES_OK -eq 0 ]; then
        echo ""
        echo "   ${YELLOW}No tables found${NC}"
        echo "   Run: ./otel-oql setup-schema --pinot-url=http://localhost:9000"
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
    echo "   Run: ./otel-oql --test-mode --pinot-url=http://localhost:9000"
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
    echo "  • Open Pinot UI: http://localhost:9000"
    if [ $SERVICE_RUNNING -eq 0 ]; then
        echo "  • Start service: ./otel-oql --test-mode --pinot-url=http://localhost:9000"
    fi
    echo "  • Send test data or run OQL queries"
elif [ $ERRORS -eq 0 ] && [ $TABLES_OK -eq 0 ]; then
    echo -e "${YELLOW}⚠ Setup incomplete${NC}"
    echo ""
    echo "Pinot is running but tables not created."
    echo ""
    echo "Run schema setup:"
    echo "  ./otel-oql setup-schema --pinot-url=http://localhost:9000"
else
    echo -e "${RED}✗ Issues found (${ERRORS} errors)${NC}"
    echo ""
    echo "Please fix the errors above and try again."
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

exit $ERRORS
