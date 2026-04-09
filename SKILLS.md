# SKILLS.md - Observability Debugging with OQL MCP Server

## Overview

This document provides debugging workflows and patterns for LLMs (like Claude) using the OTEL-OQL MCP server to investigate production issues across metrics, logs, and traces.

**MCP Server**: http://localhost:8090
**Available Tools**:
- `oql_query` - Execute OQL queries
- `oql_help` - Get OQL syntax help

## Core Debugging Principle: Signal Hopping

Observability debugging is about **starting at one signal type and hopping to others** to build a complete picture:

```
Metrics (What's broken?) → Traces (Which requests?) → Logs (Why did it fail?)
   ↓                           ↓                         ↓
Aggregated view         Individual events          Detailed context
```

**The Investigation Flow**:
1. **Metrics**: Detect anomalies (latency spikes, error rates, resource usage)
2. **Exemplars**: Bridge from aggregated metrics to specific trace IDs
3. **Traces**: See the request waterfall, identify slow spans
4. **Logs**: Find error messages, stack traces, business context

## Debugging Skills

### Skill 1: Investigating Latency Spikes (RED Metrics → Traces → Logs)

**When to use**: User reports "the app is slow" or dashboard shows latency spike

**Workflow**:

```bash
# Step 1: Find high-latency metrics
signal=metrics 
| where metric_name == "http.server.duration" and value > 2000ms
| where service_name == "checkout-service"

# Step 2: Extract exemplar to find culprit trace
| get_exemplars()

# Step 3: Jump to trace space and expand full waterfall
| expand trace

# Step 4: Find logs for the slow spans
| correlate logs
| where severity_text == "ERROR"
```

**What you're looking for**:
- Which service/endpoint is slow?
- Which span in the trace took the most time?
- Are there error logs correlated with slow spans?
- Database queries? External API calls? Lock contention?

**Alternative PromQL approach** (if using Grafana):
```promql
# Find services with high p95 latency
histogram_quantile(0.95, 
  rate(http_server_duration_bucket[5m])
) > 2
```
Then use exemplars to jump to traces.

---

### Skill 2: Investigating Error Rate Spikes (Metrics → Traces → Logs)

**When to use**: Alert fires for increased error rate

**Workflow**:

```bash
# Step 1: Find error metrics
signal=metrics
| where metric_name == "http.server.request.count" 
| where attributes.status_code =~ "5.."
| where timestamp > now() - 15m

# Step 2: Get exemplar trace IDs
| get_exemplars()
| limit 5

# Step 3: Expand to full traces
| expand trace

# Step 4: Correlate with error logs
| correlate logs
| where severity_text IN ("ERROR", "FATAL")
```

**What you're looking for**:
- Which endpoint is failing?
- What HTTP status codes?
- Error messages in logs (timeout, connection refused, null pointer, etc.)
- Is it a cascading failure (downstream service errors)?

**Alternative LogQL approach** (if you know the service):
```logql
# Find error logs with trace context
{service_name="payment-service", severity_text="ERROR"}
| json
| line_format "{{.message}}"
```

---

### Skill 3: Trace-First Investigation (User Reports Specific Failed Request)

**When to use**: User provides a request ID, order ID, or trace ID

**Workflow**:

```bash
# Step 1: Find spans with user-provided identifier
signal=spans
| where attributes.order_id == "order-12345"
  OR trace_id == "abc123xyz"
| limit 1

# Step 2: Expand to full trace waterfall
| expand trace

# Step 3: Find all logs for this trace
| correlate logs

# Step 4: Filter to errors/warnings
| filter severity_text IN ("ERROR", "WARN")
```

**What you're looking for**:
- Did the trace complete successfully?
- Which span failed (look for status_code != OK)?
- Error messages in logs for that span_id?
- Was there a timeout or dependency failure?

**Alternative TraceQL approach** (Phase 3):
```traceql
{trace_id="abc123xyz"}
```

---

### Skill 4: Log-First Investigation (Error Message in Logs)

**When to use**: Alert from log monitoring or user sees error message

**Workflow**:

```bash
# Step 1: Find error logs
signal=logs
| where severity_text == "ERROR"
| where body =~ ".*database connection timeout.*"
| where timestamp > now() - 1h
| limit 10

# Step 2: Extract trace IDs from logs
| extract trace_id

# Step 3: Switch to trace context
| switch_context signal=spans
| expand trace

# Step 4: Find related metrics
| correlate metrics
| where metric_name == "db.connection.pool.size"
```

**What you're looking for**:
- How often does this error occur?
- Which services are affected?
- Is there a pattern (time-based, user-based, region-based)?
- Are metrics showing resource exhaustion (connection pool, memory, etc.)?

**Alternative LogQL approach**:
```logql
# Count error rate over time
sum by (service_name) (
  count_over_time({severity_text="ERROR"} |= "database connection" [5m])
)
```

---

### Skill 5: Resource Exhaustion Investigation (Metrics → Logs)

**When to use**: High CPU, memory, or connection pool alerts

