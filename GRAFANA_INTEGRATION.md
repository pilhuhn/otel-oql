# Grafana Integration Guide

OTEL-OQL provides Prometheus and Loki-compatible API endpoints, enabling seamless integration with Grafana datasources. This allows you to use existing Prometheus and Loki dashboards without modification.

## Overview

OTEL-OQL exposes 4 API endpoints for Grafana compatibility:

**Prometheus Endpoints** (for metrics):
- `GET|POST /api/v1/query` - Instant queries
- `GET|POST /api/v1/query_range` - Range queries

**Loki Endpoints** (for logs):
- `GET|POST /loki/api/v1/query` - Instant log queries
- `GET|POST /loki/api/v1/query_range` - Range log queries

These endpoints follow the official Prometheus and Loki HTTP API specifications, making OTEL-OQL a drop-in replacement datasource for Grafana.

## Datasource Configuration

### Prometheus Datasource (Metrics)

Add OTEL-OQL as a Prometheus datasource in Grafana:

1. **Navigate to**: Configuration → Data Sources → Add data source
2. **Select**: Prometheus
3. **Configure**:
   - **Name**: OTEL-OQL Metrics
   - **URL**: `http://localhost:8080` (or your OTEL-OQL service URL)
   - **Access**: Server (default)
   - **Custom HTTP Headers**:
     - Header: `X-Tenant-ID`
     - Value: `0` (or your tenant ID)

4. **Save & Test**

#### YAML Configuration

```yaml
apiVersion: 1
datasources:
  - name: OTEL-OQL-Metrics
    type: prometheus
    access: proxy
    url: http://localhost:8080
    jsonData:
      httpHeaderName1: X-Tenant-ID
    secureJsonData:
      httpHeaderValue1: "0"
```

### Loki Datasource (Logs)

Add OTEL-OQL as a Loki datasource in Grafana:

1. **Navigate to**: Configuration → Data Sources → Add data source
2. **Select**: Loki
3. **Configure**:
   - **Name**: OTEL-OQL Logs
   - **URL**: `http://localhost:8080` (or your OTEL-OQL service URL)
   - **Access**: Server (default)
   - **Custom HTTP Headers**:
     - Header: `X-Tenant-ID`
     - Value: `0` (or your tenant ID)

4. **Save & Test**

#### YAML Configuration

```yaml
apiVersion: 1
datasources:
  - name: OTEL-OQL-Logs
    type: loki
    access: proxy
    url: http://localhost:8080
    jsonData:
      httpHeaderName1: X-Tenant-ID
    secureJsonData:
      httpHeaderValue1: "0"
```

## Query Examples

### PromQL Queries (Metrics)

Once configured, you can use standard PromQL queries in Grafana:

**Instant Query**:
```promql
http_requests_total{job="api", status_code="200"}
```

**Rate Calculation**:
```promql
rate(http_requests_total{job="api"}[5m])
```

**Aggregation**:
```promql
sum by (service_name) (http_requests_total)
```

**Threshold Filter**:
```promql
http_server_duration > 1000
```

### LogQL Queries (Logs)

**Stream Selector**:
```logql
{job="varlogs", level="error"}
```

**Line Filter**:
```logql
{job="varlogs"} |= "database"
```

**Regex Filter**:
```logql
{job="varlogs"} |~ "error|fail"
```

**Count Over Time**:
```logql
sum by (level) (count_over_time({job="varlogs"}[5m]))
```

**Rate of Logs**:
```logql
rate({job="varlogs"}[5m])
```

## API Endpoint Specifications

### Prometheus API

#### Instant Query: `/api/v1/query`

**Parameters**:
- `query` (string, required) - PromQL expression
- `time` (timestamp, optional) - Evaluation timestamp (default: now)
- `timeout` (duration, optional) - Query timeout

**Example**:
```bash
curl 'http://localhost:8080/api/v1/query' \
  -H 'X-Tenant-ID: 0' \
  --data-urlencode 'query=http_requests_total{job="api"}'
```

**Response Format**:
```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "__name__": "http_requests_total",
          "job": "api",
          "status_code": "200"
        },
        "value": [1710928800, "1234"]
      }
    ]
  }
}
```

#### Range Query: `/api/v1/query_range`

