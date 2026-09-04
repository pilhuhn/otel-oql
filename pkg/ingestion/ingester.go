package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pilhuhn/otel-oql/pkg/clickhouse"
	"github.com/pilhuhn/otel-oql/pkg/observability"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Ingester handles direct data ingestion to Clickhouse
type Ingester struct {
	client         *clickhouse.Client
	obs            *observability.Observability
	debugIngestion bool
}

// NewIngester creates a new ingester with Clickhouse client
func NewIngester(clickhouseURL string, obs *observability.Observability, debugIngestion bool) (*Ingester, error) {
	client := clickhouse.NewClient(clickhouseURL)
	return &Ingester{
		client:         client,
		obs:            obs,
		debugIngestion: debugIngestion,
	}, nil
}

// Close is a no-op (Clickhouse HTTP client has no persistent connection to close)
func (i *Ingester) Close() {}

// IngestTraces ingests traces into Clickhouse
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

				if i.debugIngestion && len(attrs) > 0 {
					attrsJSON, _ := json.Marshal(attrs)
					fmt.Printf("[DEBUG INGESTION] Span %s attributes: %s\n", span.Name(), string(attrsJSON))
				}

				isError := span.Status().Code() == 2 // 2 = Error in OTLP

				httpStatusCode := extractInt(attrs, "http.status_code")
				if httpStatusCode == nil {
					httpStatusCode = extractInt(attrs, "http.response.status_code")
				}

				httpMethod := extractString(attrs, "http.method")
				if httpMethod == nil {
					httpMethod = extractString(attrs, "http.request.method")
				}

				if i.debugIngestion {
					fmt.Printf("[DEBUG INGESTION] Span %s - error=%v, http_status=%v, http_method=%v\n",
						span.Name(), isError, httpStatusCode, httpMethod)
				}

				remainingAttrs := removeKnownKeys(attrs, spanKnownKeys)
				attrsJSON, _ := json.Marshal(remainingAttrs)
				remainingResourceAttrs := removeKnownKeys(resourceAttrs, spanResourceKnownKeys)
				resourceAttrsJSON, _ := json.Marshal(remainingResourceAttrs)

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

					"service_name":          stringOrEmpty(extractString(resourceAttrs, "service.name")),
					"http_method":           stringOrEmpty(httpMethod),
					"http_status_code":      httpStatusCode, // nil → Nullable null
					"http_route":            stringOrEmpty(extractString(attrs, "http.route")),
					"http_target":           stringOrEmpty(extractString(attrs, "http.target")),
					"db_system":             stringOrEmpty(extractString(attrs, "db.system")),
					"db_statement":          stringOrEmpty(extractString(attrs, "db.statement")),
					"messaging_system":      stringOrEmpty(extractString(attrs, "messaging.system")),
					"messaging_destination": stringOrEmpty(extractString(attrs, "messaging.destination")),
					"rpc_service":           stringOrEmpty(extractString(attrs, "rpc.service")),
					"rpc_method":            stringOrEmpty(extractString(attrs, "rpc.method")),
					"error":                 isError,

					"attributes":          string(attrsJSON),
					"resource_attributes": string(resourceAttrsJSON),
				}

				records = append(records, record)
			}
		}
	}

	if len(records) == 0 {
		if i.debugIngestion {
			fmt.Println("[DEBUG INGESTION] No spans to ingest")
		}
		return nil
	}

	if i.debugIngestion {
		fmt.Printf("[DEBUG INGESTION] Inserting %d span records into Clickhouse\n", len(records))
	}

	if err := i.client.Insert(ctx, "otel_spans", records); err != nil {
		return fmt.Errorf("failed to insert spans into Clickhouse: %w", err)
	}

	if i.debugIngestion {
		fmt.Printf("[DEBUG INGESTION] Successfully inserted %d spans\n", len(records))
	}

	i.obs.RecordIngestion(ctx, "spans", int64(len(records)))
	return nil
}

