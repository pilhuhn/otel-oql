package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/pilhuhn/otel-oql/pkg/observability"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Ingester handles data ingestion to Kafka
type Ingester struct {
	producer sarama.SyncProducer
	obs      *observability.Observability
}

// NewIngester creates a new ingester with Kafka producer
func NewIngester(kafkaBrokers string, obs *observability.Observability) (*Ingester, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForLocal
	config.Producer.Compression = sarama.CompressionSnappy

	brokers := []string{kafkaBrokers}
	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &Ingester{
		producer: producer,
		obs:      obs,
	}, nil
}

// Close closes the Kafka producer
func (i *Ingester) Close() {
	if i.producer != nil {
		i.producer.Close()
	}
}

// IngestTraces ingests traces into Pinot
func (i *Ingester) IngestTraces(ctx context.Context, tenantID int, traces ptrace.Traces) error {
	ctx, span := i.obs.Tracer().Start(ctx, "ingestion.traces")
	defer span.End()

	records := make([]map[string]interface{}, 0)

	for k := 0; k < traces.ResourceSpans().Len(); k++ {
		rs := traces.ResourceSpans().At(k)
		resourceAttrs := rs.Resource().Attributes().AsRaw()

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)

			for idx := 0; idx < ss.Spans().Len(); idx++ {
				span := ss.Spans().At(idx)
				attrs := span.Attributes().AsRaw()

				// Debug: log all attributes
				if len(attrs) > 0 {
					attrsJSON, _ := json.Marshal(attrs)
					fmt.Printf("DEBUG INGESTION: Span %s attributes: %s\n", span.Name(), string(attrsJSON))
				}

				// Determine error status from span status code
				isError := span.Status().Code() == 2 // 2 = Error in OTLP

				// Extract HTTP status code - try both old and new semantic conventions
				httpStatusCode := extractInt(attrs, "http.status_code")
				if httpStatusCode == nil {
					httpStatusCode = extractInt(attrs, "http.response.status_code")
				}

				// Extract HTTP method - try both old and new conventions
				httpMethod := extractString(attrs, "http.method")
				if httpMethod == nil {
					httpMethod = extractString(attrs, "http.request.method")
				}

				fmt.Printf("DEBUG INGESTION: Span %s - error=%v, http_status=%v, http_method=%v\n",
					span.Name(), isError, httpStatusCode, httpMethod)

				record := map[string]interface{}{
					"tenant_id":      tenantID,
					"trace_id":       span.TraceID().String(),
					"span_id":        span.SpanID().String(),
					"parent_span_id": span.ParentSpanID().String(),
					"name":           span.Name(),
					"kind":           span.Kind().String(),
					"duration":       span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Nanoseconds(),
					"timestamp":      span.StartTimestamp().AsTime().UnixMilli(),
					"status_code":    span.Status().Code().String(),
					"status_message": span.Status().Message(),

					// Extract common semantic convention attributes
					"service_name":          extractString(resourceAttrs, "service.name"),
					"http_method":           httpMethod,
					"http_status_code":      httpStatusCode,
					"http_route":            extractString(attrs, "http.route"),
					"http_target":           extractString(attrs, "http.target"),
					"db_system":             extractString(attrs, "db.system"),
					"db_statement":          extractString(attrs, "db.statement"),
					"messaging_system":      extractString(attrs, "messaging.system"),
					"messaging_destination": extractString(attrs, "messaging.destination"),
					"rpc_service":           extractString(attrs, "rpc.service"),
					"rpc_method":            extractString(attrs, "rpc.method"),
					"error":                 isError,

					// Store remaining attributes as JSON
					"attributes":          removeKnownKeys(attrs, spanKnownKeys),
					"resource_attributes": removeKnownKeys(resourceAttrs, spanResourceKnownKeys),
				}

				records = append(records, record)
			}
		}
	}

	if len(records) == 0 {
		fmt.Println("DEBUG INGESTER: No spans to ingest")
		return nil
	}

	fmt.Printf("DEBUG INGESTER: Publishing %d span records to Kafka\n", len(records))

	// Publish records to Kafka
	for _, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			fmt.Printf("DEBUG INGESTER: Failed to marshal span: %v\n", err)
			return fmt.Errorf("failed to marshal span record: %w", err)
		}

		fmt.Printf("DEBUG INGESTER: Marshaled span record, %d bytes\n", len(payload))

		msg := &sarama.ProducerMessage{
			Topic: "otel-spans",
			Value: sarama.ByteEncoder(payload),
		}

		partition, offset, err := i.producer.SendMessage(msg)
		if err != nil {
			fmt.Printf("DEBUG INGESTER: Failed to send to Kafka: %v\n", err)
			return fmt.Errorf("failed to send span to Kafka: %w", err)
		}
		fmt.Printf("DEBUG INGESTER: Sent to Kafka partition=%d offset=%d\n", partition, offset)
	}

	fmt.Printf("DEBUG INGESTER: Successfully published %d spans to Kafka\n", len(records))

	// Record observability metrics
	i.obs.RecordIngestion(ctx, "spans", int64(len(records)))
	i.obs.RecordKafkaPublish(ctx, "otel-spans", int64(len(records)))

	return nil
}

