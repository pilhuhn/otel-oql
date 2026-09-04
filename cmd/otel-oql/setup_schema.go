package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/pilhuhn/otel-oql/pkg/clickhouse"
)

// setupSchemaCommand runs the schema setup
func setupSchemaCommand() error {
	clickhouseURL := flag.String("clickhouse-url", "http://localhost:8123", "Clickhouse HTTP URL")
	flag.Parse()

	fmt.Printf("Setting up Clickhouse schema at %s...\n", *clickhouseURL)

	client := clickhouse.NewClient(*clickhouseURL)
	ctx := context.Background()

	if err := clickhouse.SetupSchema(ctx, client); err != nil {
		return fmt.Errorf("failed to setup schema: %w", err)
	}

	fmt.Println("Schema setup completed successfully")
	return nil
}
