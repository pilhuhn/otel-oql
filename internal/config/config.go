package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	// Pinot configuration
	PinotURL string `yaml:"pinot_url"`

	// Kafka configuration
	KafkaBrokers string `yaml:"kafka_brokers"`

	// OTLP receiver ports
	OTLPGRPCPort int `yaml:"otlp_grpc_port"`
	OTLPHTTPPort int `yaml:"otlp_http_port"`

	// Multi-tenancy
	TestMode bool `yaml:"test_mode"` // If true, uses tenant-id=0 as default

	// Query API
	QueryAPIPort int `yaml:"query_api_port"`

	// Observability (self-instrumentation)
	ObservabilityEnabled  bool   `yaml:"observability_enabled"`  // Enable self-observability
	ObservabilityEndpoint string `yaml:"observability_endpoint"` // OTLP gRPC endpoint (default: localhost:4317)
	ObservabilityTenantID string `yaml:"observability_tenant_id"` // Tenant ID for self-observability (default: "0" in test mode)
}

// Load reads configuration from config file, environment variables, and command-line flags
// Priority (highest to lowest): CLI flags > Environment variables > Config file > Defaults
func Load() (*Config, error) {
	cfg := &Config{}

	// Config file path flag (needs to be parsed first)
	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to config file (default: ./otel-oql.yaml or ~/.otel-oql/config.yaml)")

	// Define other flags with empty defaults (will be filled from config file or env)
	var pinotURL string
	var kafkaBrokers string
	var otlpGRPCPort int
	var otlpHTTPPort int
	var queryAPIPort int
	var testMode bool
	var obsEnabled bool
	var obsEndpoint string
	var obsTenantID string

	flag.StringVar(&pinotURL, "pinot-url", "", "Apache Pinot broker URL")
	flag.StringVar(&kafkaBrokers, "kafka-brokers", "", "Kafka broker addresses")
	flag.IntVar(&otlpGRPCPort, "otlp-grpc-port", 0, "OTLP gRPC receiver port")
	flag.IntVar(&otlpHTTPPort, "otlp-http-port", 0, "OTLP HTTP receiver port")
	flag.BoolVar(&testMode, "test-mode", false, "Enable test mode (default tenant-id=0)")
	flag.IntVar(&queryAPIPort, "query-api-port", 0, "Query API server port")
	flag.BoolVar(&obsEnabled, "observability-enabled", false, "Enable self-observability")
	flag.StringVar(&obsEndpoint, "observability-endpoint", "", "OTLP endpoint for self-observability")
	flag.StringVar(&obsTenantID, "observability-tenant-id", "", "Tenant ID for self-observability")

	flag.Parse()

	// 1. Load from config file (if exists)
	if err := loadConfigFile(cfg, configFile); err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// 2. Override with environment variables
	if env := os.Getenv("PINOT_URL"); env != "" {
		cfg.PinotURL = env
	}
	if env := os.Getenv("KAFKA_BROKERS"); env != "" {
		cfg.KafkaBrokers = env
	}
	if env := os.Getenv("OTLP_GRPC_PORT"); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			cfg.OTLPGRPCPort = val
		}
	}
	if env := os.Getenv("OTLP_HTTP_PORT"); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			cfg.OTLPHTTPPort = val
		}
	}
	if env := os.Getenv("QUERY_API_PORT"); env != "" {
		if val, err := strconv.Atoi(env); err == nil {
			cfg.QueryAPIPort = val
		}
	}
	if env := os.Getenv("TEST_MODE"); env != "" {
		if val, err := strconv.ParseBool(env); err == nil {
			cfg.TestMode = val
		}
	}
	if env := os.Getenv("OBSERVABILITY_ENABLED"); env != "" {
		if val, err := strconv.ParseBool(env); err == nil {
			cfg.ObservabilityEnabled = val
		}
	}
	if env := os.Getenv("OBSERVABILITY_ENDPOINT"); env != "" {
		cfg.ObservabilityEndpoint = env
	}
	if env := os.Getenv("OBSERVABILITY_TENANT_ID"); env != "" {
		cfg.ObservabilityTenantID = env
	}

	// 3. Override with CLI flags (if provided)
	if pinotURL != "" {
		cfg.PinotURL = pinotURL
	}
	if kafkaBrokers != "" {
		cfg.KafkaBrokers = kafkaBrokers
	}
	if otlpGRPCPort != 0 {
		cfg.OTLPGRPCPort = otlpGRPCPort
	}
	if otlpHTTPPort != 0 {
		cfg.OTLPHTTPPort = otlpHTTPPort
	}
	if queryAPIPort != 0 {
		cfg.QueryAPIPort = queryAPIPort
	}
	// testMode from flags is handled specially since false is the default
	if flag.Lookup("test-mode").Value.String() == "true" {
		cfg.TestMode = true
	}
	// observability flags
	if flag.Lookup("observability-enabled").Value.String() == "true" {
		cfg.ObservabilityEnabled = true
	}
	if obsEndpoint != "" {
		cfg.ObservabilityEndpoint = obsEndpoint
	}
	if obsTenantID != "" {
		cfg.ObservabilityTenantID = obsTenantID
	}

	// Apply defaults if still not set
	if cfg.PinotURL == "" {
		cfg.PinotURL = "http://localhost:9000"
	}
	if cfg.KafkaBrokers == "" {
		cfg.KafkaBrokers = "localhost:9092"
	}
	if cfg.OTLPGRPCPort == 0 {
		cfg.OTLPGRPCPort = 4317
	}
	if cfg.OTLPHTTPPort == 0 {
		cfg.OTLPHTTPPort = 4318
	}
	if cfg.QueryAPIPort == 0 {
		cfg.QueryAPIPort = 8080
	}
	// Observability defaults
	if cfg.ObservabilityEndpoint == "" {
		cfg.ObservabilityEndpoint = "localhost:4317"
	}
	if cfg.ObservabilityTenantID == "" {
		if cfg.TestMode {
			cfg.ObservabilityTenantID = "0"
		} else {
			cfg.ObservabilityTenantID = "0" // Default to tenant 0 for self-observability
		}
	}

	// Validate configuration
	if cfg.PinotURL == "" {
		return nil, fmt.Errorf("pinot-url is required")
	}

	if cfg.KafkaBrokers == "" {
		return nil, fmt.Errorf("kafka-brokers is required")
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

// loadConfigFile loads configuration from a YAML file
func loadConfigFile(cfg *Config, configPath string) error {
	// Determine config file path
	if configPath == "" {
		// Try default locations
		locations := []string{
			"./otel-oql.yaml",
			filepath.Join(os.Getenv("HOME"), ".otel-oql", "config.yaml"),
			"/etc/otel-oql/config.yaml",
		}

		for _, loc := range locations {
			if _, err := os.Stat(loc); err == nil {
				configPath = loc
				break
			}
		}
	}

	// If no config file found, return (not an error)
	if configPath == "" {
		return nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		// If explicitly specified, it's an error. If default location, it's ok.
		if flag.Lookup("config").Value.String() != "" {
			return fmt.Errorf("failed to read config file %s: %w", configPath, err)
		}
		return nil
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	return nil
}