// IngestMetrics ingests metrics into Pinot
func (i *Ingester) IngestMetrics(ctx context.Context, tenantID int, metrics pmetric.Metrics) error {
	ctx, span := i.obs.Tracer().Start(ctx, "ingestion.metrics")
	defer span.End()

	records := make([]map[string]interface{}, 0)

	for k := 0; k < metrics.ResourceMetrics().Len(); k++ {
		rm := metrics.ResourceMetrics().At(k)
		resourceAttrs := rm.Resource().Attributes().AsRaw()

		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			sm := rm.ScopeMetrics().At(j)

			for idx := 0; idx < sm.Metrics().Len(); idx++ {
				metric := sm.Metrics().At(idx)

				// Handle different metric types
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					records = append(records, i.convertGauge(tenantID, metric, resourceAttrs)...)
				case pmetric.MetricTypeSum:
					records = append(records, i.convertSum(tenantID, metric, resourceAttrs)...)
				case pmetric.MetricTypeHistogram:
					records = append(records, i.convertHistogram(tenantID, metric, resourceAttrs)...)
				}
			}
		}
	}

	if len(records) == 0 {
		fmt.Println("DEBUG INGESTER: No metrics to ingest")
		return nil
	}

	fmt.Printf("DEBUG INGESTER: Publishing %d metric records to Kafka\n", len(records))

	// Publish records to Kafka
	for _, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			fmt.Printf("DEBUG INGESTER: Failed to marshal metric: %v\n", err)
			return fmt.Errorf("failed to marshal metric record: %w", err)
		}

		msg := &sarama.ProducerMessage{
			Topic: "otel-metrics",
			Value: sarama.ByteEncoder(payload),
		}

		partition, offset, err := i.producer.SendMessage(msg)
		if err != nil {
			fmt.Printf("DEBUG INGESTER: Failed to send metric to Kafka: %v\n", err)
			return fmt.Errorf("failed to send metric to Kafka: %w", err)
		}
		fmt.Printf("DEBUG INGESTER: Sent metric to Kafka partition=%d offset=%d\n", partition, offset)
	}

	fmt.Printf("DEBUG INGESTER: Successfully published %d metrics to Kafka\n", len(records))

	// Record observability metrics
	i.obs.RecordIngestion(ctx, "metrics", int64(len(records)))
	i.obs.RecordKafkaPublish(ctx, "otel-metrics", int64(len(records)))

	return nil
}

