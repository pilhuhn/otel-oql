package clickhouse

import (
	"context"
	"fmt"
)

// tableDDLs holds the ordered list of CREATE TABLE statements to execute during schema setup.
var tableDDLs = []struct {
	name string
	ddl  string
}{
	{
		name: "otel_spans",
		ddl: `CREATE TABLE IF NOT EXISTS otel_spans (
    tenant_id         Int64,
    trace_id          String,
    span_id           String,
    parent_span_id    String,
    name              String,
    kind              String,
    duration          Int64,
    timestamp         Int64,
    status_code       String,
    status_message    String,
    service_name      String,
    http_method       String,
    http_status_code  Nullable(Int32),
    http_route        String,
    http_target       String,
    db_system         String,
    db_statement      String,
    messaging_system        String,
    messaging_destination   String,
    rpc_service       String,
    rpc_method        String,
    error             Bool,
    attributes        String,
    resource_attributes String
) ENGINE = MergeTree()
ORDER BY (tenant_id, timestamp, trace_id)`,
	},
	{
		name: "otel_metrics",
		ddl: `CREATE TABLE IF NOT EXISTS otel_metrics (
    tenant_id         Int64,
    metric_name       String,
    metric_type       String,
    timestamp         Int64,
    value             Float64,
    count             UInt64,
    sum               Float64,
    service_name      String,
    host_name         String,
    environment       String,
    job               String,
    instance          String,
    exemplar_trace_id String,
    exemplar_span_id  String,
    attributes        String,
    resource_attributes String
) ENGINE = MergeTree()
ORDER BY (tenant_id, metric_name, timestamp)`,
	},
	{
		name: "otel_logs",
		ddl: `CREATE TABLE IF NOT EXISTS otel_logs (
    tenant_id         Int64,
    timestamp         Int64,
    trace_id          String,
    span_id           String,
    severity_number   Int32,
    severity_text     String,
    body              String,
    service_name      String,
    host_name         String,
    log_level         String,
    log_source        String,
    job               String,
    instance          String,
    environment       String,
    attributes        String,
    resource_attributes String
) ENGINE = MergeTree()
ORDER BY (tenant_id, timestamp)`,
	},
}

// SetupSchema creates all required Clickhouse tables if they do not already exist.
func SetupSchema(ctx context.Context, client *Client) error {
	for _, t := range tableDDLs {
		fmt.Printf("Creating table %s...\n", t.name)
		if err := client.CreateTable(ctx, t.ddl); err != nil {
			return fmt.Errorf("failed to create table %s: %w", t.name, err)
		}
		fmt.Printf("  ✓ %s\n", t.name)
	}
	return nil
}