**Parameters**:
- `query` (string, required) - PromQL expression
- `start` (timestamp, required) - Start timestamp
- `end` (timestamp, required) - End timestamp
- `step` (duration, required) - Query resolution step width
- `timeout` (duration, optional) - Query timeout

**Example**:
```bash
curl 'http://localhost:8080/api/v1/query_range' \
  -H 'X-Tenant-ID: 0' \
  --data-urlencode 'query=rate(http_requests_total[5m])' \
  --data-urlencode 'start=2024-03-20T10:00:00Z' \
  --data-urlencode 'end=2024-03-20T11:00:00Z' \
  --data-urlencode 'step=15s'
```

**Response Format**:
```json
{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": {"job": "api"},
        "values": [
          [1710928800, "10.5"],
          [1710928815, "12.3"],
          [1710928830, "11.8"]
        ]
      }
    ]
  }
}
```

### Loki API

#### Instant Query: `/loki/api/v1/query`

**Parameters**:
- `query` (string, required) - LogQL expression
- `time` (timestamp, optional) - Evaluation timestamp (default: now)
- `limit` (int, optional) - Max number of entries (default: 100)
- `direction` (string, optional) - `forward` or `backward` (default: backward)

**Example**:
```bash
curl 'http://localhost:8080/loki/api/v1/query' \
  -H 'X-Tenant-ID: 0' \
  --data-urlencode 'query={job="varlogs", level="error"}' \
  --data-urlencode 'limit=1000'
```

**Response Format** (Log Streams):
```json
{
  "status": "success",
  "data": {
    "resultType": "streams",
    "result": [
      {
        "stream": {
          "job": "varlogs",
          "level": "error"
        },
        "values": [
          ["1710928800000000000", "database connection timeout"],
          ["1710928801000000000", "authentication failed"]
        ]
      }
    ]
  }
}
```

#### Range Query: `/loki/api/v1/query_range`

**Parameters**:
- `query` (string, required) - LogQL expression
- `start` (timestamp, required) - Start timestamp
- `end` (timestamp, required) - End timestamp
- `limit` (int, optional) - Max number of entries
- `step` (duration, optional) - Query resolution (for metric queries)
- `interval` (duration, optional) - Interval for metric queries
- `direction` (string, optional) - `forward` or `backward`

**Example**:
```bash
curl 'http://localhost:8080/loki/api/v1/query_range' \
  -H 'X-Tenant-ID: 0' \
  --data-urlencode 'query=sum by (level) (count_over_time({job="varlogs"}[5m]))' \
  --data-urlencode 'start=1710928800' \
  --data-urlencode 'end=1710932400' \
  --data-urlencode 'step=300'
```

**Response Format** (Metric Query):
```json
{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": {"level": "error"},
        "values": [
          [1710928800, "45"],
          [1710929100, "52"]
        ]
      }
    ]
  }
}
```

## Dashboard Import

You can import existing Prometheus and Loki dashboards directly into Grafana:

1. **Navigate to**: Dashboards → Import
2. **Enter Dashboard ID** (from Grafana.com) or **Upload JSON**
3. **Select Datasource**: Choose your OTEL-OQL datasource
4. **Import**

### Popular Dashboards

