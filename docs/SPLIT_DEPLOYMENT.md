# Split Deployment Guide

OTEL-OQL supports three operating modes, allowing you to deploy ingestion and query components independently for better scalability, security, and fault isolation.

## Operating Modes

### 1. All-in-One Mode (default)

**Mode**: `all`

Runs both ingestion and query components in a single process. Suitable for:
- Local development
- Small deployments
- Testing environments

```bash
./otel-oql
# or explicitly:
./otel-oql --mode=all
```

### 2. Ingestion Mode

**Mode**: `ingestion`

Runs only OTLP receivers and Kafka ingestion. Does **not** require Pinot connection.

**Components**:
- ✅ OTLP gRPC receiver (port 4317)
- ✅ OTLP HTTP receiver (port 4318)
- ✅ Kafka producer
- ❌ Query API (disabled)
- ❌ MCP server (disabled)

**Required Configuration**:
- `KAFKA_BROKERS` (required)
- `OTLP_GRPC_PORT` (default: 4317)
- `OTLP_HTTP_PORT` (default: 4318)

**Not Required**:
- `PINOT_URL` (not used)

```bash
./otel-oql --mode=ingestion --kafka-brokers=kafka:9092

# Or with environment variable:
export OTEL_OQL_MODE=ingestion
./otel-oql
```

### 3. Query Mode

**Mode**: `query`

Runs only query API and MCP server. Does **not** accept OTLP data.

**Components**:
- ✅ Query API server (port 8080)
- ✅ MCP server (port 8090)
- ✅ Pinot client
- ❌ OTLP receivers (disabled)
- ❌ Kafka producer (disabled)

**Required Configuration**:
- `PINOT_URL` (required)
- `QUERY_API_PORT` (default: 8080)
- `MCP_PORT` (default: 8090)

**Not Required**:
- `KAFKA_BROKERS` (not used)

```bash
./otel-oql --mode=query --pinot-url=http://pinot-broker:9000

# Or with environment variable:
export OTEL_OQL_MODE=query
./otel-oql
```

---

## Configuration

### Command-Line Flag

```bash
./otel-oql --mode=<mode>
```

### Environment Variable

```bash
export OTEL_OQL_MODE=<mode>
./otel-oql
```

### Configuration File

```yaml
# otel-oql.yaml
mode: ingestion  # or "query" or "all"
kafka_brokers: "kafka:9092"
otlp_grpc_port: 4317
otlp_http_port: 4318
```

### Priority

Highest to lowest:
1. Command-line flag (`--mode`)
2. Environment variable (`OTEL_OQL_MODE`)
3. Configuration file (`mode:`)
4. Default (`all`)

---

## Deployment Examples

### Docker Compose

```yaml
services:
  # Ingestion instances (scale horizontally for high load)
  otel-oql-ingestion:
    image: otel-oql:latest
    environment:
      OTEL_OQL_MODE: ingestion
      KAFKA_BROKERS: kafka:9092
      TEST_MODE: "false"
    ports:
      - "4317:4317"
      - "4318:4318"
    deploy:
      replicas: 3  # Scale based on ingestion load
    depends_on:
      - kafka

  # Query instances (scale based on query load)
  otel-oql-query:
    image: otel-oql:latest
    environment:
      OTEL_OQL_MODE: query
      PINOT_URL: http://pinot-broker:9000
      TEST_MODE: "false"
    ports:
      - "8080:8080"
      - "8090:8090"
    deploy:
      replicas: 2  # Scale based on query load
    depends_on:
      - pinot

  # Infrastructure (shared)
  kafka:
    image: docker.io/bitnami/kafka:latest
    # ... kafka config ...

  pinot:
    # ... pinot config ...
```

### Kubernetes

