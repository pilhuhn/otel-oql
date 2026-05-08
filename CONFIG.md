# Configuration Guide

OTEL-OQL can be configured via command-line flags, environment variables, or a YAML configuration file.

## Configuration Sources

Configuration is loaded in the following priority order (highest to lowest):

1. Command-line flags
2. Environment variables
3. Configuration file (`otel-oql.yaml`)
4. Default values

## All Configuration Options

### Operating Mode

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--mode` | `OTEL_OQL_MODE` | `all` | Operating mode: `all`, `ingestion`, or `query` |

**Operating Modes**:
- `all`: Run ingestion and query components together (default)
- `ingestion`: Run only OTLP receivers and Kafka ingestion
- `query`: Run only Query API and MCP server

See [docs/SPLIT_DEPLOYMENT.md](docs/SPLIT_DEPLOYMENT.md) for deployment strategies.

### Service Ports

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--otlp-grpc-port` | `OTLP_GRPC_PORT` | `4317` | OTLP gRPC receiver port |
| `--otlp-http-port` | `OTLP_HTTP_PORT` | `4318` | OTLP HTTP receiver port |
| `--query-api-port` | `QUERY_API_PORT` | `8080` | Query API server port |
| `--mcp-port` | `MCP_PORT` | `8090` | MCP server port |

### Backend Services

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--pinot-url` | `PINOT_URL` | `http://localhost:9000` | Apache Pinot broker URL |
| `--kafka-brokers` | `KAFKA_BROKERS` | `localhost:9092` | Kafka broker addresses (comma-separated) |

### Authentication

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--users-file` | `USERS_FILE` | `./users.csv` | Path to user-to-tenant mapping file |
| `--api-keys-file` | `API_KEYS_FILE` | `./api-keys.csv` | Path to API keys file |
| `--test-mode` | `TEST_MODE` | `false` | Enable test mode (relaxed auth, tenant-id=0 default) |

**Authentication Behavior**:
- **Production mode** (`--test-mode=false`): Requires API key authentication via `Authorization: Bearer <key>` header
- **Test mode** (`--test-mode=true`): Falls back to `tenant-id` header if no authentication provided
- If user files don't exist in test mode: Uses old-style tenant-id header authentication
- If user files exist: Authentication is enforced (production) or optional (test mode)

See [USER_MANAGEMENT.md](USER_MANAGEMENT.md) for complete authentication documentation.

### Observability (Self-Instrumentation)

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--observability-enabled` | `OBSERVABILITY_ENABLED` | `false` | Enable self-observability |
| `--observability-endpoint` | `OBSERVABILITY_ENDPOINT` | `localhost:4317` | OTLP endpoint for self-observability |
| `--observability-tenant-id` | `OBSERVABILITY_TENANT_ID` | `0` | Tenant ID for self-observability data |

**Note**: The service can send its own telemetry to itself or to an external OTLP endpoint.

### Debugging

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--debug-query` | `DEBUG_QUERY` | `false` | Enable query debugging (log all queries) |
| `--debug-translation` | `DEBUG_TRANSLATION` | `false` | Enable translation debugging (log SQL generation) |
| `--debug-ingestion` | `DEBUG_INGESTION` | `false` | Enable ingestion debugging (log OTLP data) |
| `--exit-on-failure` | `EXIT_ON_FAILURE` | `false` | Exit immediately if startup validation fails |

## Configuration File

Create `otel-oql.yaml` in the current directory:

```yaml
# Operating mode
mode: all  # all, ingestion, or query

# Service ports
otlp_grpc_port: 4317
otlp_http_port: 4318
query_api_port: 8080
mcp_port: 8090

# Backend services
pinot_url: http://localhost:9000
kafka_brokers: localhost:9092

# Authentication
test_mode: true
users_file: ./users.csv
api_keys_file: ./api-keys.csv

# Observability
observability_enabled: false
observability_endpoint: localhost:4317
observability_tenant_id: "0"

# Debugging
debug_query: false
debug_translation: false
debug_ingestion: false
exit_on_failure: false
```

## Examples

### Development Setup

```bash
# Option 1: Command-line flags
./otel-oql --test-mode \
  --pinot-url=http://localhost:9000 \
  --kafka-brokers=localhost:9092 \
  --debug-query

# Option 2: Environment variables
export TEST_MODE=true
export PINOT_URL=http://localhost:9000
export KAFKA_BROKERS=localhost:9092
export DEBUG_QUERY=true
./otel-oql