// IngestLogs ingests logs into Pinot
func (i *Ingester) IngestLogs(ctx context.Context, tenantID int, logs plog.Logs) error {
	ctx, span := i.obs.Tracer().Start(ctx, "ingestion.logs")
	defer span.End()

	records := make([]map[string]interface{}, 0)

	for k := 0; k < logs.ResourceLogs().Len(); k++ {
		rl := logs.ResourceLogs().At(k)
		resourceAttrs := rl.Resource().Attributes().AsRaw()

		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)

			for idx := 0; idx < sl.LogRecords().Len(); idx++ {
				logRecord := sl.LogRecords().At(idx)
				attrs := logRecord.Attributes().AsRaw()

				record := map[string]interface{}{
					"tenant_id":       tenantID,
					"timestamp":       logRecord.Timestamp().AsTime().UnixMilli(),
					"trace_id":        logRecord.TraceID().String(),
					"span_id":         logRecord.SpanID().String(),
					"severity_number": logRecord.SeverityNumber(),
					"severity_text":   logRecord.SeverityText(),
					"body":            logRecord.Body().AsString(),

					// Extract common attributes
					"service_name": extractString(resourceAttrs, "service.name"),
					"host_name":    extractString(resourceAttrs, "host.name"),
					"log_level":    extractString(attrs, "log.level"),
					"log_source":   extractString(attrs, "log.source"),

					// Store remaining attributes as JSON
					"attributes":          removeKnownKeys(attrs, logKnownKeys),
					"resource_attributes": removeKnownKeys(resourceAttrs, logResourceKnownKeys),
				}

				records = append(records, record)
			}
		}
	}

	if len(records) == 0 {
		fmt.Println("DEBUG INGESTER: No logs to ingest")
		return nil
	}

	fmt.Printf("DEBUG INGESTER: Publishing %d log records to Kafka\n", len(records))

	// Publish records to Kafka
	for _, record := range records {
		payload, err := json.Marshal(record)
		if err != nil {
			fmt.Printf("DEBUG INGESTER: Failed to marshal log: %v\n", err)
			return fmt.Errorf("failed to marshal log record: %w", err)
		}

		msg := &sarama.ProducerMessage{
			Topic: "otel-logs",
			Value: sarama.ByteEncoder(payload),
		}

		partition, offset, err := i.producer.SendMessage(msg)
		if err != nil {
			fmt.Printf("DEBUG INGESTER: Failed to send log to Kafka: %v\n", err)
			return fmt.Errorf("failed to send log to Kafka: %w", err)
		}
		fmt.Printf("DEBUG INGESTER: Sent log to Kafka partition=%d offset=%d\n", partition, offset)
	}

	fmt.Printf("DEBUG INGESTER: Successfully published %d logs to Kafka\n", len(records))

	// Record observability metrics
	i.obs.RecordIngestion(ctx, "logs", int64(len(records)))
	i.obs.RecordKafkaPublish(ctx, "otel-logs", int64(len(records)))

	return nil
}

// convertGauge converts gauge metrics to Pinot records
func (i *Ingester) convertGauge(tenantID int, metric pmetric.Metric, resourceAttrs map[string]interface{}) []map[string]interface{} {
	records := make([]map[string]interface{}, 0)
	gauge := metric.Gauge()

	for j := 0; j < gauge.DataPoints().Len(); j++ {
		dp := gauge.DataPoints().At(j)
		attrs := dp.Attributes().AsRaw()

		record := map[string]interface{}{
			"tenant_id":   tenantID,
			"metric_name": metric.Name(),
			"metric_type": "gauge",
			"timestamp":   dp.Timestamp().AsTime().UnixMilli(),
			"value":       getDataPointValue(dp),

			// Extract common attributes
			"service_name": extractString(resourceAttrs, "service.name"),
			"host_name":    extractString(resourceAttrs, "host.name"),
			"environment":  extractString(attrs, "environment"),
			"job":          extractString(attrs, "job"),
			"instance":     extractString(attrs, "instance"),

			// Store remaining attributes as JSON
			"attributes":          removeKnownKeys(attrs, metricKnownKeys),
			"resource_attributes": removeKnownKeys(resourceAttrs, metricResourceKnownKeys),
		}

		// Add exemplars if present (the "wormhole" for trace correlation)
		if dp.Exemplars().Len() > 0 {
			exemplar := dp.Exemplars().At(0)
			if !exemplar.TraceID().IsEmpty() {
				record["exemplar_trace_id"] = exemplar.TraceID().String()
			}
			if !exemplar.SpanID().IsEmpty() {
				record["exemplar_span_id"] = exemplar.SpanID().String()
			}
		}

		records = append(records, record)
	}

	return records
}