**Metrics** (use OTEL-OQL-Metrics datasource):
- [Node Exporter Full](https://grafana.com/grafana/dashboards/1860)
- [Kubernetes Cluster Monitoring](https://grafana.com/grafana/dashboards/7249)
- [Redis Dashboard](https://grafana.com/grafana/dashboards/763)

**Logs** (use OTEL-OQL-Logs datasource):
- [Loki Stack Monitoring](https://grafana.com/grafana/dashboards/14055)
- [Loki Dashboard Quick Search](https://grafana.com/grafana/dashboards/12611)

## Multi-Tenancy in Grafana

OTEL-OQL enforces multi-tenant isolation via the `X-Tenant-ID` header. Each Grafana datasource is scoped to a single tenant.

**For multiple tenants**:

1. Create separate datasources for each tenant:
   - `OTEL-OQL-Metrics-Tenant-1` (X-Tenant-ID: 1)
   - `OTEL-OQL-Metrics-Tenant-2` (X-Tenant-ID: 2)
   - etc.

2. Create tenant-specific folders and dashboards

3. Use Grafana Teams/Organizations to control access

## Troubleshooting

### Connection Error

**Symptom**: "Datasource HTTP error" or "Connection refused"

**Solution**:
- Verify OTEL-OQL is running: `curl http://localhost:8080/api/v1/query`
- Check the URL in datasource configuration
- Ensure network connectivity between Grafana and OTEL-OQL

### Unauthorized Error

**Symptom**: "401 Unauthorized" or "tenant-id not found"

**Solution**:
- Add `X-Tenant-ID` header to datasource configuration
- Verify tenant ID is valid
- If using test mode, tenant-id defaults to 0

### Empty Results

**Symptom**: "No data" or "No log streams found"

**Solution**:
- Verify data is ingested: Check OTLP receivers received data
- Check time range: Ensure query time range matches data timestamps
- Test query directly: Use CLI or curl to test the query
- Check tenant isolation: Ensure datasource tenant-id matches data

### Parse Errors

**Symptom**: "Query parse error" or "bad_data"

**Solution**:
- Verify query syntax is valid PromQL/LogQL
- Check [PromQL documentation](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- Check [LogQL documentation](https://grafana.com/docs/loki/latest/logql/)
- Review OTEL-OQL supported features (some features not yet implemented)

## Supported Features

### PromQL Support

**Supported**:
- ✅ Instant and range vector selectors
- ✅ Label matchers (`=`, `!=`, `=~`, `!~`)
- ✅ Aggregations (`sum`, `avg`, `min`, `max`, `count`)
- ✅ Rate functions (`rate`, `irate`)
- ✅ Value comparisons (`>`, `<`, `>=`, `<=`)

**Not Yet Supported**:
- ❌ Binary operations between metrics
- ❌ Subqueries
- ❌ Advanced functions (`histogram_quantile`, etc.)

### LogQL Support

**Supported**:
- ✅ Stream selectors with label matchers
- ✅ Line filters (`|=`, `!=`, `|~`, `!~`)
- ✅ Label parsers (`| json`, `| logfmt`)
- ✅ Metric queries (`count_over_time`, `rate`, `bytes_over_time`)
- ✅ Aggregations (`sum`, `avg`, `min`, `max`, `count`)

**Not Yet Supported**:
- ❌ Label filters after parsing
- ❌ Unwrap expressions
- ❌ Range aggregations

## Performance Tips

1. **Use Native Columns**: Queries on native columns (job, instance, service_name) are 10-100x faster than JSON extraction

2. **Limit Time Ranges**: Use shorter time ranges for better performance

3. **Aggregation at Source**: Prefer aggregation in queries over fetching all data

4. **Use Label Filters**: Add more label filters to reduce data scanned

## Advanced Configuration

### Custom Timeout

Configure query timeout in Grafana datasource settings:

```yaml
jsonData:
  timeout: 60  # seconds
```

### Query Caching

Grafana supports query result caching. Enable in datasource settings:

```yaml
jsonData:
  cacheLevel: Low  # or Medium, High
```

### Alerting

Grafana alerts work with OTEL-OQL datasources:

1. Create alert rule using PromQL/LogQL query
2. Set threshold and evaluation interval
3. Configure notification channels

## Migration from Prometheus/Loki

Migrating from Prometheus or Loki to OTEL-OQL:

1. **Export Existing Dashboards**: Download JSON from Grafana
2. **Update Datasources**: Change datasource to OTEL-OQL in JSON
3. **Import Updated Dashboards**: Upload modified JSON
4. **Update Alert Rules**: Point alerts to new datasource
5. **Test Queries**: Verify all queries work correctly

**Data Migration**: Use OTLP exporters to send data to OTEL-OQL:
- Prometheus → [Prometheus OTLP Exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/prometheusreceiver)
- Loki → [Loki OTLP Exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/lokiexporter)

## Additional Resources

- [OTEL-OQL README](./README.md) - Service overview
- [PromQL Reference](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [LogQL Reference](https://grafana.com/docs/loki/latest/logql/)
- [Grafana Datasources](https://grafana.com/docs/grafana/latest/datasources/)
- [OpenTelemetry Protocol](https://opentelemetry.io/docs/specs/otlp/)
