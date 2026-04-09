package common

import (
	"strings"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/sqlutil"
)

func TestMetricLabelDistinctExpr(t *testing.T) {
	t.Parallel()
	if got := MetricLabelDistinctExpr("__name__"); got != "metric_name" {
		t.Errorf("__name__: got %q", got)
	}
	if got := MetricLabelDistinctExpr("job"); got != "job" {
		t.Errorf("job: got %q", got)
	}
	unknown := MetricLabelDistinctExpr("custom_label")
	if !strings.HasPrefix(unknown, "JSON_EXTRACT_SCALAR(attributes, ") {
		t.Errorf("unknown: expected JSON_EXTRACT_SCALAR, got %q", unknown)
	}
	if !strings.Contains(unknown, sqlutil.JSONObjectKeyPathLiteral("custom_label")) {
		t.Errorf("unknown: literal not embedded safely: %q", unknown)
	}
	malicious := MetricLabelDistinctExpr("x) OR (1=1")
	wantLit := sqlutil.JSONObjectKeyPathLiteral("x) OR (1=1")
	if !strings.Contains(malicious, wantLit) {
		t.Errorf("expected escaped path literal in expression, got %q", malicious)
	}
}

func TestLogLabelDistinctExpr(t *testing.T) {
	t.Parallel()
	if got := LogLabelDistinctExpr("trace_id"); got != "trace_id" {
		t.Errorf("trace_id: got %q", got)
	}
	if got := LogLabelDistinctExpr("level"); got != "log_level" {
		t.Errorf("level: got %q", got)
	}
	unknown := LogLabelDistinctExpr("app")
	if !strings.HasPrefix(unknown, "JSON_EXTRACT_SCALAR(attributes, ") {
		t.Errorf("unknown: expected JSON_EXTRACT_SCALAR, got %q", unknown)
	}
}
