package api

import (
	"context"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/observability"
	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
)

func TestPrometheusLabels(t *testing.T) {
	// Create a test server (non-functional, just for testing handlers)
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

	// Test buildLabelsSQL
	params := &PrometheusLabelsParams{
		Limit: 100,
	}
	sql := s.buildLabelsSQL(0, params)

	if sql == "" {
		t.Error("Expected SQL query, got empty string")
	}

	if sql[:15] != "SELECT DISTINCT" {
		t.Errorf("Expected SQL to start with SELECT DISTINCT, got: %s", sql[:15])
	}
}

func TestPrometheusLabelValues(t *testing.T) {
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
			name:      "metric name (__name__)",
			labelName: "__name__",
			wantCol:   "metric_name",
		},
		{
			name:      "regular label",
			labelName: "service_name",
			wantCol:   "service_name",
		},
		{
			name:      "job label",
			labelName: "job",
			wantCol:   "job",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &PrometheusLabelValuesParams{
				LabelName: tt.labelName,
				Limit:     10,
			}
			sql := s.buildLabelValuesSQL(0, params)

			if sql == "" {
				t.Error("Expected SQL query, got empty string")
			}

			// Check that the correct column is selected
			expectedStart := "SELECT DISTINCT " + tt.wantCol
			if len(sql) < len(expectedStart) || sql[:len(expectedStart)] != expectedStart {
				t.Errorf("Expected SQL to start with %q, got: %s", expectedStart, sql)
			}
		})
	}
}
