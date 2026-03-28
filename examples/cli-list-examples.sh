#!/bin/bash
# Examples of using the oql-cli list commands

set -e

echo "=== OQL CLI List Commands Examples ==="
echo ""

echo "1. List all available metrics:"
echo "   $ ./oql-cli --tenant-id=0 'list metrics'"
./oql-cli --tenant-id=0 "list metrics" | head -10
echo "   ..."
echo ""

echo "2. List all available labels:"
echo "   $ ./oql-cli --tenant-id=0 'list labels'"
./oql-cli --tenant-id=0 "list labels"
echo ""

echo "3. List values for a specific label:"
echo "   $ ./oql-cli --tenant-id=0 'list values service_name'"
./oql-cli --tenant-id=0 "list values service_name"
echo ""

echo "4. List values for host_name:"
echo "   $ ./oql-cli --tenant-id=0 'list values host_name'"
./oql-cli --tenant-id=0 "list values host_name"
echo ""

echo "=== Interactive Mode ==="
echo "In interactive mode, you can also use these commands:"
echo "  oql> list metrics"
echo "  oql> list labels"
echo "  oql> list values service_name"
echo ""
echo "These commands help you discover what data is available before writing queries."
