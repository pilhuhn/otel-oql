# TraceQL Support - Phase 3 Plan

**Status**: 🚧 Planned (Not Started)
**Priority**: Medium (OQL already provides trace querying capabilities)
**Estimated Effort**: 2-3 weeks
**Dependencies**: None (Phases 1 & 2 complete)

## Overview

TraceQL is Grafana Tempo's query language for searching and filtering distributed traces. Adding TraceQL support would provide compatibility with Tempo-based tooling and dashboards.

## Why TraceQL?

**Benefits**:
- Compatibility with Grafana Tempo dashboards
- Familiar syntax for users coming from Tempo
- Standardized trace query language
- Enhanced trace filtering capabilities

**Alternatives**:
- OQL already provides robust trace querying via `signal=spans`
- PromQL can query trace-derived metrics
- LogQL can query trace-correlated logs

## Implementation Challenges

### 1. License Constraint

**Problem**: Grafana Tempo's TraceQL parser is AGPL-licensed
**Solution**: Must implement custom parser from scratch (no code reuse)
**Impact**: Increased development time vs PromQL/LogQL

### 2. Syntax Differences

TraceQL uses significantly different syntax than PromQL/LogQL:

```traceql
# TraceQL (Tempo syntax)
{span.http.status_code = 500 && duration > 100ms}

# vs PromQL/LogQL label matching
{http_status_code="500"}
```

**Key Differences**:
- Dot notation for span attributes (`span.http.method`)
- Structural selectors (`resource.service.name`)
- Intrinsic fields vs labels
- Different aggregation syntax

### 3. Estimated Code Reuse

| Component | Reuse from PromQL/LogQL | Custom Work |
|-----------|------------------------|-------------|
| Lexer/Parser | 0% (AGPL avoidance) | 100% custom |
| Time ranges | ~80% (concept reuse) | Format differences |
| Aggregations | ~60% (concept reuse) | Syntax differences |
| Span selection | 0% (unique to TraceQL) | 100% custom |
| **Overall** | **~30-40%** | **~60-70%** |

## Implementation Plan

### Phase 3a: Parser Development (1 week)

1. **Lexer Implementation**
   - Tokenize TraceQL syntax
   - Handle dot notation (`span.attr.name`)
   - Support intrinsic fields (`duration`, `name`, `status`)

2. **Parser Implementation**
   - Build AST for TraceQL queries
   - Support span selectors: `{span.http.method = "GET"}`
   - Support structural selectors: `{resource.service.name = "api"}`
   - Handle duration comparisons: `{duration > 100ms}`

3. **Unit Tests**
   - 150+ parser tests
   - Cover all selector types
   - Edge cases and error handling

**Deliverables**:
- `pkg/traceql/lexer.go`
- `pkg/traceql/parser.go`
- `pkg/traceql/ast.go`
- `pkg/traceql/parser_test.go` (150 tests)

### Phase 3b: Translation to Pinot SQL (1 week)

1. **Translator Implementation**
   - TraceQL AST → Pinot SQL for `otel_spans` table
   - Map intrinsic fields to native columns
   - Handle dot notation attribute access
   - Generate efficient SQL with tenant isolation

2. **Column Mapping**
   ```go
   // Intrinsic fields → Native columns
   "name"           → "name"
   "duration"       → "duration"
   "status"         → "status_code"
   "kind"           → "kind"

   // Span attributes → Check native columns first
   "span.http.method"      → "http_method" (native)
   "span.http.status_code" → "http_status_code" (native)
   "span.custom.field"     → JSON_EXTRACT(attributes, '$.custom.field')

   // Resource attributes
   "resource.service.name" → "service_name" (native)
   ```

3. **Unit Tests**
   - 150+ translator tests
   - SQL output verification
   - Tenant isolation tests
   - Native vs JSON column usage

**Deliverables**:
- `pkg/traceql/translator.go`
- `pkg/traceql/translator_test.go` (150 tests)

### Phase 3c: API Integration & Testing (3-5 days)

1. **API Routing**
   - Add TraceQL to `pkg/api/server.go`
   - Update `executeTraceQLQuery()` function
   - Language detection: `"language": "traceql"`

2. **Integration Tests**
   - 30+ API-level tests
   - Complex query patterns
   - Error handling
   - Tenant isolation verification

3. **Documentation**
   - TRACEQL_SUPPORT.md (similar to LOGQL_SUPPORT.md)
   - Update README.md
   - Update CHECKPOINT.md
   - Add examples

**Deliverables**:
- `pkg/api/traceql_integration_test.go` (30 tests)
- `TRACEQL_SUPPORT.md`
- Updated documentation

