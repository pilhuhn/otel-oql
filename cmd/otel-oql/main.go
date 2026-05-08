package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/pilhuhn/otel-oql/internal/config"
	"github.com/pilhuhn/otel-oql/pkg/api"
	"github.com/pilhuhn/otel-oql/pkg/auth"
	"github.com/pilhuhn/otel-oql/pkg/ingestion"
	"github.com/pilhuhn/otel-oql/pkg/mcp"
	"github.com/pilhuhn/otel-oql/pkg/observability"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/receiver"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"github.com/pilhuhn/otel-oql/pkg/userstore"
	"google.golang.org/grpc"
)

func main() {
	// Add panic handler for unexpected crashes
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PANIC: %v\n", r)
			fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", debug.Stack())
			os.Exit(2)
		}
	}()

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

	ctx := context.Background()

	// Route to mode-specific run function
	switch cfg.Mode {
	case "ingestion":
		return runIngestionMode(ctx, cfg)
	case "query":
		return runQueryMode(ctx, cfg)
	case "all":
		return runAllMode(ctx, cfg)
	default:
		return fmt.Errorf("invalid mode: %s", cfg.Mode)
	}
}

// initAuth initializes user store and auth middleware
func initAuth(cfg *config.Config) (*auth.Middleware, error) {
	// Check if user files exist
	if _, err := os.Stat(cfg.UsersFile); os.IsNotExist(err) {
		// In test mode, auth is optional
		if cfg.TestMode {
			fmt.Printf("Warning: Users file not found (%s), running in test mode without authentication\n", cfg.UsersFile)
			return nil, nil
		}
		return nil, fmt.Errorf("users file not found: %s (create users.csv or enable test mode)", cfg.UsersFile)
	}

	if _, err := os.Stat(cfg.APIKeysFile); os.IsNotExist(err) {
		if cfg.TestMode {
			fmt.Printf("Warning: API keys file not found (%s), running in test mode without authentication\n", cfg.APIKeysFile)
			return nil, nil
		}
		return nil, fmt.Errorf("API keys file not found: %s (create api-keys.csv or enable test mode)", cfg.APIKeysFile)
	}

	// Initialize user store
	store, err := userstore.NewFileStore(cfg.UsersFile, cfg.APIKeysFile)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize user store: %w", err)
	}

	fmt.Printf("Authentication enabled:\n")
	fmt.Printf("  Users file: %s\n", cfg.UsersFile)
	fmt.Printf("  API keys file: %s\n", cfg.APIKeysFile)

	// Create auth middleware
	authMiddleware := auth.NewMiddleware(store, cfg.TestMode)
	return authMiddleware, nil
}

