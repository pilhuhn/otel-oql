package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

// Config holds the application configuration
type Config struct {
	// Pinot configuration
	PinotURL string

	// OTLP receiver ports
	OTLPGRPCPort int
	OTLPHTTPPort int

	// Multi-tenancy
	TestMode bool // If true, uses tenant-id=0 as default

	// Query API
	QueryAPIPort int
}

// Load reads configuration from environment variables and command-line flags
func Load() (*Config, error) {
	cfg := &Config{}

	// Define flags
	flag.StringVar(&cfg.PinotURL, "pinot-url", getEnv("PINOT_URL", "http://localhost:9000"), "Apache Pinot broker URL")
	flag.IntVar(&cfg.OTLPGRPCPort, "otlp-grpc-port", getEnvInt("OTLP_GRPC_PORT", 4317), "OTLP gRPC receiver port")
	flag.IntVar(&cfg.OTLPHTTPPort, "otlp-http-port", getEnvInt("OTLP_HTTP_PORT", 4318), "OTLP HTTP receiver port")
	flag.BoolVar(&cfg.TestMode, "test-mode", getEnvBool("TEST_MODE", false), "Enable test mode (default tenant-id=0)")
	flag.IntVar(&cfg.QueryAPIPort, "query-api-port", getEnvInt("QUERY_API_PORT", 8080), "Query API server port")

	flag.Parse()

	// Validate configuration
	if cfg.PinotURL == "" {
		return nil, fmt.Errorf("pinot-url is required")
	}

	if cfg.OTLPGRPCPort <= 0 || cfg.OTLPGRPCPort > 65535 {
		return nil, fmt.Errorf("invalid otlp-grpc-port: %d", cfg.OTLPGRPCPort)
	}

	if cfg.OTLPHTTPPort <= 0 || cfg.OTLPHTTPPort > 65535 {
		return nil, fmt.Errorf("invalid otlp-http-port: %d", cfg.OTLPHTTPPort)
	}

	if cfg.QueryAPIPort <= 0 || cfg.QueryAPIPort > 65535 {
		return nil, fmt.Errorf("invalid query-api-port: %d", cfg.QueryAPIPort)
	}

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retrieves an integer environment variable or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool retrieves a boolean environment variable or returns a default value
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
