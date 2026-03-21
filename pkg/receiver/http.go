package receiver

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/pilhuhn/otel-oql/pkg/ingestion"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

// HTTPReceiver implements OTLP HTTP receiver
type HTTPReceiver struct {
	port      int
	validator *tenant.Validator
	ingester  *ingestion.Ingester
	server    *http.Server
}

// NewHTTPReceiver creates a new OTLP HTTP receiver
func NewHTTPReceiver(port int, validator *tenant.Validator, ingester *ingestion.Ingester) *HTTPReceiver {
	return &HTTPReceiver{
		port:      port,
		validator: validator,
		ingester:  ingester,
	}
}

// Start starts the HTTP server
func (r *HTTPReceiver) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register OTLP endpoints with tenant validation middleware
	mux.Handle("/v1/traces", r.validator.HTTPMiddleware(http.HandlerFunc(r.handleTraces)))
	mux.Handle("/v1/metrics", r.validator.HTTPMiddleware(http.HandlerFunc(r.handleMetrics)))
	mux.Handle("/v1/logs", r.validator.HTTPMiddleware(http.HandlerFunc(r.handleLogs)))

	r.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", r.port),
		Handler: mux,
	}

	go func() {
		if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	fmt.Printf("OTLP HTTP receiver listening on port %d\n", r.port)
	return nil
}

// Stop stops the HTTP server
func (r *HTTPReceiver) Stop(ctx context.Context) error {
	if r.server != nil {
		return r.server.Shutdown(ctx)
	}
	return nil
}

// handleTraces handles trace export requests
func (r *HTTPReceiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Unmarshal OTLP request
	otlpReq := ptraceotlp.NewExportRequest()
	if err := otlpReq.UnmarshalProto(body); err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal request: %v", err), http.StatusBadRequest)
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(req.Context())
	if !ok {
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Ingest traces
	if err := r.ingester.IngestTraces(req.Context(), tenantID, otlpReq.Traces()); err != nil {
		http.Error(w, fmt.Sprintf("failed to ingest traces: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
}

// handleMetrics handles metric export requests
func (r *HTTPReceiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Unmarshal OTLP request
	otlpReq := pmetricotlp.NewExportRequest()
	if err := otlpReq.UnmarshalProto(body); err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal request: %v", err), http.StatusBadRequest)
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(req.Context())
	if !ok {
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Ingest metrics
	if err := r.ingester.IngestMetrics(req.Context(), tenantID, otlpReq.Metrics()); err != nil {
		http.Error(w, fmt.Sprintf("failed to ingest metrics: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
}

// handleLogs handles log export requests
func (r *HTTPReceiver) handleLogs(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Unmarshal OTLP request
	otlpReq := plogotlp.NewExportRequest()
	if err := otlpReq.UnmarshalProto(body); err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal request: %v", err), http.StatusBadRequest)
		return
	}

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(req.Context())
	if !ok {
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}

	// Ingest logs
	if err := r.ingester.IngestLogs(req.Context(), tenantID, otlpReq.Logs()); err != nil {
		http.Error(w, fmt.Sprintf("failed to ingest logs: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
}
