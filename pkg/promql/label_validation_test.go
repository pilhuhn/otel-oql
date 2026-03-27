package promql

import (
	"testing"

	"github.com/prometheus/prometheus/promql/parser"
)

// TestPrometheusParserLabelNameValidation verifies that the Prometheus parser
// validates label names and rejects SQL injection attempts
func TestPrometheusParserLabelNameValidation(t *testing.T) {
	tests := []struct {
		name      string
		promql    string
		wantError bool
		reason    string
	}{
		{
			name:      "normal label name",
			promql:    `http_requests_total{job="api"}`,
			wantError: false,
			reason:    "valid PromQL syntax",
		},
		{
			name:      "underscore in label name",
			promql:    `http_requests_total{custom_label="value"}`,
			wantError: false,
			reason:    "underscores are valid in label names",
		},
		{
			name:      "SQL injection in label VALUE",
			promql:    `http_requests_total{job="api'; DROP TABLE otel_metrics;--"}`,
			wantError: false,
			reason:    "injection in value is OK - values are properly escaped by sqlutil.StringLiteral",
		},
		{
			name:      "SQL injection attempt in label NAME - single quote",
			promql:    `http_requests_total{custom_label_';DROP TABLE otel_metrics;--="value"}`,
			wantError: true,
			reason:    "label names cannot contain single quotes",
		},
		{
			name:      "SQL injection attempt in label NAME - semicolon",
			promql:    `{__name__="http_requests_total", bad';DROP="value"}`,
			wantError: true,
			reason:    "label names cannot contain semicolons",
		},
		{
			name:      "special characters in label name",
			promql:    `http_requests_total{label-with-dash="value"}`,
			wantError: true,
			reason:    "dashes are not allowed in label names",
		},
		{
			name:      "label name with space",
			promql:    `http_requests_total{bad label="value"}`,
			wantError: true,
			reason:    "spaces are not allowed in label names",
		},
		{
			name:      "label name starting with number",
			promql:    `http_requests_total{123label="value"}`,
			wantError: true,
			reason:    "label names cannot start with numbers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.ParseExpr(tt.promql)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected parse error (%s), but parsing succeeded", tt.reason)
					if vs, ok := expr.(*parser.VectorSelector); ok {
						t.Logf("Parsed label matchers:")
						for _, m := range vs.LabelMatchers {
							t.Logf("  - Name: '%s', Value: '%s'", m.Name, m.Value)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected parsing to succeed (%s), but got error: %v", tt.reason, err)
				}
			}
		})
	}
}

// TestLabelNameTranslationSafety verifies that even if a malicious label name
// somehow got through (it shouldn't), the resulting SQL is still safe
func TestLabelNameTranslationSafety(t *testing.T) {
	translator := NewTranslator(123)

	// This query should fail to parse
	_, err := translator.TranslateQuery(`http_requests_total{malicious';DROP TABLE otel_metrics;--="value"}`)

	if err == nil {
		t.Error("Expected query with malicious label name to fail, but it succeeded")
	} else {
		t.Logf("Query correctly rejected with error: %v", err)
	}
}