// convertSum converts sum metrics to Pinot records
func (i *Ingester) convertSum(tenantID int, metric pmetric.Metric, resourceAttrs map[string]interface{}) []map[string]interface{} {
	records := make([]map[string]interface{}, 0)
	sum := metric.Sum()

	for j := 0; j < sum.DataPoints().Len(); j++ {
		dp := sum.DataPoints().At(j)
		attrs := dp.Attributes().AsRaw()

		record := map[string]interface{}{
			"tenant_id":   tenantID,
			"metric_name": metric.Name(),
			"metric_type": "sum",
			"timestamp":   dp.Timestamp().AsTime().UnixMilli(),
			"value":       getDataPointValue(dp),

			// Extract common attributes
			"service_name": extractString(resourceAttrs, "service.name"),
			"host_name":    extractString(resourceAttrs, "host.name"),
			"environment":  extractString(attrs, "environment"),
			"job":          extractString(attrs, "job"),
			"instance":     extractString(attrs, "instance"),

			// Store remaining attributes as JSON
			"attributes":          removeKnownKeys(attrs, metricKnownKeys),
			"resource_attributes": removeKnownKeys(resourceAttrs, metricResourceKnownKeys),
		}

		// Add exemplars if present (the "wormhole" for trace correlation)
		if dp.Exemplars().Len() > 0 {
			exemplar := dp.Exemplars().At(0)
			if !exemplar.TraceID().IsEmpty() {
				record["exemplar_trace_id"] = exemplar.TraceID().String()
			}
			if !exemplar.SpanID().IsEmpty() {
				record["exemplar_span_id"] = exemplar.SpanID().String()
			}
		}

		records = append(records, record)
	}

	return records
}

// convertHistogram converts histogram metrics to Pinot records
func (i *Ingester) convertHistogram(tenantID int, metric pmetric.Metric, resourceAttrs map[string]interface{}) []map[string]interface{} {
	records := make([]map[string]interface{}, 0)
	histogram := metric.Histogram()

	for j := 0; j < histogram.DataPoints().Len(); j++ {
		dp := histogram.DataPoints().At(j)
		attrs := dp.Attributes().AsRaw()

		record := map[string]interface{}{
			"tenant_id":   tenantID,
			"metric_name": metric.Name(),
			"metric_type": "histogram",
			"timestamp":   dp.Timestamp().AsTime().UnixMilli(),
			"count":       dp.Count(),
			"sum":         dp.Sum(),

			// Extract common attributes
			"service_name": extractString(resourceAttrs, "service.name"),
			"host_name":    extractString(resourceAttrs, "host.name"),
			"environment":  extractString(attrs, "environment"),
			"job":          extractString(attrs, "job"),
			"instance":     extractString(attrs, "instance"),

			// Store remaining attributes as JSON
			"attributes":          removeKnownKeys(attrs, metricKnownKeys),
			"resource_attributes": removeKnownKeys(resourceAttrs, metricResourceKnownKeys),
		}

		// Add exemplars if present (the "wormhole" for trace correlation)
		if dp.Exemplars().Len() > 0 {
			exemplar := dp.Exemplars().At(0)
			if !exemplar.TraceID().IsEmpty() {
				record["exemplar_trace_id"] = exemplar.TraceID().String()
			}
			if !exemplar.SpanID().IsEmpty() {
				record["exemplar_span_id"] = exemplar.SpanID().String()
			}
		}

		records = append(records, record)
	}

	return records
}

// getDataPointValue extracts the value from a NumberDataPoint
func getDataPointValue(dp pmetric.NumberDataPoint) float64 {
	switch dp.ValueType() {
	case pmetric.NumberDataPointValueTypeDouble:
		return dp.DoubleValue()
	case pmetric.NumberDataPointValueTypeInt:
		return float64(dp.IntValue())
	default:
		return 0
	}
}
