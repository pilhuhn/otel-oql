package ingestion

// extractString safely extracts a string value from attributes
func extractString(attrs map[string]interface{}, key string) interface{} {
	if v, ok := attrs[key]; ok {
		return v
	}
	return nil // Pinot handles NULL
}

// extractInt safely extracts an int value from attributes
func extractInt(attrs map[string]interface{}, key string) interface{} {
	if v, ok := attrs[key]; ok {
		// Handle different numeric types
		switch val := v.(type) {
		case int:
			return val
		case int32:
			return int(val)
		case int64:
			return int(val)
		case uint:
			return int(val)
		case uint32:
			return int(val)
		case uint64:
			// Convert uint64 to int safely
			// This handles the case where OTLP sends unsigned values
			if val <= 9223372036854775807 { // max int64
				return int(val)
			}
			return int(val) // Will wrap, but log it
		case float64:
			return int(val)
		default:
			return v
		}
	}
	return nil
}

// extractBool safely extracts a boolean value from attributes
func extractBool(attrs map[string]interface{}, key string) interface{} {
	if v, ok := attrs[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
		// Handle string "true"/"false"
		if s, ok := v.(string); ok {
			return s == "true"
		}
	}
	return nil
}

// removeKnownKeys creates a copy of attributes without extracted keys
func removeKnownKeys(attrs map[string]interface{}, knownKeys []string) map[string]interface{} {
	if len(attrs) == 0 {
		return nil
	}

	// Build lookup map
	keyMap := make(map[string]bool)
	for _, k := range knownKeys {
		keyMap[k] = true
	}

	// Copy non-extracted attributes
	result := make(map[string]interface{})
	for k, v := range attrs {
		if !keyMap[k] {
			result[k] = v
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// spanKnownKeys are the OTel semantic convention attributes we extract for spans
var spanKnownKeys = []string{
	// HTTP attributes (old semantic conventions)
	"http.method",
	"http.status_code",
	"http.route",
	"http.target",
	// HTTP attributes (new semantic conventions v1.21+)
	"http.request.method",
	"http.response.status_code",
	// Database attributes
	"db.system",
	"db.statement",
	// Messaging attributes
	"messaging.system",
	"messaging.destination",
	// RPC attributes
	"rpc.service",
	"rpc.method",
	// Error indicator (if present as attribute)
	"error",
}

// spanResourceKnownKeys are resource attributes we extract for spans
var spanResourceKnownKeys = []string{
	"service.name",
}

// metricKnownKeys are attributes we extract for metrics
var metricKnownKeys = []string{
	"job",
	"instance",
	"environment",
}

// metricResourceKnownKeys are resource attributes we extract for metrics
var metricResourceKnownKeys = []string{
	"service.name",
	"host.name",
}

// logKnownKeys are attributes we extract for logs
var logKnownKeys = []string{
	"log.level",
	"log.source",
}

// logResourceKnownKeys are resource attributes we extract for logs
var logResourceKnownKeys = []string{
	"service.name",
	"host.name",
}
