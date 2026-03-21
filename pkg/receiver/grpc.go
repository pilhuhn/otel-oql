package receiver

import (
	"context"
	"fmt"
	"net"

	"github.com/pilhuhn/otel-oql/pkg/ingestion"
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
}

// NewGRPCReceiver creates a new OTLP gRPC receiver
func NewGRPCReceiver(port int, validator *tenant.Validator, ingester *ingestion.Ingester) *GRPCReceiver {
	return &GRPCReceiver{
		port:      port,
		validator: validator,
		ingester:  ingester,
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
	ptraceotlp.RegisterGRPCServer(r.server, &traceHandler{ingester: r.ingester})
	pmetricotlp.RegisterGRPCServer(r.server, &metricHandler{ingester: r.ingester})
	plogotlp.RegisterGRPCServer(r.server, &logHandler{ingester: r.ingester})

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
}

func (h *traceHandler) Export(ctx context.Context, req ptraceotlp.ExportRequest) (ptraceotlp.ExportResponse, error) {
	traces := req.Traces()

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		return ptraceotlp.NewExportResponse(), fmt.Errorf("tenant-id not found in context")
	}

	// Ingest traces
	if err := h.ingester.IngestTraces(ctx, tenantID, traces); err != nil {
		return ptraceotlp.NewExportResponse(), fmt.Errorf("failed to ingest traces: %w", err)
	}

	return ptraceotlp.NewExportResponse(), nil
}

// metricHandler implements the metrics service
type metricHandler struct {
	pmetricotlp.UnimplementedGRPCServer
	ingester *ingestion.Ingester
}

func (h *metricHandler) Export(ctx context.Context, req pmetricotlp.ExportRequest) (pmetricotlp.ExportResponse, error) {
	metrics := req.Metrics()

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		return pmetricotlp.NewExportResponse(), fmt.Errorf("tenant-id not found in context")
	}

	// Ingest metrics
	if err := h.ingester.IngestMetrics(ctx, tenantID, metrics); err != nil {
		return pmetricotlp.NewExportResponse(), fmt.Errorf("failed to ingest metrics: %w", err)
	}

	return pmetricotlp.NewExportResponse(), nil
}

// logHandler implements the logs service
type logHandler struct {
	plogotlp.UnimplementedGRPCServer
	ingester *ingestion.Ingester
}

func (h *logHandler) Export(ctx context.Context, req plogotlp.ExportRequest) (plogotlp.ExportResponse, error) {
	logs := req.Logs()

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(ctx)
	if !ok {
		return plogotlp.NewExportResponse(), fmt.Errorf("tenant-id not found in context")
	}

	// Ingest logs
	if err := h.ingester.IngestLogs(ctx, tenantID, logs); err != nil {
		return plogotlp.NewExportResponse(), fmt.Errorf("failed to ingest logs: %w", err)
	}

	return plogotlp.NewExportResponse(), nil
}
