#!/bin/bash
set -e

echo "📊 Perses Datasource Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Check if perses-support directory exists
if [ ! -d "perses-support" ]; then
    echo "❌ perses-support directory not found"
    echo "Please run this script from the project root directory"
    exit 1
fi

# Check if template files exist
if [ ! -f "perses-support/otel-oql-metrics.json.template" ]; then
    echo "❌ Template files not found in perses-support/"
    exit 1
fi

# Create output directory if it doesn't exist
mkdir -p data/perses/globaldatasources

# Ask for endpoint
echo "Enter OTEL-OQL endpoint (e.g., localhost, 192.168.1.10, or localhost:8080):"
read -r endpoint

# Trim whitespace
endpoint=$(echo "$endpoint" | xargs)

# Check if endpoint is empty
if [ -z "$endpoint" ]; then
    echo "❌ Endpoint cannot be empty"
    exit 1
fi

# Parse endpoint and add port if missing
if [[ "$endpoint" =~ ^https?:// ]]; then
    # Remove http:// or https:// prefix
    endpoint="${endpoint#http://}"
    endpoint="${endpoint#https://}"
fi

# Check if port is included
if [[ "$endpoint" =~ :[0-9]+$ ]]; then
    # Port already specified
    otel_oql_url="http://$endpoint"
else
    # Add default port 8080
    otel_oql_url="http://$endpoint:8080"
fi

echo ""
echo "Using OTEL-OQL endpoint: $otel_oql_url"
echo ""

# Process each template
templates=(
    "otel-oql-metrics.json.template"
    "otel-oql-logs.json.template"
    "oql-otel-trace.json.template"
)

datasource_count=0
updated_count=0
created_count=0

for template in "${templates[@]}"; do
    template_path="perses-support/$template"
    output_file="data/perses/globaldatasources/${template%.template}"

    if [ ! -f "$template_path" ]; then
        echo "⚠️  Template not found: $template_path"
        continue
    fi

    # Check if datasource already exists
    if [ -f "$output_file" ]; then
        echo "🔄 Updating existing datasource: ${template%.template}"
        updated_count=$((updated_count + 1))
    else
        echo "✨ Creating new datasource: ${template%.template}"
        created_count=$((created_count + 1))
    fi

    # Replace placeholder with actual endpoint
    sed "s|{{OTEL_OQL_ENDPOINT}}|$otel_oql_url|g" "$template_path" > "$output_file"

    datasource_count=$((datasource_count + 1))
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Perses datasources configured!"
echo ""
echo "Summary:"
echo "  • Total datasources: $datasource_count"
echo "  • Created: $created_count"
echo "  • Updated: $updated_count"
echo "  • Endpoint: $otel_oql_url"
echo ""
echo "Datasources created in: data/perses/globaldatasources/"
echo ""
echo "The datasources will be automatically loaded when Perses starts."
echo "If Perses is already running, you may need to restart it:"
echo "  podman compose restart perses"
echo ""
echo "Access Perses UI at: http://localhost:8082"
echo ""