```yaml
---
# Ingestion Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-oql-ingestion
  namespace: observability
spec:
  replicas: 3
  selector:
    matchLabels:
      app: otel-oql
      component: ingestion
  template:
    metadata:
      labels:
        app: otel-oql
        component: ingestion
    spec:
      containers:
      - name: otel-oql
        image: otel-oql:latest
        env:
        - name: OTEL_OQL_MODE
          value: "ingestion"
        - name: KAFKA_BROKERS
          value: "kafka-service:9092"
        - name: TEST_MODE
          value: "false"
        ports:
        - name: otlp-grpc
          containerPort: 4317
        - name: otlp-http
          containerPort: 4318
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi

---
# Ingestion Service (LoadBalancer for external traffic)
apiVersion: v1
kind: Service
metadata:
  name: otel-oql-ingestion
  namespace: observability
spec:
  type: LoadBalancer
  selector:
    app: otel-oql
    component: ingestion
  ports:
  - name: otlp-grpc
    port: 4317
    targetPort: 4317
  - name: otlp-http
    port: 4318
    targetPort: 4318

---
# Query Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-oql-query
  namespace: observability
spec:
  replicas: 2
  selector:
    matchLabels:
      app: otel-oql
      component: query
  template:
    metadata:
      labels:
        app: otel-oql
        component: query
    spec:
      containers:
      - name: otel-oql
        image: otel-oql:latest
        env:
        - name: OTEL_OQL_MODE
          value: "query"
        - name: PINOT_URL
          value: "http://pinot-broker:9000"
        - name: TEST_MODE
          value: "false"
        ports:
        - name: query-api
          containerPort: 8080
        - name: mcp
          containerPort: 8090
        resources:
          requests:
            cpu: 500m
            memory: 1Gi
          limits:
            cpu: 2000m
            memory: 4Gi

---
# Query Service (Internal only)
apiVersion: v1
kind: Service
metadata:
  name: otel-oql-query
  namespace: observability
spec:
  type: ClusterIP
  selector:
    app: otel-oql
    component: query
  ports:
  - name: query-api
    port: 8080
    targetPort: 8080
  - name: mcp
    port: 8090
    targetPort: 8090
```

### Horizontal Pod Autoscaler (HPA)

Scale ingestion based on CPU/memory:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: otel-oql-ingestion-hpa
  namespace: observability
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: otel-oql-ingestion
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

Scale query based on request rate:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: otel-oql-query-hpa
  namespace: observability
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: otel-oql-query
  minReplicas: 2
  maxReplicas: 5
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60
```

---

## Architecture

### Data Flow

```
External OTLP
    │
    ▼
┌─────────────────────────┐
│  Ingestion Instances    │  (mode=ingestion)
│  - OTLP gRPC/HTTP       │  Replicas: 3+
│  - Tenant validation    │
│  - Kafka producer       │
└────────┬────────────────┘
         │
         ▼
    Kafka Topics
  (otel-spans/metrics/logs)
         │
         ▼
    Apache Pinot
  (REALTIME tables)
         │
         ▼
┌─────────────────────────┐
│   Query Instances       │  (mode=query)
│  - Query API (8080)     │  Replicas: 2+
│  - MCP server (8090)    │
│  - OQL/PromQL/LogQL     │
└─────────────────────────┘
         │
         ▼
    API Responses
```

### Component Isolation

| Component | Ingestion Mode | Query Mode | All Mode |
|-----------|----------------|------------|----------|
| OTLP gRPC receiver | ✅ | ❌ | ✅ |
| OTLP HTTP receiver | ✅ | ❌ | ✅ |
| Kafka producer | ✅ | ❌ | ✅ |
| Pinot client | ❌ | ✅ | ✅ |
| Query API | ❌ | ✅ | ✅ |
| MCP server | ❌ | ✅ | ✅ |

---

## Benefits

### 1. Independent Scaling

Scale ingestion and query independently based on load:

```bash
# High ingestion load → scale ingestion
kubectl scale deployment otel-oql-ingestion --replicas=10

