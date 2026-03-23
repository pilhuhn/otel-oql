package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/pilhuhn/otel-oql/internal/config"
	"github.com/pilhuhn/otel-oql/pkg/api"
	"github.com/pilhuhn/otel-oql/pkg/ingestion"
	"github.com/pilhuhn/otel-oql/pkg/observability"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/receiver"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
)

func main() {
	// Add exit handler to capture stack trace on any exit
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
			fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", debug.Stack())
			os.Exit(2)
		}
		fmt.Println("DEBUG: main() function exiting normally")
		fmt.Printf("Stack trace at exit:\n%s\n", debug.Stack())
	}()

	// Enable full stack traces on panic
	debug.SetTraceback("all")

	// Check for subcommand
	if len(os.Args) > 1 && os.Args[1] == "setup-schema" {
		if err := setupSchemaCommand(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Default: run the service
	fmt.Println("DEBUG: Calling run()")
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("DEBUG: run() returned without error")
}

func run() error {
	fmt.Println("DEBUG: run() started")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	fmt.Println("DEBUG: Configuration loaded")

	fmt.Printf("Starting OTEL-OQL service...\n")
	fmt.Printf("Pinot URL: %s\n", cfg.PinotURL)
	fmt.Printf("Kafka Brokers: %s\n", cfg.KafkaBrokers)
	fmt.Printf("OTLP gRPC Port: %d\n", cfg.OTLPGRPCPort)
	fmt.Printf("OTLP HTTP Port: %d\n", cfg.OTLPHTTPPort)
	fmt.Printf("Query API Port: %d\n", cfg.QueryAPIPort)
	fmt.Printf("Test Mode: %v\n", cfg.TestMode)
	fmt.Printf("Observability: %v\n", cfg.ObservabilityEnabled)
	if cfg.ObservabilityEnabled {
		fmt.Printf("  Endpoint: %s\n", cfg.ObservabilityEndpoint)
		fmt.Printf("  Tenant ID: %s\n", cfg.ObservabilityTenantID)
	}

	ctx := context.Background()

	// Initialize observability (self-instrumentation)
	obs, err := observability.New(ctx, observability.Config{
		ServiceName:    "otel-oql",
		ServiceVersion: "1.0.0",
		OTLPEndpoint:   cfg.ObservabilityEndpoint,
		TenantID:       cfg.ObservabilityTenantID,
		Enabled:        cfg.ObservabilityEnabled,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	defer obs.Shutdown(ctx)
	fmt.Println("DEBUG: Observability initialized")

	// Initialize Pinot client (for queries)
	pinotClient := pinot.NewClient(cfg.PinotURL)
	fmt.Println("DEBUG: Pinot client created")

	// Initialize tenant validator
	validator := tenant.NewValidator(cfg.TestMode)
	fmt.Println("DEBUG: Tenant validator created")

	// Initialize ingester (with Kafka)
	ingester, err := ingestion.NewIngester(cfg.KafkaBrokers, obs)
	if err != nil {
		return fmt.Errorf("failed to create ingester: %w", err)
	}
	defer ingester.Close()
	fmt.Println("DEBUG: Ingester created")

	// Initialize receivers
	grpcReceiver := receiver.NewGRPCReceiver(cfg.OTLPGRPCPort, validator, ingester, obs)
	httpReceiver := receiver.NewHTTPReceiver(cfg.OTLPHTTPPort, validator, ingester, obs)
	fmt.Println("DEBUG: Receivers created")

	// Initialize query API server
	queryServer := api.NewServer(cfg.QueryAPIPort, validator, pinotClient, obs)
	fmt.Println("DEBUG: Query server created")

	// Start receivers
	fmt.Println("DEBUG: Starting gRPC receiver...")
	if err := grpcReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start gRPC receiver: %w", err)
	}
	fmt.Println("DEBUG: gRPC receiver started")

	fmt.Println("DEBUG: Starting HTTP receiver...")
	if err := httpReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP receiver: %w", err)
	}
	fmt.Println("DEBUG: HTTP receiver started")

	// Start query API server
	fmt.Println("DEBUG: Starting query API server...")
	if err := queryServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start query API server: %w", err)
	}
	fmt.Println("DEBUG: Query API server started")

	fmt.Println("OTEL-OQL service started successfully")

	// Wait for shutdown signal
	fmt.Println("DEBUG: Creating signal channel and waiting for signal...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan
	fmt.Printf("DEBUG: Received signal: %v\n", sig)

	fmt.Println("\nShutting down...")

	// Stop servers
	if err := grpcReceiver.Stop(ctx); err != nil {
		fmt.Printf("Error stopping gRPC receiver: %v\n", err)
	}

	if err := httpReceiver.Stop(ctx); err != nil {
		fmt.Printf("Error stopping HTTP receiver: %v\n", err)
	}

	if err := queryServer.Stop(ctx); err != nil {
		fmt.Printf("Error stopping query API server: %v\n", err)
	}

	// Close Pulsar connections
	ingester.Close()

	fmt.Println("Shutdown complete")
	return nil
}
