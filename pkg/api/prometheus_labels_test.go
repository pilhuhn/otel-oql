package api

import (
	"context"
	"strings"
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
		{
			name:      "unknown label uses JSON not identifier",
			labelName: "custom_attr",
			wantCol:   "JSON_EXTRACT_SCALAR(attributes, '$.custom_attr', 'STRING')",
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

			expectedStart := "SELECT DISTINCT " + tt.wantCol
			if len(sql) < len(expectedStart) || sql[:len(expectedStart)] != expectedStart {
				t.Errorf("Expected SQL to start with %q, got: %s", expectedStart, sql)
			}
		})
	}
}

func TestPrometheusLabelValuesSQLInjectionLabelNameNotIdentifier(t *testing.T) {
	obs, _ := observability.New(context.Background(), observability.Config{
		ServiceName: "test",
		Enabled:     false,
	})
	defer obs.Shutdown(context.Background())

	s := &Server{
		pinotClient:      pinot.NewClient("http://localhost:9000"),
		validator:        tenant.NewValidator(true),
		obs:              obs,
		debugQuery:       false,
		debugTranslation: false,
	}

	sql := s.buildLabelValuesSQL(0, &PrometheusLabelValuesParams{
		LabelName: "bad'); SELECT 1; --",
		Limit:     5,
	})
	if !strings.HasPrefix(sql, "SELECT DISTINCT JSON_EXTRACT_SCALAR(attributes,") {
		t.Fatalf("unknown label must use JSON_EXTRACT_SCALAR, not bare identifier: %s", sql)
	}
	if !strings.Contains(sql, "tenant_id = 0") {
		t.Fatalf("expected tenant filter: %s", sql)
	}
}
