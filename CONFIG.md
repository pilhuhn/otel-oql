# OTEL-OQL Configuration

## Configuration Methods

OTEL-OQL can be configured using three methods, with the following priority (highest to lowest):

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Configuration file** (lowest priority)

## Configuration File

### Default Locations

OTEL-OQL automatically looks for configuration files in these locations (in order):

1. `./otel-oql.yaml` (current directory)
2. `~/.otel-oql/config.yaml` (user home directory)
3. `/etc/otel-oql/config.yaml` (system-wide)

### Custom Config File

You can specify a custom configuration file location:

```bash
./otel-oql --config=/path/to/custom-config.yaml
```

### Config File Format

The configuration file uses YAML format:

```yaml
# OTEL-OQL Configuration File

# Pinot configuration
pinot_url: "http://localhost:8000"

# Kafka configuration
kafka_brokers: "localhost:9092"

# OTLP receiver ports
otlp_grpc_port: 4317
otlp_http_port: 4318

# Query API port
query_api_port: 8080

# Test mode (set to true to use tenant-id=0 as default)
test_mode: true
```

## Configuration Options

| Option | Config File | Environment Variable | CLI Flag | Default | Description |
|--------|------------|---------------------|----------|---------|-------------|
| Pinot URL | `pinot_url` | `PINOT_URL` | `--pinot-url` | `http://localhost:9000` | Apache Pinot broker URL |
| Kafka Brokers | `kafka_brokers` | `KAFKA_BROKERS` | `--kafka-brokers` | `localhost:9092` | Kafka broker addresses |
| OTLP gRPC Port | `otlp_grpc_port` | `OTLP_GRPC_PORT` | `--otlp-grpc-port` | `4317` | OTLP gRPC receiver port |
| OTLP HTTP Port | `otlp_http_port` | `OTLP_HTTP_PORT` | `--otlp-http-port` | `4318` | OTLP HTTP receiver port |
| Query API Port | `query_api_port` | `QUERY_API_PORT` | `--query-api-port` | `8080` | Query API server port |
| Test Mode | `test_mode` | `TEST_MODE` | `--test-mode` | `false` | Enable test mode (default tenant-id=0) |

## Usage Examples

### Using Config File Only

Create a `otel-oql.yaml` file in your working directory, then run:

```bash
./otel-oql
```

### Override Specific Settings

Start with config file and override Pinot URL:

```bash
./otel-oql --pinot-url=http://production-pinot:9000
```

### Environment Variables

```bash
export PINOT_URL=http://localhost:8000
export KAFKA_BROKERS=localhost:9092
export TEST_MODE=true
./otel-oql
```

### Mix All Methods

```yaml
# otel-oql.yaml
pinot_url: "http://localhost:8000"
kafka_brokers: "localhost:9092"
test_mode: true
```

```bash
# Override test_mode via environment
export TEST_MODE=false

# Override pinot_url via CLI
./otel-oql --pinot-url=http://production:9000

# Result: Uses production Pinot, localhost Kafka, test_mode=false
```

## Priority Example

Given:
- Config file: `pinot_url: "http://config:9000"`
- Environment: `PINOT_URL=http://env:9000`
- CLI flag: `--pinot-url=http://cli:9000`

Result: Uses `http://cli:9000` (CLI has highest priority)

## Validation

All configuration values are validated on startup. The service will fail to start if:

- Required fields are missing (pinot_url, kafka_brokers)
- Port numbers are invalid (< 1 or > 65535)
- Config file has invalid YAML syntax

## Development vs Production

### Development Setup

```yaml
# otel-oql.yaml (local development)
pinot_url: "http://localhost:8000"
kafka_brokers: "localhost:9092"
test_mode: true  # Allow requests without explicit tenant-id
```

### Production Setup

```yaml
# /etc/otel-oql/config.yaml (production)
pinot_url: "http://pinot-broker.production:8000"
kafka_brokers: "kafka-1:9092,kafka-2:9092,kafka-3:9092"
test_mode: false  # Require explicit tenant-id on all requests
otlp_grpc_port: 4317
otlp_http_port: 4318
query_api_port: 8080
```

## Getting Help

View all available configuration options:

```bash
./otel-oql --help
```