**Workflow**:

```bash
# Step 1: Find resource metrics
signal=metrics
| where metric_name == "process.runtime.jvm.memory.usage"
| where attributes.pool == "heap"
| where value > 0.9  # >90% heap usage

# Step 2: Find logs during high memory period
| correlate logs
| where severity_text IN ("WARN", "ERROR")
| where body =~ ".*(OutOfMemory|GC|heap).*"

# Step 3: Find traces during this time window
| correlate spans
| where duration > 5000ms  # Slow requests during OOM
```

**What you're looking for**:
- Memory leaks (heap usage climbing)?
- GC pauses causing latency?
- Connection pool exhaustion?
- Large requests/responses?

---

### Skill 6: Dependency Failure Investigation (Cascading Failures)

**When to use**: Service calling downstream service that's failing

**Workflow**:

```bash
# Step 1: Find spans calling the failing service
signal=spans
| where attributes.rpc.service == "payment-service"
| where status_code == "ERROR"
| where timestamp > now() - 10m

# Step 2: Expand to see full trace context
| expand trace

# Step 3: Find logs from BOTH services
| correlate logs
| where service_name IN ("checkout-service", "payment-service")

# Step 4: Check metrics for downstream service
| correlate metrics
| where service_name == "payment-service"
| where metric_name == "http.server.request.count"
```

**What you're looking for**:
- Is the downstream service completely down?
- Specific endpoint failing?
- Timeout vs. connection refused vs. 5xx errors?
- Circuit breaker triggered?

---

### Skill 7: Performance Regression Investigation (Compare Time Periods)

**When to use**: Deployment caused slowdown, need to compare before/after

**Workflow**:

```bash
# Step 1: Find baseline metrics (before deployment)
signal=metrics
| where metric_name == "http.server.duration"
| where service_name == "api-gateway"
| where timestamp BETWEEN "2026-04-08T10:00:00Z" AND "2026-04-08T11:00:00Z"
| summarize avg(value) as baseline

# Step 2: Find current metrics (after deployment)
signal=metrics
| where metric_name == "http.server.duration"
| where service_name == "api-gateway"
| where timestamp > now() - 1h
| summarize avg(value) as current

# Step 3: If current > baseline, find slow traces
signal=metrics
| where metric_name == "http.server.duration"
| where value > baseline_threshold
| get_exemplars()
| expand trace
```

**What you're looking for**:
- Which endpoint regressed?
- New slow database queries?
- Increased payload size?
- N+1 queries introduced?

---

## Advanced Patterns

### Pattern: The Wormhole (Aggregation Space → Event Space)

**Concept**: Metrics show aggregated behavior, but you need the individual events that caused it.

```bash
# Metrics show spike (Aggregation Space)
signal=metrics
| where metric_name == "http.server.duration"
| where value > 5000ms

# Exemplar is the "wormhole key"
| extract exemplar.trace_id as bad_trace

# Jump to Event Space
| switch_context signal=spans
| where trace_id == bad_trace
| expand trace  # Now you see the individual slow request
```

**Why this works**: Exemplars attach trace IDs to metric samples, linking aggregated data back to specific events.

---

### Pattern: Progressive Refinement

**Concept**: Start broad, then narrow down based on findings.

```bash
# Broad search
signal=spans | where duration > 5s

# Refine based on what you see
filter attributes.http.status_code == 500

# Further refine
filter service_name == "checkout-service"

# Expand context
expand trace
correlate logs
```

**When to use**: Exploratory debugging when you don't know the root cause yet.

---

### Pattern: Multi-Tenant Debugging

**Concept**: Isolate issues to specific tenants.

```bash
# Find which tenant is affected
signal=metrics
| where metric_name == "http.server.request.count"
| where attributes.status_code == "500"
| group by tenant_id

# Investigate specific tenant
signal=spans
| where tenant_id == "problematic_tenant"
| where status_code == "ERROR"
| expand trace
| correlate logs
```

**Important**: All queries automatically scope to the authenticated tenant, but you can analyze patterns across your tenant's data.

---

## Query Language Cheat Sheet

### When to Use Each Language

| Language | Best For | Example |
|----------|----------|---------|
| **OQL** | Cross-signal correlation, debugging workflows | `signal=metrics \| get_exemplars() \| expand trace \| correlate logs` |
| **PromQL** | Metrics queries, Grafana dashboards | `rate(http_requests_total[5m]) > 100` |
| **LogQL** | Log searches, text filtering | `{service_name="api"} \|= "error" \| json` |
| **TraceQL** | Trace-specific queries (Phase 3) | `{span.http.status_code >= 500}` |

### Quick OQL Reference

```bash
# Signal selection
signal=metrics | signal=logs | signal=spans

# Filtering
| where condition
| filter additional_condition

# Correlation (magic!)
| correlate logs, metrics     # Find matching signals
| expand trace                # Get full trace waterfall
| get_exemplars()             # Extract trace IDs from metrics

# Context switching
| extract field as variable
| switch_context signal=spans

# Aggregation
| summarize avg(value), max(value)
| group by service_name
| limit 10
```