// IngestMetrics ingests metrics into Clickhouse
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
		if i.debugIngestion {
			fmt.Println("[DEBUG INGESTION] No metrics to ingest")
		}
		return nil
	}

	if i.debugIngestion {
		fmt.Printf("[DEBUG INGESTION] Inserting %d metric records into Clickhouse\n", len(records))
	}

	if err := i.client.Insert(ctx, "otel_metrics", records); err != nil {
		return fmt.Errorf("failed to insert metrics into Clickhouse: %w", err)
	}

	if i.debugIngestion {
		fmt.Printf("[DEBUG INGESTION] Successfully inserted %d metrics\n", len(records))
	}

	i.obs.RecordIngestion(ctx, "metrics", int64(len(records)))
	return nil
}

// IngestLogs ingests logs into Clickhouse
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

				remainingAttrs := removeKnownKeys(attrs, logKnownKeys)
				attrsJSON, _ := json.Marshal(remainingAttrs)
				remainingResourceAttrs := removeKnownKeys(resourceAttrs, logResourceKnownKeys)
				resourceAttrsJSON, _ := json.Marshal(remainingResourceAttrs)

				record := map[string]interface{}{
					"tenant_id":       tenantID,
					"timestamp":       logRecord.Timestamp().AsTime().UnixMilli(),
					"trace_id":        logRecord.TraceID().String(),
					"span_id":         logRecord.SpanID().String(),
					"severity_number": int32(logRecord.SeverityNumber()),
					"severity_text":   logRecord.SeverityText(),
					"body":            logRecord.Body().AsString(),

					"service_name": stringOrEmpty(extractString(resourceAttrs, "service.name")),
					"host_name":    stringOrEmpty(extractString(resourceAttrs, "host.name")),
					"log_level":    stringOrEmpty(extractString(attrs, "log.level")),
					"log_source":   stringOrEmpty(extractString(attrs, "log.source")),
					"job":          stringOrEmpty(extractString(attrs, "job")),
					"instance":     stringOrEmpty(extractString(attrs, "instance")),
					"environment":  stringOrEmpty(extractString(attrs, "environment")),

					"attributes":          string(attrsJSON),
					"resource_attributes": string(resourceAttrsJSON),
				}

				records = append(records, record)
			}
		}
	}

	if len(records) == 0 {
		if i.debugIngestion {
			fmt.Println("[DEBUG INGESTION] No logs to ingest")
		}
		return nil
	}

	if i.debugIngestion {
		fmt.Printf("[DEBUG INGESTION] Inserting %d log records into Clickhouse\n", len(records))
	}

	if err := i.client.Insert(ctx, "otel_logs", records); err != nil {
		return fmt.Errorf("failed to insert logs into Clickhouse: %w", err)
	}

	if i.debugIngestion {
		fmt.Printf("[DEBUG INGESTION] Successfully inserted %d logs\n", len(records))
	}

	i.obs.RecordIngestion(ctx, "logs", int64(len(records)))
	return nil
}

