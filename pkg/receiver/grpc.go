package receiver

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/ingestion"
	"github.com/pilhuhn/otel-oql/pkg/observability"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
)

// GRPCReceiver implements OTLP gRPC receiver
type GRPCReceiver struct {
	port      int
	validator *tenant.Validator
	ingester  *ingestion.Ingester
	server    *grpc.Server
	obs       *observability.Observability
}

// NewGRPCReceiver creates a new OTLP gRPC receiver
func NewGRPCReceiver(port int, validator *tenant.Validator, ingester *ingestion.Ingester, obs *observability.Observability) *GRPCReceiver {
	return &GRPCReceiver{
		port:      port,
		validator: validator,
		ingester:  ingester,
		obs:       obs,
	}
}

// Start starts the gRPC server
func (r *GRPCReceiver) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", r.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", r.port, err)
	}

	r.server = grpc.NewServer(
		grpc.UnaryInterceptor(r.validator.GRPCUnaryInterceptor()),
	)

	// Register OTLP services with separate handlers
	ptraceotlp.RegisterGRPCServer(r.server, &traceHandler{ingester: r.ingester, obs: r.obs})
	pmetricotlp.RegisterGRPCServer(r.server, &metricHandler{ingester: r.ingester, obs: r.obs})
	plogotlp.RegisterGRPCServer(r.server, &logHandler{ingester: r.ingester, obs: r.obs})

	go func() {
		if err := r.server.Serve(lis); err != nil {
			fmt.Printf("gRPC server error: %v\n", err)
		}
	}()

	fmt.Printf("OTLP gRPC receiver listening on port %d\n", r.port)
	return nil
}

// Stop stops the gRPC server
func (r *GRPCReceiver) Stop(ctx context.Context) error {
	if r.server != nil {
		r.server.GracefulStop()
	}
	return nil
}

// traceHandler implements the trace service
type traceHandler struct {
	ptraceotlp.UnimplementedGRPCServer
	ingester *ingestion.Ingester
	obs      *observability.Observability
}

func (h *traceHandler) Export(ctx context.Context, req ptraceotlp.ExportRequest) (ptraceotlp.ExportResponse, error) {
	start := time.Now()
	ctx, span := h.obs.Tracer().Start(ctx, "grpc.traces.export")
	defer span.End()

	traces := req.Traces()

	// DEBUG: Print incoming trace data
	fmt.Println("\n========== INCOMING OTLP TRACE (gRPC:4317) ==========")
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		fmt.Printf("ResourceSpan #%d:\n", i+1)
		fmt.Printf("  Resource Attributes: %+v\n", rs.Resource().Attributes().AsRaw())

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			fmt.Printf("  ScopeSpan #%d: %d spans\n", j+1, ss.Spans().Len())

			for k := 0; k < ss.Spans().Len(); k++ {
				s := ss.Spans().At(k)
				fmt.Printf("    Span #%d:\n", k+1)
				fmt.Printf("      Name: %s\n", s.Name())
				fmt.Printf("      TraceID: %s\n", s.TraceID().String())
				fmt.Printf("      SpanID: %s\n", s.SpanID().String())
				fmt.Printf("      ParentSpanID: %s\n", s.ParentSpanID().String())
				fmt.Printf("      Kind: %s\n", s.Kind().String())
				fmt.Printf("      Status: %s (%s)\n", s.Status().Code().String(), s.Status().Message())
				fmt.Printf("      StartTime: %s\n", s.StartTimestamp().AsTime())
				fmt.Printf("      EndTime: %s\n", s.EndTimestamp().AsTime())
				fmt.Printf("      Attributes: %+v\n", s.Attributes().AsRaw())
			}
		}
	}
	fmt.Println("=====================================================")

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		h.obs.RecordError(ctx, "missing_tenant_id", "grpc_receiver")
		return ptraceotlp.NewExportResponse(), fmt.Errorf("tenant-id not found in context")
	}

	// Ingest traces
	if err := h.ingester.IngestTraces(ctx, tenantID, traces); err != nil {
		h.obs.RecordError(ctx, "ingest_failure", "grpc_receiver")
		return ptraceotlp.NewExportResponse(), fmt.Errorf("failed to ingest traces: %w", err)
	}

	h.obs.RecordRequest(ctx, "/v1/traces", time.Since(start), 200)
	return ptraceotlp.NewExportResponse(), nil
}

// metricHandler implements the metrics service
type metricHandler struct {
	pmetricotlp.UnimplementedGRPCServer
	ingester *ingestion.Ingester
	obs      *observability.Observability
}