---

## Common Pitfalls

### Pitfall 1: Starting at the Wrong Signal

**Bad**:
```bash
# Starting with logs when you don't know what you're looking for
signal=logs | where severity_text == "ERROR"  # Too broad!
```

**Good**:
```bash
# Start with metrics to identify the anomaly
signal=metrics | where metric_name == "error_rate" | where value > threshold
| get_exemplars()  # Then jump to specific events
| expand trace
| correlate logs  # Now logs have context
```

**Lesson**: Start with the highest-level view (metrics), then drill down.

---

### Pitfall 2: Ignoring Exemplars

**Bad**:
```bash
# Finding slow metrics but not using exemplars
signal=metrics | where value > 2000ms
# Dead end - you found the aggregate, but not the culprit
```

**Good**:
```bash
signal=metrics | where value > 2000ms
| get_exemplars()  # The wormhole!
| expand trace
```

**Lesson**: Exemplars are the bridge from "what's broken" to "which request broke it".

---

### Pitfall 3: Not Using `expand trace`

**Bad**:
```bash
# Finding a single span
signal=spans | where name == "checkout" | where duration > 5s
# You only see one span, not the full context
```

**Good**:
```bash
signal=spans | where name == "checkout" | where duration > 5s
| expand trace  # See the full waterfall
```

**Lesson**: Individual spans are rarely useful; you need the full trace context.

---

### Pitfall 4: Forgetting Time Ranges

**Bad**:
```bash
# Querying without time bounds
signal=logs | where severity_text == "ERROR"
# Might return millions of rows, slow query
```

**Good**:
```bash
signal=logs 
| where severity_text == "ERROR"
| where timestamp > now() - 1h  # Last hour only
```

**Lesson**: Always scope queries to relevant time ranges.

---

## Debugging Checklist

When investigating an issue, follow this checklist:

- [ ] **Define the problem**: What symptom are you seeing? (latency, errors, crashes?)
- [ ] **Check RED metrics**: Rate, Errors, Duration for affected service
- [ ] **Find anomaly time window**: When did it start? Still happening?
- [ ] **Get exemplar traces**: Use `get_exemplars()` to find specific requests
- [ ] **Expand trace context**: Use `expand trace` to see full waterfall
- [ ] **Correlate logs**: Use `correlate logs` to find error messages
- [ ] **Check dependencies**: Are downstream services healthy?
- [ ] **Look for patterns**: Time-based? User-based? Endpoint-based?
- [ ] **Verify fix**: After mitigation, re-run queries to confirm resolution

---

## Example Debugging Session

**Scenario**: User reports "checkout is slow"

```bash
# 1. Check RED metrics for checkout service
signal=metrics
| where service_name == "checkout-service"
| where metric_name == "http.server.duration"
| where attributes.http.route == "/api/checkout"
| where timestamp > now() - 30m
| summarize avg(value), p95(value), p99(value)

# Result: p95 is 3000ms (normally 200ms) - confirmed spike!

# 2. Get exemplar trace for slow request
signal=metrics
| where service_name == "checkout-service"
| where metric_name == "http.server.duration"
| where value > 2500ms
| get_exemplars()
| limit 1

# Result: exemplar.trace_id = "abc123xyz"

# 3. Expand to full trace
| expand trace

# Result: See waterfall - "payment-service" span took 2800ms!

# 4. Correlate with logs
| correlate logs
| where service_name == "payment-service"
| where severity_text == "ERROR"

# Result: "Database connection pool exhausted" errors

# 5. Check payment-service metrics
signal=metrics
| where service_name == "payment-service"
| where metric_name == "db.connection.pool.active"
| where timestamp > now() - 30m

# Result: Pool at 100% capacity - need to increase pool size!
```

**Root Cause**: Payment service database connection pool is too small, causing checkout requests to wait for available connections.

**Fix**: Increase connection pool size or add connection pooling autoscaling.

---

## Tips for LLMs Using MCP

1. **Always specify tenant-id**: Required for multi-tenant isolation
2. **Start broad, then narrow**: Use metrics → exemplars → traces → logs
3. **Use time ranges**: Prevent slow queries and irrelevant results
4. **Leverage correlation**: Don't manually join signals, use `correlate`
5. **Show your work**: Explain each query step to the user
6. **Interpret results**: Don't just dump data, explain what it means
7. **Suggest next steps**: "Based on these logs, we should check X"
8. **Use multiple languages**: Switch between OQL/PromQL/LogQL as needed

---

## MCP Tool Usage Examples

### Using `oql_query` Tool

```json
{
  "tool": "oql_query",
  "params": {
    "query": "signal=metrics | where metric_name == 'http.server.duration' | where value > 2000ms | get_exemplars()",
    "tenant_id": "0"
  }
}
```

### Using `oql_help` Tool

```json
{
  "tool": "oql_help",
  "params": {
    "topic": "exemplars"
  }
}
```

---