func runAllMode(ctx context.Context, cfg *config.Config) error {
	fmt.Printf("Starting OTEL-OQL service (all-in-one mode)...\n")
	fmt.Printf("Pinot URL: %s\n", cfg.PinotURL)
	fmt.Printf("Kafka Brokers: %s\n", cfg.KafkaBrokers)
	fmt.Printf("OTLP gRPC Port: %d\n", cfg.OTLPGRPCPort)
	fmt.Printf("OTLP HTTP Port: %d\n", cfg.OTLPHTTPPort)
	fmt.Printf("Query API Port: %d\n", cfg.QueryAPIPort)
	fmt.Printf("MCP Port: %d\n", cfg.MCPPort)
	fmt.Printf("Test Mode: %v\n", cfg.TestMode)
	fmt.Printf("Observability: %v\n", cfg.ObservabilityEnabled)
	if cfg.ObservabilityEnabled {
		fmt.Printf("  Endpoint: %s\n", cfg.ObservabilityEndpoint)
		fmt.Printf("  Tenant ID: %s\n", cfg.ObservabilityTenantID)
	}

	// Initialize authentication
	authMiddleware, err := initAuth(cfg)
	if err != nil {
		return err
	}

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

	// Initialize Pinot client (for queries)
	pinotClient := pinot.NewClient(cfg.PinotURL)

	// Determine which middleware to use
	var httpMiddleware func(http.Handler) http.Handler
	var grpcUnaryInterceptor func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error)

	if authMiddleware != nil {
		// Use auth middleware (production mode or test mode with user files)
		httpMiddleware = authMiddleware.HTTPMiddleware
		grpcUnaryInterceptor = authMiddleware.GRPCUnaryInterceptor()
	} else {
		// Fall back to tenant validator (test mode without user files)
		validator := tenant.NewValidator(cfg.TestMode)
		httpMiddleware = validator.HTTPMiddleware
		grpcUnaryInterceptor = validator.GRPCUnaryInterceptor()
	}

	// Initialize ingester (with Kafka)
	ingester, err := ingestion.NewIngester(cfg.KafkaBrokers, obs, cfg.DebugIngestion)
	if err != nil {
		return fmt.Errorf("failed to create ingester: %w", err)
	}
	defer ingester.Close()

	// Initialize receivers with auth/tenant middleware
	grpcReceiver := receiver.NewGRPCReceiver(cfg.OTLPGRPCPort, grpcUnaryInterceptor, ingester, obs, cfg.DebugIngestion)
	httpReceiver := receiver.NewHTTPReceiver(cfg.OTLPHTTPPort, httpMiddleware, ingester, obs, cfg.DebugIngestion)

	// Initialize query API server with auth/tenant middleware
	queryServer := api.NewServer(cfg.QueryAPIPort, httpMiddleware, pinotClient, obs, cfg.DebugQuery, cfg.DebugTranslation)

	// Initialize MCP server
	mcpServer := mcp.NewServer(cfg.MCPPort, pinotClient)

	// Start receivers
	if err := grpcReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start gRPC receiver: %w", err)
	}

	if err := httpReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP receiver: %w", err)
	}

	// Start query API server
	if err := queryServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start query API server: %w", err)
	}

	// Start MCP server
	if err := mcpServer.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	fmt.Println("OTEL-OQL service started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

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

	if err := mcpServer.Stop(ctx); err != nil {
		fmt.Printf("Error stopping MCP server: %v\n", err)
	}

	// Close Pulsar connections
	ingester.Close()

	fmt.Println("Shutdown complete")
	return nil
}

func runIngestionMode(ctx context.Context, cfg *config.Config) error {
	fmt.Printf("Starting OTEL-OQL service (ingestion mode)...\n")
	fmt.Printf("Kafka Brokers: %s\n", cfg.KafkaBrokers)
	fmt.Printf("OTLP gRPC Port: %d\n", cfg.OTLPGRPCPort)
	fmt.Printf("OTLP HTTP Port: %d\n", cfg.OTLPHTTPPort)
	fmt.Printf("Test Mode: %v\n", cfg.TestMode)
	fmt.Printf("Observability: %v\n", cfg.ObservabilityEnabled)
	if cfg.ObservabilityEnabled {
		fmt.Printf("  Endpoint: %s\n", cfg.ObservabilityEndpoint)
		fmt.Printf("  Tenant ID: %s\n", cfg.ObservabilityTenantID)
	}

	// Initialize authentication
	authMiddleware, err := initAuth(cfg)
	if err != nil {
		return err
	}

	// Initialize observability (self-instrumentation)
	obs, err := observability.New(ctx, observability.Config{
		ServiceName:    "otel-oql-ingestion",
		ServiceVersion: "1.0.0",
		OTLPEndpoint:   cfg.ObservabilityEndpoint,
		TenantID:       cfg.ObservabilityTenantID,
		Enabled:        cfg.ObservabilityEnabled,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	defer obs.Shutdown(ctx)

	// Determine which middleware to use
	var httpMiddleware func(http.Handler) http.Handler
	var grpcUnaryInterceptor func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error)

	if authMiddleware != nil {
		httpMiddleware = authMiddleware.HTTPMiddleware
		grpcUnaryInterceptor = authMiddleware.GRPCUnaryInterceptor()
	} else {
		validator := tenant.NewValidator(cfg.TestMode)
		httpMiddleware = validator.HTTPMiddleware
		grpcUnaryInterceptor = validator.GRPCUnaryInterceptor()
	}

	// Initialize ingester (with Kafka)
	ingester, err := ingestion.NewIngester(cfg.KafkaBrokers, obs, cfg.DebugIngestion)
	if err != nil {
		return fmt.Errorf("failed to create ingester: %w", err)
	}
	defer ingester.Close()

	// Initialize receivers with auth/tenant middleware
	grpcReceiver := receiver.NewGRPCReceiver(cfg.OTLPGRPCPort, grpcUnaryInterceptor, ingester, obs, cfg.DebugIngestion)
	httpReceiver := receiver.NewHTTPReceiver(cfg.OTLPHTTPPort, httpMiddleware, ingester, obs, cfg.DebugIngestion)

	// Start receivers
	if err := grpcReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start gRPC receiver: %w", err)
	}

	if err := httpReceiver.Start(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP receiver: %w", err)
	}

	fmt.Println("OTEL-OQL ingestion service started successfully")
	fmt.Printf("Listening for OTLP data on ports %d (gRPC) and %d (HTTP)\n", cfg.OTLPGRPCPort, cfg.OTLPHTTPPort)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down ingestion service...")

	// Stop receivers
	if err := grpcReceiver.Stop(ctx); err != nil {
		fmt.Printf("Error stopping gRPC receiver: %v\n", err)
	}

	if err := httpReceiver.Stop(ctx); err != nil {
		fmt.Printf("Error stopping HTTP receiver: %v\n", err)
	}

	// Close Kafka connections
	ingester.Close()

	fmt.Println("Shutdown complete")
	return nil
}

