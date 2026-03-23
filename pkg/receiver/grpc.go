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
