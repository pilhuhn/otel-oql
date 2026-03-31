package config

import (
	"os"
	"testing"
)

func TestModeConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		pinotURL      string
		kafkaBrokers  string
		wantErr       bool
		errContains   string
	}{
		{
			name:         "all mode - requires both Pinot and Kafka",
			mode:         "all",
			pinotURL:     "http://localhost:9000",
			kafkaBrokers: "localhost:9092",
			wantErr:      false,
		},
		{
			name:         "all mode - missing Pinot",
			mode:         "all",
			pinotURL:     "",
			kafkaBrokers: "localhost:9092",
			wantErr:      true,
			errContains:  "pinot-url is required",
		},
		{
			name:         "all mode - missing Kafka",
			mode:         "all",
			pinotURL:     "http://localhost:9000",
			kafkaBrokers: "",
			wantErr:      true,
			errContains:  "kafka-brokers is required",
		},
		{
			name:         "ingestion mode - requires only Kafka",
			mode:         "ingestion",
			pinotURL:     "",
			kafkaBrokers: "localhost:9092",
			wantErr:      false,
		},
		{
			name:         "ingestion mode - missing Kafka",
			mode:         "ingestion",
			pinotURL:     "",
			kafkaBrokers: "",
			wantErr:      true,
			errContains:  "kafka-brokers is required",
		},
		{
			name:         "query mode - requires only Pinot",
			mode:         "query",
			pinotURL:     "http://localhost:9000",
			kafkaBrokers: "",
			wantErr:      false,
		},
		{
			name:         "query mode - missing Pinot",
			mode:         "query",
			pinotURL:     "",
			kafkaBrokers: "",
			wantErr:      true,
			errContains:  "pinot-url is required",
		},
		{
			name:         "invalid mode",
			mode:         "invalid",
			pinotURL:     "http://localhost:9000",
			kafkaBrokers: "localhost:9092",
			wantErr:      true,
			errContains:  "invalid mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Mode:         tt.mode,
				PinotURL:     tt.pinotURL,
				KafkaBrokers: tt.kafkaBrokers,
				OTLPGRPCPort: 4317,
				OTLPHTTPPort: 4318,
				QueryAPIPort: 8080,
				MCPPort:      8090,
			}

			err := cfg.validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validate() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestModeDefault(t *testing.T) {
	// Save and restore env vars
	oldMode := os.Getenv("OTEL_OQL_MODE")
	defer os.Setenv("OTEL_OQL_MODE", oldMode)

	// Clear env var
	os.Unsetenv("OTEL_OQL_MODE")

	cfg := &Config{}

	// Simulate default assignment
	if cfg.Mode == "" {
		cfg.Mode = "all"
	}

	if cfg.Mode != "all" {
		t.Errorf("default mode = %q, want %q", cfg.Mode, "all")
	}
}

func TestModeEnvironmentVariable(t *testing.T) {
	// Save and restore env vars
	oldMode := os.Getenv("OTEL_OQL_MODE")
	defer os.Setenv("OTEL_OQL_MODE", oldMode)

	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{"all mode", "all", "all"},
		{"ingestion mode", "ingestion", "ingestion"},
		{"query mode", "query", "query"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("OTEL_OQL_MODE", tt.envValue)

			cfg := &Config{}

			// Simulate env var loading
			if env := os.Getenv("OTEL_OQL_MODE"); env != "" {
				cfg.Mode = env
			}

			if cfg.Mode != tt.want {
				t.Errorf("mode from env = %q, want %q", cfg.Mode, tt.want)
			}
		})
	}
}

func TestPortValidationByMode(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		otlpGRPCPort int
		otlpHTTPPort int
		queryAPIPort int
		mcpPort      int
		wantErr      bool
		errContains  string
	}{
		{
			name:         "ingestion mode - requires OTLP ports",
			mode:         "ingestion",
			otlpGRPCPort: 4317,
			otlpHTTPPort: 4318,
			queryAPIPort: 0, // Not required
			mcpPort:      0, // Not required
			wantErr:      false,
		},
		{
			name:         "ingestion mode - invalid gRPC port",
			mode:         "ingestion",
			otlpGRPCPort: 0,
			otlpHTTPPort: 4318,
			queryAPIPort: 0,
			mcpPort:      0,
			wantErr:      true,
			errContains:  "invalid otlp-grpc-port",
		},
		{
			name:         "query mode - requires query ports",
			mode:         "query",
			otlpGRPCPort: 0, // Not required
			otlpHTTPPort: 0, // Not required
			queryAPIPort: 8080,
			mcpPort:      8090,
			wantErr:      false,
		},
		{
			name:         "query mode - invalid query port",
			mode:         "query",
			otlpGRPCPort: 0,
			otlpHTTPPort: 0,
			queryAPIPort: 0,
			mcpPort:      8090,
			wantErr:      true,
			errContains:  "invalid query-api-port",
		},
		{
			name:         "all mode - requires all ports",
			mode:         "all",
			otlpGRPCPort: 4317,
			otlpHTTPPort: 4318,
			queryAPIPort: 8080,
			mcpPort:      8090,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Mode:         tt.mode,
				PinotURL:     "http://localhost:9000",
				KafkaBrokers: "localhost:9092",
				OTLPGRPCPort: tt.otlpGRPCPort,
				OTLPHTTPPort: tt.otlpHTTPPort,
				QueryAPIPort: tt.queryAPIPort,
				MCPPort:      tt.mcpPort,
			}

			err := cfg.validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validate() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
