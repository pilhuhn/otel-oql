# Migration Guide: LogQL Schema Changes

## Do I Need to Migrate?

**If you're starting fresh**: No migration needed, just run `./otel-oql setup-schema`

**If you have existing data**: Yes, follow this guide.

## What Changed?

The `otel_logs` table schema added 3 new columns:
- `job` (STRING)
- `instance` (STRING)
- `environment` (STRING)

And updated inverted indexes to include these plus `span_id` and `log_level`.

## Impact If You Don't Migrate

LogQL queries using these labels will **FAIL**:
```logql
{job="varlogs"}         # ❌ Error: Column 'job' not found
{instance="pod-1"}      # ❌ Error: Column 'instance' not found
{environment="prod"}    # ❌ Error: Column 'environment' not found
```

## Migration Options

### Option 1: Fresh Start (No Data Loss Concerns)

```bash
# Delete existing tables
curl -X DELETE http://localhost:9000/tables/otel_logs
curl -X DELETE http://localhost:9000/schemas/otel_logs

# Recreate with new schema
./otel-oql setup-schema

# Restart ingestion
```

### Option 2: Pinot Schema Update (Recommended for Production)

Check your Pinot version first:
```bash
curl http://localhost:9000/version
# Pinot 0.10+ supports schema evolution
```

Update schema via Pinot API:
```bash
# Get current schema
curl http://localhost:9000/schemas/otel_logs > current_schema.json

# Edit to add new fields (see schema.json example below)

# Update schema
curl -X PUT http://localhost:9000/schemas/otel_logs \
  -H 'Content-Type: application/json' \
  -d @new_schema.json

# Reload table
curl -X POST http://localhost:9000/tables/otel_logs/reload

# Update table config for new indexes
curl -X PUT http://localhost:9000/tables/otel_logs \
  -H 'Content-Type: application/json' \
  -d @new_table_config.json
```

**schema.json** (add these to dimensionFieldSpecs):
```json
{
  "name": "job",
  "dataType": "STRING"
},
{
  "name": "instance",
  "dataType": "STRING"
},
{
  "name": "environment",
  "dataType": "STRING"
}
```

**table_config.json** (update invertedIndexColumns):
```json
{
  "tableIndexConfig": {
    "invertedIndexColumns": [
      "tenant_id", "trace_id", "span_id", "severity_text",
      "service_name", "log_level", "job", "instance", "environment"
    ]
  }
}
```

**Note**: Existing rows will have NULL values for new columns until you backfill or new data arrives.

### Option 3: Temporary Workaround (No Migration)

If you can't migrate immediately, modify the code to use JSON extraction for all labels:

Edit `pkg/querylangs/common/matcher.go`:

```go
func GetLogNativeColumn(labelName string) string {
    // TEMPORARY: Only use native columns that exist in old schema
    nativeColumns := map[string]string{
        // Only include columns that exist in your current schema
        "trace_id":      "trace_id",
        "traceId":       "trace_id",
        "span_id":       "span_id",
        "spanId":        "span_id",
        "severity":      "severity_text",
        "severity_text": "severity_text",
        "level":         "log_level",
        "log_level":     "log_level",
        "service":       "service_name",
        "service_name":  "service_name",
        "host":          "host_name",
        "host_name":     "host_name",
        "source":        "log_source",
        "log_source":    "log_source",
        // REMOVE these until schema is updated:
        // "job", "instance", "environment"
    }

    if nativeCol, ok := nativeColumns[labelName]; ok {
        return nativeCol
    }
    return ""  // Use JSON extraction
}
```

This makes queries work but **slower** (10-100x) for job/instance/environment labels.

## Updating Ingestion

After schema migration, update your OTLP ingestion to populate new columns.

Check the ingestion service extracts these from OTel resource attributes:
```json
{
  "job": resource.attributes["service.name"] || "unknown",
  "instance": resource.attributes["service.instance.id"] || "unknown",
  "environment": resource.attributes["deployment.environment"] || "production"
}
```

## Verification

After migration, test LogQL queries:

```bash
# Should work without errors
curl -X POST http://localhost:8080/query \
  -H 'X-Tenant-ID: 0' \
  -H 'Content-Type: application/json' \
  -d '{"query": "{job=\"varlogs\"}", "language": "logql"}'

# Check SQL output
# Should show: WHERE job = 'varlogs'
# NOT: WHERE JSON_EXTRACT_SCALAR(attributes, '$.job'...
```

## Rollback

If issues occur:

1. **Stop new ingestion**
2. **Revert code changes** to previous version
3. **Keep old schema** (queries will work with old code)
4. **Debug and retry** migration when ready

## Support

If you encounter issues:
- Check Pinot logs: `docker logs pinot-broker`
- Verify schema: `curl http://localhost:9000/schemas/otel_logs`
- Verify table config: `curl http://localhost:9000/tables/otel_logs`
- Test queries: Use Pinot Query Console (http://localhost:9000)

## Summary

- ✅ **Fresh install**: Just run `setup-schema`
- ⚠️ **Production**: Use Pinot schema update API (Option 2)
- 🔧 **Can't migrate yet**: Use temporary workaround (Option 3)
- ❌ **Don't ignore**: Queries will fail without migration
