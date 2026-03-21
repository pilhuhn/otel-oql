package ingestion

import (
	"context"

	"github.com/pilhuhn/otel-oql/pkg/pinot"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Ingester handles data ingestion to Pinot
type Ingester struct {
	client *pinot.Client
}

// NewIngester creates a new ingester
func NewIngester(client *pinot.Client) *Ingester {
	return &Ingester{
		client: client,
	}
}

// IngestTraces ingests traces into Pinot
func (i *Ingester) IngestTraces(ctx context.Context, tenantID int, traces ptrace.Traces) error {
	records := make([]map[string]interface{}, 0)

	for k := 0; k < traces.ResourceSpans().Len(); k++ {
		rs := traces.ResourceSpans().At(k)
		resourceAttrs := rs.Resource().Attributes().AsRaw()

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)

			for idx := 0; idx < ss.Spans().Len(); idx++ {
				span := ss.Spans().At(idx)

				record := map[string]interface{}{
					"tenant_id":  tenantID,
					"trace_id":   span.TraceID().String(),
					"span_id":    span.SpanID().String(),
					"parent_span_id": span.ParentSpanID().String(),
					"name":       span.Name(),
					"kind":       span.Kind().String(),
					"start_time": span.StartTimestamp().AsTime().UnixNano(),
					"end_time":   span.EndTimestamp().AsTime().UnixNano(),
					"duration":   span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Nanoseconds(),
					"timestamp":  span.StartTimestamp().AsTime().UnixMilli(),
					"attributes": span.Attributes().AsRaw(),
					"resource_attributes": resourceAttrs,
					"status_code": span.Status().Code().String(),
					"status_message": span.Status().Message(),
				}

				records = append(records, record)
			}
		}
	}

	if len(records) == 0 {
		return nil
	}

	return i.client.Insert(ctx, "otel_spans", records)
}

// IngestMetrics ingests metrics into Pinot
func (i *Ingester) IngestMetrics(ctx context.Context, tenantID int, metrics pmetric.Metrics) error {
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
		return nil
	}

	return i.client.Insert(ctx, "otel_metrics", records)
}

// IngestLogs ingests logs into Pinot
func (i *Ingester) IngestLogs(ctx context.Context, tenantID int, logs plog.Logs) error {
	records := make([]map[string]interface{}, 0)

	for k := 0; k < logs.ResourceLogs().Len(); k++ {
		rl := logs.ResourceLogs().At(k)
		resourceAttrs := rl.Resource().Attributes().AsRaw()

		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)

			for idx := 0; idx < sl.LogRecords().Len(); idx++ {
				logRecord := sl.LogRecords().At(idx)

				record := map[string]interface{}{
					"tenant_id":  tenantID,
					"timestamp":  logRecord.Timestamp().AsTime().UnixMilli(),
					"trace_id":   logRecord.TraceID().String(),
					"span_id":    logRecord.SpanID().String(),
					"severity_number": logRecord.SeverityNumber(),
					"severity_text": logRecord.SeverityText(),
					"body":       logRecord.Body().AsString(),
					"attributes": logRecord.Attributes().AsRaw(),
					"resource_attributes": resourceAttrs,
				}

				records = append(records, record)
			}
		}
	}

	if len(records) == 0 {
		return nil
	}

	return i.client.Insert(ctx, "otel_logs", records)
}

// convertGauge converts gauge metrics to Pinot records
func (i *Ingester) convertGauge(tenantID int, metric pmetric.Metric, resourceAttrs map[string]interface{}) []map[string]interface{} {
	records := make([]map[string]interface{}, 0)
	gauge := metric.Gauge()

	for j := 0; j < gauge.DataPoints().Len(); j++ {
		dp := gauge.DataPoints().At(j)
		record := map[string]interface{}{
			"tenant_id":    tenantID,
			"metric_name":  metric.Name(),
			"metric_type":  "gauge",
			"timestamp":    dp.Timestamp().AsTime().UnixMilli(),
			"value":        getDataPointValue(dp),
			"attributes":   dp.Attributes().AsRaw(),
			"resource_attributes": resourceAttrs,
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
		record := map[string]interface{}{
			"tenant_id":    tenantID,
			"metric_name":  metric.Name(),
			"metric_type":  "sum",
			"timestamp":    dp.Timestamp().AsTime().UnixMilli(),
			"value":        getDataPointValue(dp),
			"attributes":   dp.Attributes().AsRaw(),
			"resource_attributes": resourceAttrs,
		}

		// Add exemplars if present
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
		record := map[string]interface{}{
			"tenant_id":    tenantID,
			"metric_name":  metric.Name(),
			"metric_type":  "histogram",
			"timestamp":    dp.Timestamp().AsTime().UnixMilli(),
			"count":        dp.Count(),
			"sum":          dp.Sum(),
			"attributes":   dp.Attributes().AsRaw(),
			"resource_attributes": resourceAttrs,
		}

		// Add exemplars if present
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
