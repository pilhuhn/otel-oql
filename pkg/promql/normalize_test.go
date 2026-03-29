package promql

import (
	"testing"
)

func TestNormalizeMetricNames(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantNormalized  string
		wantMapping     map[string]string
	}{
		{
			name:           "simple OTel metric name",
			input:          "jvm.memory.used",
			wantNormalized: "jvm_memory_used",
			wantMapping: map[string]string{
				"jvm_memory_used": "jvm.memory.used",
			},
		},
		{
			name:           "OTel metric with labels",
			input:          `jvm.memory.used{job="api"}`,
			wantNormalized: `jvm_memory_used{job="api"}`,
			wantMapping: map[string]string{
				"jvm_memory_used": "jvm.memory.used",
			},
		},
		{
			name:           "multiple dots in metric name",
			input:          "http.server.request.duration",
			wantNormalized: "http_server_request_duration",
			wantMapping: map[string]string{
				"http_server_request_duration": "http.server.request.duration",
			},
		},
		{
			name:           "metric without dots (standard PromQL)",
			input:          "http_requests_total",
			wantNormalized: "http_requests_total",
			wantMapping:    map[string]string{},
		},
		{
			name:           "aggregation with OTel metric",
			input:          `sum(jvm.memory.used)`,
			wantNormalized: `sum(jvm_memory_used)`,
			wantMapping: map[string]string{
				"jvm_memory_used": "jvm.memory.used",
			},
		},
		{
			name:           "rate function with OTel metric",
			input:          `rate(http.server.duration[5m])`,
			wantNormalized: `rate(http_server_duration[5m])`,
			wantMapping: map[string]string{
				"http_server_duration": "http.server.duration",
			},
		},
		{
			name:           "complex query with multiple OTel metrics",
			input:          `jvm.memory.used / jvm.memory.max`,
			wantNormalized: `jvm_memory_used / jvm_memory_max`,
			wantMapping: map[string]string{
				"jvm_memory_used": "jvm.memory.used",
				"jvm_memory_max":  "jvm.memory.max",
			},
		},
		{
			name:           "mixed PromQL and OTel metrics",
			input:          `http_requests_total + http.server.requests`,
			wantNormalized: `http_requests_total + http_server_requests`,
			wantMapping: map[string]string{
				"http_server_requests": "http.server.requests",
			},
		},
		{
			name:           "OTel metric with by clause",
			input:          `sum by (service_name) (jvm.memory.used)`,
			wantNormalized: `sum by (service_name) (jvm_memory_used)`,
			wantMapping: map[string]string{
				"jvm_memory_used": "jvm.memory.used",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, mapping := normalizeMetricNames(tt.input)

			if normalized != tt.wantNormalized {
				t.Errorf("normalizeMetricNames() normalized = %q, want %q", normalized, tt.wantNormalized)
			}

			if len(mapping) != len(tt.wantMapping) {
				t.Errorf("normalizeMetricNames() mapping length = %d, want %d", len(mapping), len(tt.wantMapping))
			}

			for key, expectedValue := range tt.wantMapping {
				if actualValue, ok := mapping[key]; !ok {
					t.Errorf("normalizeMetricNames() mapping missing key %q", key)
				} else if actualValue != expectedValue {
					t.Errorf("normalizeMetricNames() mapping[%q] = %q, want %q", key, actualValue, expectedValue)
				}
			}
		})
	}
}