func (h *metricHandler) Export(ctx context.Context, req pmetricotlp.ExportRequest) (pmetricotlp.ExportResponse, error) {
	start := time.Now()
	ctx, span := h.obs.Tracer().Start(ctx, "grpc.metrics.export")
	defer span.End()

	metrics := req.Metrics()

	// DEBUG: Print incoming metric data
	fmt.Println("\n========== INCOMING OTLP METRIC (gRPC:4317) ==========")
	for i := 0; i < metrics.ResourceMetrics().Len(); i++ {
		rm := metrics.ResourceMetrics().At(i)
		fmt.Printf("ResourceMetric #%d:\n", i+1)
		fmt.Printf("  Resource Attributes: %+v\n", rm.Resource().Attributes().AsRaw())

		for j := 0; j < rm.ScopeMetrics().Len(); j++ {
			sm := rm.ScopeMetrics().At(j)
			fmt.Printf("  ScopeMetric #%d: %d metrics\n", j+1, sm.Metrics().Len())

			for k := 0; k < sm.Metrics().Len(); k++ {
				m := sm.Metrics().At(k)
				fmt.Printf("    Metric #%d:\n", k+1)
				fmt.Printf("      Name: %s\n", m.Name())
				fmt.Printf("      Description: %s\n", m.Description())
				fmt.Printf("      Type: %s\n", m.Type().String())
				// Print data points based on type
				switch m.Type().String() {
				case "Gauge":
					for p := 0; p < m.Gauge().DataPoints().Len(); p++ {
						dp := m.Gauge().DataPoints().At(p)
						fmt.Printf("        DataPoint: value=%v, attributes=%+v\n",
							dp.DoubleValue(), dp.Attributes().AsRaw())
					}
				case "Sum":
					for p := 0; p < m.Sum().DataPoints().Len(); p++ {
						dp := m.Sum().DataPoints().At(p)
						fmt.Printf("        DataPoint: value=%v, attributes=%+v\n",
							dp.DoubleValue(), dp.Attributes().AsRaw())
					}
				}
			}
		}
	}
	fmt.Println("=====================================================")

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		h.obs.RecordError(ctx, "missing_tenant_id", "grpc_receiver")
		return pmetricotlp.NewExportResponse(), fmt.Errorf("tenant-id not found in context")
	}

	// Ingest metrics
	if err := h.ingester.IngestMetrics(ctx, tenantID, metrics); err != nil {
		h.obs.RecordError(ctx, "ingest_failure", "grpc_receiver")
		return pmetricotlp.NewExportResponse(), fmt.Errorf("failed to ingest metrics: %w", err)
	}

	h.obs.RecordRequest(ctx, "/v1/metrics", time.Since(start), 200)
	return pmetricotlp.NewExportResponse(), nil
}

// logHandler implements the logs service
type logHandler struct {
	plogotlp.UnimplementedGRPCServer
	ingester *ingestion.Ingester
	obs      *observability.Observability
}

func (h *logHandler) Export(ctx context.Context, req plogotlp.ExportRequest) (plogotlp.ExportResponse, error) {
	start := time.Now()
	ctx, span := h.obs.Tracer().Start(ctx, "grpc.logs.export")
	defer span.End()

	logs := req.Logs()

	// DEBUG: Print incoming log data
	fmt.Println("\n========== INCOMING OTLP LOG (gRPC:4317) ==========")
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		fmt.Printf("ResourceLog #%d:\n", i+1)
		fmt.Printf("  Resource Attributes: %+v\n", rl.Resource().Attributes().AsRaw())

		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			fmt.Printf("  ScopeLog #%d: %d logs\n", j+1, sl.LogRecords().Len())

			for k := 0; k < sl.LogRecords().Len(); k++ {
				lr := sl.LogRecords().At(k)
				fmt.Printf("    LogRecord #%d:\n", k+1)
				fmt.Printf("      Timestamp: %s\n", lr.Timestamp().AsTime())
				fmt.Printf("      SeverityNumber: %d\n", lr.SeverityNumber())
				fmt.Printf("      SeverityText: %s\n", lr.SeverityText())
				fmt.Printf("      Body: %s\n", lr.Body().AsString())
				fmt.Printf("      TraceID: %s\n", lr.TraceID().String())
				fmt.Printf("      SpanID: %s\n", lr.SpanID().String())
				fmt.Printf("      Attributes: %+v\n", lr.Attributes().AsRaw())
			}
		}
	}
	fmt.Println("=====================================================")

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		h.obs.RecordError(ctx, "missing_tenant_id", "grpc_receiver")
		return plogotlp.NewExportResponse(), fmt.Errorf("tenant-id not found in context")
	}

	// Ingest logs
	if err := h.ingester.IngestLogs(ctx, tenantID, logs); err != nil {
		h.obs.RecordError(ctx, "ingest_failure", "grpc_receiver")
		return plogotlp.NewExportResponse(), fmt.Errorf("failed to ingest logs: %w", err)
	}

	h.obs.RecordRequest(ctx, "/v1/logs", time.Since(start), 200)
	return plogotlp.NewExportResponse(), nil
}
