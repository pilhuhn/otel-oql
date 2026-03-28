package pinot

import (
	"context"
	"fmt"
)

// PinotSchema represents a complete Pinot schema definition
type PinotSchema struct {
	SchemaName             string                `json:"schemaName"`
	DimensionFieldSpecs    []FieldSpec           `json:"dimensionFieldSpecs"`
	MetricFieldSpecs       []FieldSpec           `json:"metricFieldSpecs,omitempty"`
	DateTimeFieldSpecs     []DateTimeFieldSpec   `json:"dateTimeFieldSpecs,omitempty"`
	PrimaryKeyColumns      []string              `json:"primaryKeyColumns,omitempty"`
}

// FieldSpec represents a field specification
type FieldSpec struct {
	Name             string      `json:"name"`
	DataType         string      `json:"dataType"`
	DefaultNullValue interface{} `json:"defaultNullValue,omitempty"`
}

// DateTimeFieldSpec represents a datetime field specification
type DateTimeFieldSpec struct {
	Name        string `json:"name"`
	DataType    string `json:"dataType"`
	Format      string `json:"format"`
	Granularity string `json:"granularity"`
}

// TableConfig represents a Pinot table configuration
type TableConfig struct {
	TableName        string                 `json:"tableName"`
	TableType        string                 `json:"tableType"`
	Segmentation     *SegmentationConfig    `json:"segmentsConfig,omitempty"`
	Tenants          *TenantsConfig         `json:"tenants"`
	TableIndexConfig *TableIndexConfig      `json:"tableIndexConfig"`
	Metadata         *MetadataConfig        `json:"metadata,omitempty"`
	Routing          *RoutingConfig         `json:"routing,omitempty"`
}

// SegmentationConfig represents segmentation configuration
type SegmentationConfig struct {
	TimeColumnName          string                        `json:"timeColumnName,omitempty"` // Required for REALTIME tables
	TimeType                string                        `json:"timeType,omitempty"`
	Replication             string                        `json:"replication,omitempty"`
	SegmentPushType         string                        `json:"segmentPushType,omitempty"`
	SegmentPartitionConfig  *SegmentPartitionConfig       `json:"segmentPartitionConfig,omitempty"`
}

// SegmentPartitionConfig represents segment partition configuration
type SegmentPartitionConfig struct {
	ColumnPartitionMap map[string]*ColumnPartition `json:"columnPartitionMap"`
}

// ColumnPartition represents a column partition configuration
type ColumnPartition struct {
	FunctionName  string `json:"functionName"`
	NumPartitions int    `json:"numPartitions"`
}

// TenantsConfig represents tenants configuration
type TenantsConfig struct {
	Broker string `json:"broker"`
	Server string `json:"server"`
}

// TableIndexConfig represents table index configuration
type TableIndexConfig struct {
	LoadMode             string            `json:"loadMode"`
	InvertedIndexColumns []string          `json:"invertedIndexColumns,omitempty"`
	NoDictionaryColumns  []string          `json:"noDictionaryColumns,omitempty"`
	RangeIndexColumns    []string          `json:"rangeIndexColumns,omitempty"`
	JsonIndexColumns     []string          `json:"jsonIndexColumns,omitempty"`
	StreamConfigs        map[string]string `json:"streamConfigs,omitempty"` // For REALTIME tables
}

// MetadataConfig represents metadata configuration
type MetadataConfig struct {
	CustomConfigs map[string]string `json:"customConfigs,omitempty"`
}

// RoutingConfig represents routing configuration
type RoutingConfig struct {
	SegmentPrunerTypes []string `json:"segmentPrunerTypes,omitempty"`
}

