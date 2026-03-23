package receiver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pilhuhn/otel-oql/pkg/ingestion"
	"github.com/pilhuhn/otel-oql/pkg/observability"
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
	obs       *observability.Observability
}

// NewHTTPReceiver creates a new OTLP HTTP receiver
func NewHTTPReceiver(port int, validator *tenant.Validator, ingester *ingestion.Ingester, obs *observability.Observability) *HTTPReceiver {
	return &HTTPReceiver{
		port:      port,
		validator: validator,
		ingester:  ingester,
		obs:       obs,
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
	start := time.Now()
	ctx, span := r.obs.Tracer().Start(req.Context(), "http.traces.export")
	defer span.End()

	fmt.Printf("DEBUG HTTP: Received traces request from %s\n", req.RemoteAddr)

	if req.Method != http.MethodPost {
		r.obs.RecordRequest(ctx, "/v1/traces", time.Since(start), http.StatusMethodNotAllowed)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		fmt.Printf("DEBUG HTTP: Failed to read body: %v\n", err)
		r.obs.RecordError(ctx, "read_body_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/traces", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()
	fmt.Printf("DEBUG HTTP: Read %d bytes from request body\n", len(body))

	// Unmarshal OTLP request
	otlpReq := ptraceotlp.NewExportRequest()
	if err := otlpReq.UnmarshalProto(body); err != nil {
		fmt.Printf("DEBUG HTTP: Failed to unmarshal: %v\n", err)
		r.obs.RecordError(ctx, "unmarshal_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/traces", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("failed to unmarshal request: %v", err), http.StatusBadRequest)
		return
	}
	fmt.Printf("DEBUG HTTP: Successfully unmarshaled traces\n")

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(req.Context())
	if !ok {
		fmt.Printf("DEBUG HTTP: Tenant ID not found in context\n")
		r.obs.RecordError(ctx, "missing_tenant_id", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/traces", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}
	fmt.Printf("DEBUG HTTP: Tenant ID: %d\n", tenantID)

	// Ingest traces
	if err := r.ingester.IngestTraces(ctx, tenantID, otlpReq.Traces()); err != nil {
		fmt.Printf("DEBUG HTTP: Failed to ingest traces: %v\n", err)
		r.obs.RecordError(ctx, "ingest_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/traces", time.Since(start), http.StatusInternalServerError)
		http.Error(w, fmt.Sprintf("failed to ingest traces: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Printf("DEBUG HTTP: Successfully ingested traces\n")

	// Return success
	r.obs.RecordRequest(ctx, "/v1/traces", time.Since(start), http.StatusOK)
	w.WriteHeader(http.StatusOK)
}

// handleMetrics handles metric export requests
func (r *HTTPReceiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	ctx, span := r.obs.Tracer().Start(req.Context(), "http.metrics.export")
	defer span.End()

	fmt.Printf("DEBUG HTTP: Received metrics request from %s\n", req.RemoteAddr)

	if req.Method != http.MethodPost {
		r.obs.RecordRequest(ctx, "/v1/metrics", time.Since(start), http.StatusMethodNotAllowed)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		fmt.Printf("DEBUG HTTP: Failed to read body: %v\n", err)
		r.obs.RecordError(ctx, "read_body_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/metrics", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()
	fmt.Printf("DEBUG HTTP: Read %d bytes from request body\n", len(body))

	// Unmarshal OTLP request
	otlpReq := pmetricotlp.NewExportRequest()
	if err := otlpReq.UnmarshalProto(body); err != nil {
		fmt.Printf("DEBUG HTTP: Failed to unmarshal: %v\n", err)
		r.obs.RecordError(ctx, "unmarshal_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/metrics", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("failed to unmarshal request: %v", err), http.StatusBadRequest)
		return
	}
	fmt.Printf("DEBUG HTTP: Successfully unmarshaled metrics\n")

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(req.Context())
	if !ok {
		fmt.Printf("DEBUG HTTP: Tenant ID not found in context\n")
		r.obs.RecordError(ctx, "missing_tenant_id", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/metrics", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}
	fmt.Printf("DEBUG HTTP: Tenant ID: %d\n", tenantID)

	// Ingest metrics
	if err := r.ingester.IngestMetrics(ctx, tenantID, otlpReq.Metrics()); err != nil {
		fmt.Printf("DEBUG HTTP: Failed to ingest metrics: %v\n", err)
		r.obs.RecordError(ctx, "ingest_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/metrics", time.Since(start), http.StatusInternalServerError)
		http.Error(w, fmt.Sprintf("failed to ingest metrics: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Printf("DEBUG HTTP: Successfully ingested metrics\n")

	// Return success
	r.obs.RecordRequest(ctx, "/v1/metrics", time.Since(start), http.StatusOK)
	w.WriteHeader(http.StatusOK)
}

// handleLogs handles log export requests
func (r *HTTPReceiver) handleLogs(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	ctx, span := r.obs.Tracer().Start(req.Context(), "http.logs.export")
	defer span.End()

	fmt.Printf("DEBUG HTTP: Received logs request from %s\n", req.RemoteAddr)

	if req.Method != http.MethodPost {
		r.obs.RecordRequest(ctx, "/v1/logs", time.Since(start), http.StatusMethodNotAllowed)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		fmt.Printf("DEBUG HTTP: Failed to read body: %v\n", err)
		r.obs.RecordError(ctx, "read_body_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/logs", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()
	fmt.Printf("DEBUG HTTP: Read %d bytes from request body\n", len(body))

	// Unmarshal OTLP request
	otlpReq := plogotlp.NewExportRequest()
	if err := otlpReq.UnmarshalProto(body); err != nil {
		fmt.Printf("DEBUG HTTP: Failed to unmarshal: %v\n", err)
		r.obs.RecordError(ctx, "unmarshal_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/logs", time.Since(start), http.StatusBadRequest)
		http.Error(w, fmt.Sprintf("failed to unmarshal request: %v", err), http.StatusBadRequest)
		return
	}
	fmt.Printf("DEBUG HTTP: Successfully unmarshaled logs\n")

	// Get tenant ID from context
	tenantID, ok := tenant.FromContext(req.Context())
	if !ok {
		fmt.Printf("DEBUG HTTP: Tenant ID not found in context\n")
		r.obs.RecordError(ctx, "missing_tenant_id", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/logs", time.Since(start), http.StatusUnauthorized)
		http.Error(w, "tenant-id not found", http.StatusUnauthorized)
		return
	}
	fmt.Printf("DEBUG HTTP: Tenant ID: %d\n", tenantID)

	// Ingest logs
	if err := r.ingester.IngestLogs(ctx, tenantID, otlpReq.Logs()); err != nil {
		fmt.Printf("DEBUG HTTP: Failed to ingest logs: %v\n", err)
		r.obs.RecordError(ctx, "ingest_failure", "http_receiver")
		r.obs.RecordRequest(ctx, "/v1/logs", time.Since(start), http.StatusInternalServerError)
		http.Error(w, fmt.Sprintf("failed to ingest logs: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Printf("DEBUG HTTP: Successfully ingested logs\n")

	// Return success
	r.obs.RecordRequest(ctx, "/v1/logs", time.Since(start), http.StatusOK)
	w.WriteHeader(http.StatusOK)
}
