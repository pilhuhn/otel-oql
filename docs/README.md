# OTEL-OQL Documentation

This directory contains comprehensive documentation for OTEL-OQL. For a quick start, see the [main README](../README.md).

## Directory Structure

### 📡 API Documentation
- [**GRAFANA_INTEGRATION.md**](api/GRAFANA_INTEGRATION.md) - Complete Grafana setup, datasource configuration, and troubleshooting
- [**API_COMPATIBILITY_PROPOSAL.md**](api/API_COMPATIBILITY_PROPOSAL.md) - API design and compatibility considerations

### 🔍 Query Languages
- [**OQL_REFERENCE.md**](query-languages/OQL_REFERENCE.md) - OQL syntax reference and examples
- [**LOGQL_SUPPORT.md**](query-languages/LOGQL_SUPPORT.md) - LogQL implementation details and feature coverage
- [**PROMQL_TESTING.md**](query-languages/PROMQL_TESTING.md) - PromQL testing documentation
- [**PROMQL_METRIC_NAME_CONVERSION.md**](query-languages/PROMQL_METRIC_NAME_CONVERSION.md) - Bidirectional metric name conversion (PromQL ↔ OTel)
- [**QUERY_LANGUAGE_ANALYSIS.md**](query-languages/QUERY_LANGUAGE_ANALYSIS.md) - Parser reuse analysis and commonalities
- [**TRACEQL_PHASE3.md**](query-languages/TRACEQL_PHASE3.md) - TraceQL implementation plan (Phase 3)

### 🏗️ Architecture
- [**SCHEMA.md**](architecture/SCHEMA.md) - Pinot schema documentation (REALTIME tables, native columns)
- [**SCHEMA_CHANGES.md**](architecture/SCHEMA_CHANGES.md) - Schema evolution and migration history
- [**MIGRATION_GUIDE.md**](architecture/MIGRATION_GUIDE.md) - Guide for migrating between schema versions
- [**PINOT_LIMITATIONS.md**](architecture/PINOT_LIMITATIONS.md) - Known Pinot constraints and workarounds

### 🛠️ Development
- [**TESTING.md**](development/TESTING.md) - Testing strategy, test pyramid, and examples
- [**CHECKPOINT.md**](development/CHECKPOINT.md) - Implementation progress and milestones

### 📚 Guides
- [**GETTING_STARTED.md**](guides/GETTING_STARTED.md) - Detailed getting started guide
- [**QUICKSTART.md**](guides/QUICKSTART.md) - Quick setup for impatient users
- [**CONFIG.md**](guides/CONFIG.md) - Configuration reference

## Quick Links

### For Users
- [Quick Start Guide](guides/QUICKSTART.md)
- [Grafana Integration](api/GRAFANA_INTEGRATION.md)
- [OQL Query Examples](query-languages/OQL_REFERENCE.md)

### For Developers
- [Testing Strategy](development/TESTING.md)
- [Schema Documentation](architecture/SCHEMA.md)
- [Development Progress](development/CHECKPOINT.md)

### For Contributors
- AI Assistant Instructions: [CLAUDE.md](../CLAUDE.md) (in project root)
- Project Specification: [SPEC.md](../SPEC.md) (in project root)
