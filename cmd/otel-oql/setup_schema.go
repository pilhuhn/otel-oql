package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/pilhuhn/otel-oql/pkg/pinot"
)

// setupSchemaCommand runs the schema setup
func setupSchemaCommand() error {
	pinotURL := flag.String("pinot-url", "http://localhost:9000", "Pinot broker URL")
	flag.Parse()

	if *pinotURL == "" {
		return fmt.Errorf("pinot-url is required")
	}

	fmt.Printf("Setting up Pinot schema at %s...\n", *pinotURL)

	client := pinot.NewClient(*pinotURL)
	ctx := context.Background()

	if err := pinot.SetupSchema(ctx, client); err != nil {
		return fmt.Errorf("failed to setup schema: %w", err)
	}

	fmt.Println("Schema setup completed successfully")
	return nil
}