# Option 3: Configuration file
cat > otel-oql.yaml <<EOF
mode: all
test_mode: true
pinot_url: http://localhost:9000
kafka_brokers: localhost:9092
debug_query: true
EOF
./otel-oql
```

### Production Setup with Authentication

```bash
# Create user files
cat > users.csv <<EOF
username,tenant_id
prod_user_1,100
prod_user_2,200
EOF

cat > api-keys.csv <<EOF
username,api_key
prod_user_1,$(openssl rand -hex 16)
prod_user_2,$(openssl rand -hex 16)
EOF

# Secure the files
chmod 600 users.csv api-keys.csv

# Start service
./otel-oql \
  --mode=all \
  --users-file=/etc/otel-oql/users.csv \
  --api-keys-file=/etc/otel-oql/api-keys.csv \
  --pinot-url=http://pinot:9000 \
  --kafka-brokers=kafka-1:9092,kafka-2:9092,kafka-3:9092 \
  --observability-enabled \
  --observability-endpoint=otel-collector:4317
```

### Split Deployment

**Ingestion Service**:
```bash
# Only ingests OTLP data, doesn't need Pinot
./otel-oql \
  --mode=ingestion \
  --kafka-brokers=kafka:9092 \
  --otlp-grpc-port=4317 \
  --otlp-http-port=4318 \
  --users-file=/etc/otel-oql/users.csv \
  --api-keys-file=/etc/otel-oql/api-keys.csv
```

**Query Service**:
```bash
# Only serves queries, doesn't accept OTLP data
./otel-oql \
  --mode=query \
  --pinot-url=http://pinot:9000 \
  --query-api-port=8080 \
  --mcp-port=8090 \
  --users-file=/etc/otel-oql/users.csv \
  --api-keys-file=/etc/otel-oql/api-keys.csv
```

## Environment-Specific Configurations

### Local Development

```yaml
mode: all
test_mode: true
debug_query: true
debug_translation: true
pinot_url: http://localhost:9000
kafka_brokers: localhost:9092
```

### Staging

```yaml
mode: all
test_mode: false
users_file: /etc/otel-oql/users.csv
api_keys_file: /etc/otel-oql/api-keys.csv
pinot_url: http://pinot-staging:9000
kafka_brokers: kafka-staging-1:9092,kafka-staging-2:9092
observability_enabled: true
observability_endpoint: otel-collector:4317
```

### Production

```yaml
mode: all
test_mode: false
users_file: /etc/otel-oql/users.csv
api_keys_file: /etc/otel-oql/api-keys.csv
pinot_url: http://pinot-prod:9000
kafka_brokers: kafka-1:9092,kafka-2:9092,kafka-3:9092
observability_enabled: true
observability_endpoint: otel-collector:4317
observability_tenant_id: "1000"
exit_on_failure: true
```

## Validation

The service validates configuration at startup:

1. **Dependency Checks** (if `exit_on_failure=true`):
   - Kafka connectivity
   - Pinot availability
   - Required tables exist

2. **Authentication** (if user files specified):
   - Files exist and are readable
   - CSV format is valid
   - No duplicate usernames or API keys

3. **Port Availability**:
   - Ports are not already in use

## Troubleshooting

### "users file not found" Error

**Problem**: Service fails to start with file not found error.

**Solutions**:
- Create `users.csv` and `api-keys.csv` in the current directory, OR
- Specify full paths: `--users-file=/path/to/users.csv`, OR
- Enable test mode: `--test-mode`

### "failed to connect to Kafka" Error

**Problem**: Cannot reach Kafka brokers.

**Solutions**:
- Verify Kafka is running: `podman-compose ps`
- Check broker address: `--kafka-brokers=localhost:9092`
- For split deployment: Ensure ingestion service can reach Kafka

### "failed to query Pinot" Error

**Problem**: Cannot reach Pinot broker.

**Solutions**:
- Verify Pinot is running: `curl http://localhost:9000/health`
- Check broker URL: `--pinot-url=http://localhost:9000`
- For split deployment: Ensure query service can reach Pinot

### Port Already in Use

**Problem**: `bind: address already in use`

**Solutions**:
- Check which process is using the port: `lsof -i :4317`
- Use different ports: `--otlp-grpc-port=14317`
- Stop conflicting service

## See Also

- [USER_MANAGEMENT.md](USER_MANAGEMENT.md) - Authentication and user management
- [README.md](README.md) - Quick start guide
- [CLAUDE.md](CLAUDE.md) - Architecture documentation
- [docs/SPLIT_DEPLOYMENT.md](docs/SPLIT_DEPLOYMENT.md) - Deployment strategies