func runQueryMode(ctx context.Context, cfg *config.Config) error {
	fmt.Printf("Starting OTEL-OQL service (query mode)...\n")
	fmt.Printf("Pinot URL: %s\n", cfg.PinotURL)
	fmt.Printf("Query API Port: %d\n", cfg.QueryAPIPort)
	fmt.Printf("MCP Port: %d\n", cfg.MCPPort)
	fmt.Printf("Test Mode: %v\n", cfg.TestMode)
	fmt.Printf("Observability: %v\n", cfg.ObservabilityEnabled)
	if cfg.ObservabilityEnabled {
		fmt.Printf("  Endpoint: %s\n", cfg.ObservabilityEndpoint)
		fmt.Printf("  Tenant ID: %s\n", cfg.ObservabilityTenantID)
	}

	// Initialize authentication
	authMiddleware, err := initAuth(cfg)
	if err != nil {
		return err
	}

	// Initialize observability (self-instrumentation)
	obs, err := observability.New(ctx, observability.Config{
		ServiceName:    "otel-oql-query",
		ServiceVersion: "1.0.0",
		OTLPEndpoint:   cfg.ObservabilityEndpoint,
		TenantID:       cfg.ObservabilityTenantID,
		Enabled:        cfg.ObservabilityEnabled,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}
	defer obs.Shutdown(ctx)

	// Initialize Pinot client (for queries)
	pinotClient := pinot.NewClient(cfg.PinotURL)

	// Determine which middleware to use
	var httpMiddleware func(http.Handler) http.Handler

	if authMiddleware != nil {
		httpMiddleware = authMiddleware.HTTPMiddleware
	} else {
		validator := tenant.NewValidator(cfg.TestMode)
		httpMiddleware = validator.HTTPMiddleware
	}

	// Initialize query API server with auth/tenant middleware
	queryServer := api.NewServer(cfg.QueryAPIPort, httpMiddleware, pinotClient, obs, cfg.DebugQuery, cfg.DebugTranslation)

	// Initialize MCP server
	mcpServer := mcp.NewServer(cfg.MCPPort, pinotClient)

	// Start query API server
	if err := queryServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start query API server: %w", err)
	}

	// Start MCP server
	if err := mcpServer.Start(); err != nil {
		return fmt.Errorf("failed to start MCP server: %w", err)
	}

	fmt.Println("OTEL-OQL query service started successfully")
	fmt.Printf("Query API available on port %d\n", cfg.QueryAPIPort)
	fmt.Printf("MCP server available on port %d\n", cfg.MCPPort)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down query service...")

	// Stop servers
	if err := queryServer.Stop(ctx); err != nil {
		fmt.Printf("Error stopping query API server: %v\n", err)
	}

	if err := mcpServer.Stop(ctx); err != nil {
		fmt.Printf("Error stopping MCP server: %v\n", err)
	}

	fmt.Println("Shutdown complete")
	return nil
}
