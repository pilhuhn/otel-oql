package api

import (
	"testing"
)

func TestConvertOTelToPromQLMetricName(t *testing.T) {
	tests := []struct {
		name     string
		otelName string
		want     string
	}{
		{
			name:     "simple OTel metric name",
			otelName: "jvm.memory.used",
			want:     "jvm_memory_used",
		},
		{
			name:     "http server duration",
			otelName: "http.server.duration",
			want:     "http_server_duration",
		},
		{
			name:     "multiple dots",
			otelName: "system.cpu.utilization.percent",
			want:     "system_cpu_utilization_percent",
		},
		{
			name:     "no dots (already PromQL format)",
			otelName: "http_requests_total",
			want:     "http_requests_total",
		},
		{
			name:     "single segment",
			otelName: "requests",
			want:     "requests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertOTelToPromQLMetricName(tt.otelName)
			if got != tt.want {
				t.Errorf("convertOTelToPromQLMetricName(%q) = %q, want %q", tt.otelName, got, tt.want)
			}
		})
	}
}
