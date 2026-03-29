package api

import (
	"context"
	"strings"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/observability"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
)

// TestLokiLabels tests the /loki/api/v1/labels endpoint
func TestLokiLabels(t *testing.T) {
	// Create a test server
	obs, _ := observability.New(context.Background(), observability.Config{
		ServiceName: "test",
		Enabled:     false,
	})
	defer obs.Shutdown(context.Background())

	client := pinot.NewClient("http://localhost:9000")
	validator := tenant.NewValidator(true)

	s := &Server{
		pinotClient:      client,
		validator:        validator,
		obs:              obs,
		debugQuery:       false,
		debugTranslation: false,
	}

	// Test buildLokiLabelsSQL
	params := &LokiLabelsParams{
		Limit: 100,
	}
	sql := s.buildLokiLabelsSQL(0, params)

	if sql == "" {
		t.Error("Expected SQL query, got empty string")
	}

	if !strings.Contains(sql, "SELECT DISTINCT") {
		t.Errorf("Expected SQL to contain SELECT DISTINCT, got: %s", sql)
	}

	if !strings.Contains(sql, "FROM otel_logs") {
		t.Errorf("Expected SQL to query otel_logs table, got: %s", sql)
	}

	if !strings.Contains(sql, "WHERE tenant_id = 0") {
		t.Errorf("Expected SQL to filter by tenant_id, got: %s", sql)
	}
}

// TestLokiLabelValues tests the /loki/api/v1/label/{name}/values endpoint
func TestLokiLabelValues(t *testing.T) {
	obs, _ := observability.New(context.Background(), observability.Config{
		ServiceName: "test",
		Enabled:     false,
	})
	defer obs.Shutdown(context.Background())

	client := pinot.NewClient("http://localhost:9000")
	validator := tenant.NewValidator(true)

	s := &Server{
		pinotClient:      client,
		validator:        validator,
		obs:              obs,
		debugQuery:       false,
		debugTranslation: false,
	}

	tests := []struct {
		name      string
		labelName string
		wantCol   string
	}{
		{
			name:      "service_name label",
			labelName: "service_name",
			wantCol:   "service_name",
		},
		{
			name:      "job label (maps to service_name)",
			labelName: "job",
			wantCol:   "service_name",
		},
		{
			name:      "level label (maps to log_level)",
			labelName: "level",
			wantCol:   "log_level",
		},
		{
			name:      "trace_id label",
			labelName: "trace_id",
			wantCol:   "trace_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &LokiLabelValuesParams{
				LabelName: tt.labelName,
				Limit:     10,
			}
			sql := s.buildLokiLabelValuesSQL(0, params)

			if sql == "" {
				t.Error("Expected SQL query, got empty string")
			}

			// Check that the correct column is selected
			expectedStart := "SELECT DISTINCT " + tt.wantCol
			if !strings.HasPrefix(sql, expectedStart) {
				t.Errorf("Expected SQL to start with %q, got: %s", expectedStart, sql)
			}

			if !strings.Contains(sql, "FROM otel_logs") {
				t.Errorf("Expected SQL to query otel_logs table, got: %s", sql)
			}
		})
	}
}