# High query load → scale query
kubectl scale deployment otel-oql-query --replicas=5
```

### 2. Resource Isolation

- **Ingestion**: CPU-bound (OTLP parsing, protobuf decoding)
- **Query**: Memory/I/O-bound (query planning, Pinot queries)

Optimize resource allocation per component:
- Ingestion: More CPU, less memory
- Query: More memory, moderate CPU

### 3. Security Boundaries

- **Ingestion**: Deploy in DMZ or edge network (accepts external traffic)
- **Query**: Deploy in private zone (internal-only access)

### 4. Fault Isolation

- Query instance OOM → Ingestion continues
- Ingestion backpressure → Queries unaffected
- Query API bug → OTLP data still ingested

### 5. Deployment Flexibility

- **Development**: Use `mode=all` (single process)
- **Production**: Split for scalability
- **Edge locations**: Ingestion-only instances → central query cluster

---

## Performance Considerations

### Ingestion Instance Sizing

**Typical profile**:
- CPU: 500m - 2000m per instance
- Memory: 512Mi - 2Gi per instance
- Disk: Minimal (no persistence)

**Scaling triggers**:
- CPU > 70%
- Kafka producer lag
- OTLP request latency

### Query Instance Sizing

**Typical profile**:
- CPU: 500m - 2000m per instance
- Memory: 1Gi - 4Gi per instance
- Disk: Minimal (no caching yet)

**Scaling triggers**:
- CPU > 60%
- Query latency p99 > threshold
- Request queue depth

---

## Monitoring

### Ingestion Metrics

Monitor with `observability_enabled=true`:

```bash
./otel-oql --mode=ingestion \
  --observability-enabled \
  --observability-endpoint=otel-collector:4317
```

**Key metrics**:
- `otel_oql_spans_received_total`
- `otel_oql_metrics_received_total`
- `otel_oql_logs_received_total`
- `otel_oql_kafka_produce_errors_total`

### Query Metrics

```bash
./otel-oql --mode=query \
  --observability-enabled \
  --observability-endpoint=otel-collector:4317
```

**Key metrics**:
- `otel_oql_queries_total`
- `otel_oql_query_duration_seconds`
- `otel_oql_pinot_errors_total`

---

## Migration Guide

### From All-in-One to Split

**Step 1**: Deploy query instances

```bash
kubectl apply -f query-deployment.yaml
# Wait for healthy
kubectl wait --for=condition=ready pod -l component=query
```

**Step 2**: Verify queries work

```bash
curl http://otel-oql-query:8080/query \
  -H 'Content-Type: application/json' \
  -d '{"query": "signal=spans | limit 1"}'
```

**Step 3**: Deploy ingestion instances

```bash
kubectl apply -f ingestion-deployment.yaml
# Wait for healthy
kubectl wait --for=condition=ready pod -l component=ingestion
```

**Step 4**: Switch OTLP traffic

```bash
# Update application OTLP endpoint from:
#   otel-oql:4317
# To:
#   otel-oql-ingestion:4317
```

**Step 5**: Decommission all-in-one instances

```bash
kubectl delete deployment otel-oql-all
```

---

## Troubleshooting

### Ingestion instance won't start

**Error**: `kafka-brokers is required for ingestion mode`

**Solution**: Set `KAFKA_BROKERS` environment variable or `--kafka-brokers` flag

### Query instance won't start

**Error**: `pinot-url is required for query mode`

**Solution**: Set `PINOT_URL` environment variable or `--pinot-url` flag

### Mode not recognized

**Error**: `invalid mode: xyz (must be: all, ingestion, query)`

**Solution**: Use one of the three valid modes: `all`, `ingestion`, or `query`

---

## Best Practices

1. **Always use split deployment in production**
   - Better scalability
   - Fault isolation
   - Security boundaries

2. **Use health checks**
   ```yaml
   livenessProbe:
     httpGet:
       path: /health  # (future feature)
       port: 8080
   ```

3. **Monitor both components**
   - Enable observability on both ingestion and query
   - Use separate tenant IDs if needed

4. **Size appropriately**
   - Start with 3 ingestion, 2 query instances
   - Scale based on actual load

5. **Network policies**
   - Ingestion: Allow external OTLP traffic
   - Query: Internal-only access

6. **Use service mesh** (optional)
   - Mutual TLS between components
   - Circuit breakers
   - Retry policies

---

## Summary

| Feature | Ingestion Mode | Query Mode | All Mode |
|---------|----------------|------------|----------|
| **OTLP Receivers** | ✅ | ❌ | ✅ |
| **Kafka** | ✅ | ❌ | ✅ |
| **Pinot** | ❌ | ✅ | ✅ |
| **Query API** | ❌ | ✅ | ✅ |
| **MCP Server** | ❌ | ✅ | ✅ |
| **Use Case** | Data ingestion | Query serving | Development |
| **Scaling** | Horizontal | Horizontal | Vertical |

Split deployment enables production-grade scalability, security, and fault isolation for OTEL-OQL.