// SetupSchema creates the necessary tables and schemas in Pinot
func SetupSchema(ctx context.Context, client *Client) error {
	// Create spans schema and table
	if err := createSchemaAndTable(ctx, client, getSpansSchema(), getSpansTableConfig()); err != nil {
		return fmt.Errorf("failed to create spans: %w", err)
	}

	// Create metrics schema and table
	if err := createSchemaAndTable(ctx, client, getMetricsSchema(), getMetricsTableConfig()); err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	// Create logs schema and table
	if err := createSchemaAndTable(ctx, client, getLogsSchema(), getLogsTableConfig()); err != nil {
		return fmt.Errorf("failed to create logs: %w", err)
	}

	return nil
}

// createSchemaAndTable creates both schema and table configuration
func createSchemaAndTable(ctx context.Context, client *Client, schema *PinotSchema, tableConfig *TableConfig) error {
	// Create schema first
	if err := client.CreateSchema(ctx, schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Then create table
	if err := client.CreateTable(ctx, tableConfig); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// getSpansSchema returns the complete schema for the spans table
func getSpansSchema() *PinotSchema {
	return &PinotSchema{
		SchemaName: "otel_spans",
		DimensionFieldSpecs: []FieldSpec{
			// Tenant & Identity
			{Name: "tenant_id", DataType: "INT"},
			{Name: "trace_id", DataType: "STRING"},
			{Name: "span_id", DataType: "STRING"},
			{Name: "parent_span_id", DataType: "STRING"},
			{Name: "name", DataType: "STRING"},
			{Name: "kind", DataType: "STRING"},

			// Status
			{Name: "status_code", DataType: "STRING"},
			{Name: "status_message", DataType: "STRING"},

			// Common OTel Semantic Conventions - extracted for performance
			{Name: "service_name", DataType: "STRING"},
			{Name: "http_method", DataType: "STRING"},
			{Name: "http_status_code", DataType: "INT", DefaultNullValue: -1}, // -1 indicates no HTTP status (valid range: 100-599)
			{Name: "http_route", DataType: "STRING"},
			{Name: "http_target", DataType: "STRING"},
			{Name: "db_system", DataType: "STRING"},
			{Name: "db_statement", DataType: "STRING"},
			{Name: "messaging_system", DataType: "STRING"},
			{Name: "messaging_destination", DataType: "STRING"},
			{Name: "rpc_service", DataType: "STRING"},
			{Name: "rpc_method", DataType: "STRING"},
			{Name: "error", DataType: "BOOLEAN"},

			// Flexible attributes (remaining attributes as JSON)
			{Name: "attributes", DataType: "JSON"},
			{Name: "resource_attributes", DataType: "JSON"},
		},
		MetricFieldSpecs: []FieldSpec{
			{Name: "duration", DataType: "LONG"}, // nanoseconds
		},
		DateTimeFieldSpecs: []DateTimeFieldSpec{
			{
				Name:        "timestamp",
				DataType:    "LONG",
				Format:      "1:MILLISECONDS:EPOCH",
				Granularity: "1:MILLISECONDS",
			},
		},
	}
}

// getSpansTableConfig returns the table configuration for spans
func getSpansTableConfig() *TableConfig {
	return &TableConfig{
		TableName: "otel_spans",
		TableType: "REALTIME",
		Segmentation: &SegmentationConfig{
			TimeColumnName: "timestamp",
			TimeType:       "MILLISECONDS",
			Replication:    "1",
			SegmentPartitionConfig: &SegmentPartitionConfig{
				ColumnPartitionMap: map[string]*ColumnPartition{
					"tenant_id": {
						FunctionName:  "Modulo",
						NumPartitions: 10,
					},
				},
			},
		},
		Tenants: &TenantsConfig{
			Broker: "DefaultTenant",
			Server: "DefaultTenant",
		},
		Metadata: &MetadataConfig{
			CustomConfigs: map[string]string{},
		},
		TableIndexConfig: &TableIndexConfig{
			LoadMode:             "MMAP",
			InvertedIndexColumns: []string{"tenant_id", "trace_id", "span_id", "name", "service_name", "http_status_code"},
			JsonIndexColumns:     []string{"attributes", "resource_attributes"},
			RangeIndexColumns:    []string{"timestamp", "duration"},
			StreamConfigs: map[string]string{
				"streamType":                                    "kafka",
				"stream.kafka.topic.name":                       "otel-spans",
				"stream.kafka.broker.list":                      "kafka:9092",
				"stream.kafka.consumer.type":                    "lowlevel",
				"stream.kafka.consumer.factory.class.name":      "org.apache.pinot.plugin.stream.kafka20.KafkaConsumerFactory",
				"stream.kafka.decoder.class.name":               "org.apache.pinot.plugin.inputformat.json.JSONMessageDecoder",
				"stream.kafka.consumer.prop.auto.offset.reset":  "smallest",
				"realtime.segment.flush.threshold.rows":         "10000",
				"realtime.segment.flush.threshold.time":         "1m",
			},
		},
	}
}

// getMetricsSchema returns the complete schema for the metrics table
func getMetricsSchema() *PinotSchema {
	return &PinotSchema{
		SchemaName: "otel_metrics",
		DimensionFieldSpecs: []FieldSpec{
			// Tenant & Identity
			{Name: "tenant_id", DataType: "INT"},
			{Name: "metric_name", DataType: "STRING"},
			{Name: "metric_type", DataType: "STRING"}, // gauge, sum, histogram

			// Common metric labels - extracted for performance
			{Name: "service_name", DataType: "STRING"},
			{Name: "host_name", DataType: "STRING"},
			{Name: "environment", DataType: "STRING"},
			{Name: "job", DataType: "STRING"},
			{Name: "instance", DataType: "STRING"},

			// Exemplar support (the "wormhole" for trace correlation)
			{Name: "exemplar_trace_id", DataType: "STRING"},
			{Name: "exemplar_span_id", DataType: "STRING"},

			// Flexible attributes
			{Name: "attributes", DataType: "JSON"},
			{Name: "resource_attributes", DataType: "JSON"},
		},
		MetricFieldSpecs: []FieldSpec{
			{Name: "value", DataType: "DOUBLE"},
			{Name: "count", DataType: "LONG"},  // for histograms
			{Name: "sum", DataType: "DOUBLE"},  // for histograms
		},
		DateTimeFieldSpecs: []DateTimeFieldSpec{
			{
				Name:        "timestamp",
				DataType:    "LONG",
				Format:      "1:MILLISECONDS:EPOCH",
				Granularity: "1:MILLISECONDS",
			},
		},
	}
}

// getMetricsTableConfig returns the table configuration for metrics
func getMetricsTableConfig() *TableConfig {
	return &TableConfig{
		TableName: "otel_metrics",
		TableType: "REALTIME",
		Segmentation: &SegmentationConfig{
			TimeColumnName: "timestamp",
			TimeType:       "MILLISECONDS",
			Replication:    "1",
			SegmentPartitionConfig: &SegmentPartitionConfig{
				ColumnPartitionMap: map[string]*ColumnPartition{
					"tenant_id": {
						FunctionName:  "Modulo",
						NumPartitions: 10,
					},
				},
			},
		},
		Tenants: &TenantsConfig{
			Broker: "DefaultTenant",
			Server: "DefaultTenant",
		},
		Metadata: &MetadataConfig{
			CustomConfigs: map[string]string{},
		},
		TableIndexConfig: &TableIndexConfig{
			LoadMode:             "MMAP",
			InvertedIndexColumns: []string{"tenant_id", "metric_name", "service_name", "exemplar_trace_id"},
			JsonIndexColumns:     []string{"attributes", "resource_attributes"},
			RangeIndexColumns:    []string{"timestamp", "value"},
			StreamConfigs: map[string]string{
				"streamType":                                    "kafka",
				"stream.kafka.topic.name":                       "otel-metrics",
				"stream.kafka.broker.list":                      "kafka:9092",
				"stream.kafka.consumer.type":                    "lowlevel",
				"stream.kafka.consumer.factory.class.name":      "org.apache.pinot.plugin.stream.kafka20.KafkaConsumerFactory",
				"stream.kafka.decoder.class.name":               "org.apache.pinot.plugin.inputformat.json.JSONMessageDecoder",
				"stream.kafka.consumer.prop.auto.offset.reset":  "smallest",
				"realtime.segment.flush.threshold.rows":         "10000",
				"realtime.segment.flush.threshold.time":         "1m",
			},
		},
	}
}

// getLogsSchema returns the complete schema for the logs table
func getLogsSchema() *PinotSchema {
	return &PinotSchema{
		SchemaName: "otel_logs",
		DimensionFieldSpecs: []FieldSpec{
			// Tenant & Identity
			{Name: "tenant_id", DataType: "INT"},
			{Name: "trace_id", DataType: "STRING"},
			{Name: "span_id", DataType: "STRING"},
			{Name: "severity_text", DataType: "STRING"},
			{Name: "body", DataType: "STRING"},

			// Common log attributes - extracted for performance
			{Name: "service_name", DataType: "STRING"},
			{Name: "host_name", DataType: "STRING"},
			{Name: "log_level", DataType: "STRING"},
			{Name: "log_source", DataType: "STRING"},

			// Prometheus/Loki common labels - extracted for performance
			{Name: "job", DataType: "STRING"},
			{Name: "instance", DataType: "STRING"},
			{Name: "environment", DataType: "STRING"},

			// Flexible attributes
			{Name: "attributes", DataType: "JSON"},
			{Name: "resource_attributes", DataType: "JSON"},
		},
		MetricFieldSpecs: []FieldSpec{
			{Name: "severity_number", DataType: "INT"},
		},
		DateTimeFieldSpecs: []DateTimeFieldSpec{
			{
				Name:        "timestamp",
				DataType:    "LONG",
				Format:      "1:MILLISECONDS:EPOCH",
				Granularity: "1:MILLISECONDS",
			},
		},
	}
}

// getLogsTableConfig returns the table configuration for logs
func getLogsTableConfig() *TableConfig {
	return &TableConfig{
		TableName: "otel_logs",
		TableType: "REALTIME",
		Segmentation: &SegmentationConfig{
			TimeColumnName: "timestamp",
			TimeType:       "MILLISECONDS",
			Replication:    "1",
			SegmentPartitionConfig: &SegmentPartitionConfig{
				ColumnPartitionMap: map[string]*ColumnPartition{
					"tenant_id": {
						FunctionName:  "Modulo",
						NumPartitions: 10,
					},
				},
			},
		},
		Tenants: &TenantsConfig{
			Broker: "DefaultTenant",
			Server: "DefaultTenant",
		},
		Metadata: &MetadataConfig{
			CustomConfigs: map[string]string{},
		},
		TableIndexConfig: &TableIndexConfig{
			LoadMode:             "MMAP",
			InvertedIndexColumns: []string{"tenant_id", "trace_id", "span_id", "severity_text", "service_name", "log_level", "job", "instance", "environment"},
			JsonIndexColumns:     []string{"attributes", "resource_attributes"},
			RangeIndexColumns:    []string{"timestamp", "severity_number"},
			StreamConfigs: map[string]string{
				"streamType":                                    "kafka",
				"stream.kafka.topic.name":                       "otel-logs",
				"stream.kafka.broker.list":                      "kafka:9092",
				"stream.kafka.consumer.type":                    "lowlevel",
				"stream.kafka.consumer.factory.class.name":      "org.apache.pinot.plugin.stream.kafka20.KafkaConsumerFactory",
				"stream.kafka.decoder.class.name":               "org.apache.pinot.plugin.inputformat.json.JSONMessageDecoder",
				"stream.kafka.consumer.prop.auto.offset.reset":  "smallest",
				"realtime.segment.flush.threshold.rows":         "10000",
				"realtime.segment.flush.threshold.time":         "1m",
			},
		},
	}
}