// stringOrEmpty returns the string value from an interface{} or "" if nil
func stringOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// convertGauge converts gauge metrics to Clickhouse records
func (i *Ingester) convertGauge(tenantID int, metric pmetric.Metric, resourceAttrs map[string]interface{}) []map[string]interface{} {
	records := make([]map[string]interface{}, 0)
	gauge := metric.Gauge()

	for j := 0; j < gauge.DataPoints().Len(); j++ {
		dp := gauge.DataPoints().At(j)
		attrs := dp.Attributes().AsRaw()

		remainingAttrs := removeKnownKeys(attrs, metricKnownKeys)
		attrsJSON, _ := json.Marshal(remainingAttrs)
		remainingResourceAttrs := removeKnownKeys(resourceAttrs, metricResourceKnownKeys)
		resourceAttrsJSON, _ := json.Marshal(remainingResourceAttrs)

		record := map[string]interface{}{
			"tenant_id":   tenantID,
			"metric_name": metric.Name(),
			"metric_type": "gauge",
			"timestamp":   dp.Timestamp().AsTime().UnixMilli(),
			"value":       getDataPointValue(dp),
			"count":       uint64(0),
			"sum":         float64(0),

			"service_name": stringOrEmpty(extractString(resourceAttrs, "service.name")),
			"host_name":    stringOrEmpty(extractString(resourceAttrs, "host.name")),
			"environment":  stringOrEmpty(extractString(attrs, "environment")),
			"job":          stringOrEmpty(extractString(attrs, "job")),
			"instance":     stringOrEmpty(extractString(attrs, "instance")),

			"exemplar_trace_id": "",
			"exemplar_span_id":  "",

			"attributes":          string(attrsJSON),
			"resource_attributes": string(resourceAttrsJSON),
		}

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

// convertSum converts sum metrics to Clickhouse records
func (i *Ingester) convertSum(tenantID int, metric pmetric.Metric, resourceAttrs map[string]interface{}) []map[string]interface{} {
	records := make([]map[string]interface{}, 0)
	sum := metric.Sum()

	for j := 0; j < sum.DataPoints().Len(); j++ {
		dp := sum.DataPoints().At(j)
		attrs := dp.Attributes().AsRaw()

		remainingAttrs := removeKnownKeys(attrs, metricKnownKeys)
		attrsJSON, _ := json.Marshal(remainingAttrs)
		remainingResourceAttrs := removeKnownKeys(resourceAttrs, metricResourceKnownKeys)
		resourceAttrsJSON, _ := json.Marshal(remainingResourceAttrs)

		record := map[string]interface{}{
			"tenant_id":   tenantID,
			"metric_name": metric.Name(),
			"metric_type": "sum",
			"timestamp":   dp.Timestamp().AsTime().UnixMilli(),
			"value":       getDataPointValue(dp),
			"count":       uint64(0),
			"sum":         float64(0),

			"service_name": stringOrEmpty(extractString(resourceAttrs, "service.name")),
			"host_name":    stringOrEmpty(extractString(resourceAttrs, "host.name")),
			"environment":  stringOrEmpty(extractString(attrs, "environment")),
			"job":          stringOrEmpty(extractString(attrs, "job")),
			"instance":     stringOrEmpty(extractString(attrs, "instance")),

			"exemplar_trace_id": "",
			"exemplar_span_id":  "",

			"attributes":          string(attrsJSON),
			"resource_attributes": string(resourceAttrsJSON),
		}

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

// convertHistogram converts histogram metrics to Clickhouse records
func (i *Ingester) convertHistogram(tenantID int, metric pmetric.Metric, resourceAttrs map[string]interface{}) []map[string]interface{} {
	records := make([]map[string]interface{}, 0)
	histogram := metric.Histogram()

	for j := 0; j < histogram.DataPoints().Len(); j++ {
		dp := histogram.DataPoints().At(j)
		attrs := dp.Attributes().AsRaw()

		remainingAttrs := removeKnownKeys(attrs, metricKnownKeys)
		attrsJSON, _ := json.Marshal(remainingAttrs)
		remainingResourceAttrs := removeKnownKeys(resourceAttrs, metricResourceKnownKeys)
		resourceAttrsJSON, _ := json.Marshal(remainingResourceAttrs)

		record := map[string]interface{}{
			"tenant_id":   tenantID,
			"metric_name": metric.Name(),
			"metric_type": "histogram",
			"timestamp":   dp.Timestamp().AsTime().UnixMilli(),
			"value":       float64(0),
			"count":       dp.Count(),
			"sum":         dp.Sum(),

			"service_name": stringOrEmpty(extractString(resourceAttrs, "service.name")),
			"host_name":    stringOrEmpty(extractString(resourceAttrs, "host.name")),
			"environment":  stringOrEmpty(extractString(attrs, "environment")),
			"job":          stringOrEmpty(extractString(attrs, "job")),
			"instance":     stringOrEmpty(extractString(attrs, "instance")),

			"exemplar_trace_id": "",
			"exemplar_span_id":  "",

			"attributes":          string(attrsJSON),
			"resource_attributes": string(resourceAttrsJSON),
		}

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
