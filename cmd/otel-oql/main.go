package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pilhuhn/otel-oql/internal/config"
	"github.com/pilhuhn/otel-oql/pkg/ingestion"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/receiver"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
)

func main() {
	// Check for subcommand
	if len(os.Args) > 1 && os.Args[1] == "setup-schema" {
		if err := setupSchemaCommand(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Default: run the service
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Starting OTEL-OQL service...\n")
	fmt.Printf("Pinot URL: %s\n", cfg.PinotURL)
	fmt.Printf("OTLP gRPC Port: %d\n", cfg.OTLPGRPCPort)
	fmt.Printf("OTLP HTTP Port: %d\n", cfg.OTLPHTTPPort)
	fmt.Printf("Test Mode: %v\n", cfg.TestMode)

	// Initialize Pinot client
	pinotClient := pinot.NewClient(cfg.PinotURL)

	// Initialize tenant validator
	validator := tenant.NewValidator(cfg.TestMode)

	// Initialize ingester
	ingester := ingestion.NewIngester(pinotClient)

	// Initialize receivers
	grpcReceiver := receiver.NewGRPCReceiver(cfg.OTLPGRPCPort, validator, ingester)
	httpReceiver := receiver.NewHTTPReceiver(cfg.OTLPHTTPPort, validator, ingester)

	ctx := context.Background()

	// Start receivers
	if err := grpcReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start gRPC receiver: %w", err)
	}

	if err := httpReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP receiver: %w", err)
	}

	fmt.Println("OTEL-OQL service started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")

	// Stop receivers
	if err := grpcReceiver.Stop(ctx); err != nil {
		fmt.Printf("Error stopping gRPC receiver: %v\n", err)
	}

	if err := httpReceiver.Stop(ctx); err != nil {
		fmt.Printf("Error stopping HTTP receiver: %v\n", err)
	}

	fmt.Println("Shutdown complete")
	return nil
}
