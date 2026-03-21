package pinot

import (
	"context"
	"fmt"
)

// TableSchema represents a Pinot table schema
type TableSchema struct {
	SchemaName string `json:"schemaName"`
	TableName  string `json:"tableName"`
	TableType  string `json:"tableType"`
	Segmentation struct {
		SegmentPartitionConfig struct {
			ColumnPartitionMap map[string]struct {
				FunctionName string `json:"functionName"`
				NumPartitions int   `json:"numPartitions"`
			} `json:"columnPartitionMap"`
		} `json:"segmentPartitionConfig"`
	} `json:"segmentation"`
	Tenants struct {
		Broker string `json:"broker"`
		Server string `json:"server"`
	} `json:"tenants"`
	TableIndexConfig struct {
		LoadMode         string   `json:"loadMode"`
		InvertedIndexColumns []string `json:"invertedIndexColumns"`
	} `json:"tableIndexConfig"`
	Metadata struct {
		CustomConfigs map[string]string `json:"customConfigs"`
	} `json:"metadata"`
	FieldConfigList []struct {
		Name         string `json:"name"`
		EncodingType string `json:"encodingType"`
		IndexType    string `json:"indexType"`
	} `json:"fieldConfigList"`
}

// SetupSchema creates the necessary tables in Pinot
func SetupSchema(ctx context.Context, client *Client) error {
	// Create metrics table
	if err := client.CreateTable(ctx, getMetricsSchema()); err != nil {
		return fmt.Errorf("failed to create metrics table: %w", err)
	}

	// Create logs table
	if err := client.CreateTable(ctx, getLogsSchema()); err != nil {
		return fmt.Errorf("failed to create logs table: %w", err)
	}

	// Create spans table
	if err := client.CreateTable(ctx, getSpansSchema()); err != nil {
		return fmt.Errorf("failed to create spans table: %w", err)
	}

	return nil
}

// getMetricsSchema returns the schema for the metrics table
func getMetricsSchema() *TableSchema {
	schema := &TableSchema{
		SchemaName: "otel_metrics",
		TableName:  "otel_metrics",
		TableType:  "REALTIME",
	}

	// Partition by tenant_id
	schema.Segmentation.SegmentPartitionConfig.ColumnPartitionMap = map[string]struct {
		FunctionName  string `json:"functionName"`
		NumPartitions int    `json:"numPartitions"`
	}{
		"tenant_id": {
			FunctionName:  "Modulo",
			NumPartitions: 10,
		},
	}

	schema.Tenants.Broker = "DefaultTenant"
	schema.Tenants.Server = "DefaultTenant"

	schema.TableIndexConfig.LoadMode = "MMAP"
	schema.TableIndexConfig.InvertedIndexColumns = []string{"tenant_id", "metric_name", "timestamp"}

	return schema
}

// getLogsSchema returns the schema for the logs table
func getLogsSchema() *TableSchema {
	schema := &TableSchema{
		SchemaName: "otel_logs",
		TableName:  "otel_logs",
		TableType:  "REALTIME",
	}

	// Partition by tenant_id
	schema.Segmentation.SegmentPartitionConfig.ColumnPartitionMap = map[string]struct {
		FunctionName  string `json:"functionName"`
		NumPartitions int    `json:"numPartitions"`
	}{
		"tenant_id": {
			FunctionName:  "Modulo",
			NumPartitions: 10,
		},
	}

	schema.Tenants.Broker = "DefaultTenant"
	schema.Tenants.Server = "DefaultTenant"

	schema.TableIndexConfig.LoadMode = "MMAP"
	schema.TableIndexConfig.InvertedIndexColumns = []string{"tenant_id", "trace_id", "timestamp"}

	return schema
}

// getSpansSchema returns the schema for the spans table
func getSpansSchema() *TableSchema {
	schema := &TableSchema{
		SchemaName: "otel_spans",
		TableName:  "otel_spans",
		TableType:  "REALTIME",
	}

	// Partition by tenant_id
	schema.Segmentation.SegmentPartitionConfig.ColumnPartitionMap = map[string]struct {
		FunctionName  string `json:"functionName"`
		NumPartitions int    `json:"numPartitions"`
	}{
		"tenant_id": {
			FunctionName:  "Modulo",
			NumPartitions: 10,
		},
	}

	schema.Tenants.Broker = "DefaultTenant"
	schema.Tenants.Server = "DefaultTenant"

	schema.TableIndexConfig.LoadMode = "MMAP"
	schema.TableIndexConfig.InvertedIndexColumns = []string{"tenant_id", "trace_id", "span_id", "name", "timestamp"}

	return schema
}