## Test Coverage Goals

Following PromQL/LogQL patterns:

| Test Type | Target | Description |
|-----------|--------|-------------|
| Parser unit tests | 150 | TraceQL syntax parsing |
| Translator unit tests | 150 | SQL generation |
| Integration tests | 30 | API-level tests |
| **Total** | **330** | Comprehensive coverage |

## TraceQL Feature Scope

### Supported (Planned)

**Span Selection**:
- ✅ Intrinsic fields: `{name = "HTTP GET"}`, `{duration > 100ms}`
- ✅ Span attributes: `{span.http.status_code = 500}`
- ✅ Resource attributes: `{resource.service.name = "api"}`
- ✅ Comparison operators: `=`, `!=`, `>`, `<`, `>=`, `<=`
- ✅ Logical operators: `&&`, `||`

**Time Ranges**:
- ✅ Duration literals: `100ms`, `5s`, `1m`
- ✅ Time range filters

**Aggregations**:
- ✅ Count spans: `count()`
- ✅ Group by attributes: `by(span.service.name)`

### Not Supported (Out of Scope)

- ❌ Span sets and unions
- ❌ Advanced metrics functions (rate, quantile over time)
- ❌ Trace-level aggregations (count > 5 spans)
- ❌ Complex structural queries

## Example Queries

### Simple Span Selection

```traceql
# Find all error spans
{status = error}

# Find slow HTTP requests
{span.http.method = "GET" && duration > 1s}

# Find spans from specific service
{resource.service.name = "checkout-service"}
```

### With Filters

```traceql
# HTTP 500 errors in checkout service
{
  resource.service.name = "checkout-service" &&
  span.http.status_code = 500
}

# Slow database queries
{
  span.db.system = "postgresql" &&
  duration > 500ms
}
```

### Translation Example

**TraceQL Input**:
```traceql
{resource.service.name = "api" && span.http.status_code = 500 && duration > 100ms}
```

**Pinot SQL Output**:
```sql
SELECT * FROM otel_spans
WHERE tenant_id = 0
  AND service_name = 'api'
  AND http_status_code = 500
  AND duration > 100000000  -- nanoseconds
ORDER BY timestamp DESC
```

## Dependencies & Tools

**Required**:
- Custom lexer/parser (no external dependencies due to AGPL)
- Potentially use `text/scanner` from Go stdlib
- Or hand-rolled lexer (preferred for control)

**No Reuse**:
- ❌ Grafana Tempo parser (AGPL)
- ❌ Any AGPL-licensed parsing libraries

**Acceptable**:
- ✅ Go stdlib packages
- ✅ Apache 2.0 licensed tools
- ✅ Concepts from PromQL/LogQL implementation

## Success Criteria

1. **Parser**: Successfully parse 95%+ of common TraceQL queries
2. **Translation**: Generate correct Pinot SQL with tenant isolation
3. **Tests**: 330+ tests with 100% pass rate
4. **Performance**: Native column usage for common attributes (same as LogQL)
5. **Documentation**: Complete TRACEQL_SUPPORT.md with examples
6. **Integration**: Seamless API routing alongside OQL/PromQL/LogQL

## Alternative: Enhanced OQL for Traces

Instead of TraceQL, consider enhancing OQL's trace capabilities:

```oql
# OQL already supports robust trace queries
signal=spans where http_status_code == 500 and duration > 100ms

# With trace expansion
signal=spans where name == "checkout" | expand trace

# With correlation
signal=spans where error == true | correlate logs, metrics
```

**Advantages**:
- No additional parser development
- Consistent syntax across all signal types
- Already fully functional
- Better cross-signal correlation

**Disadvantages**:
- Not Tempo-compatible
- Users must learn OQL syntax

## Recommendation

**Priority: Medium-Low**

Since OQL already provides comprehensive trace querying and correlation capabilities, TraceQL implementation should be prioritized **after**:

1. Performance testing and optimization
2. Production hardening
3. Health check endpoints
4. Query caching for expand/correlate operations

TraceQL would be valuable for **Grafana Tempo compatibility** if that becomes a key requirement, but is **not essential** for core functionality.

## References

- [TraceQL Documentation](https://grafana.com/docs/tempo/latest/traceql/)
- [Grafana Tempo GitHub](https://github.com/grafana/tempo) (AGPL - reference only)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
- QUERY_LANGUAGE_ANALYSIS.md - PromQL/LogQL reuse analysis
- LOGQL_SUPPORT.md - Similar implementation pattern
