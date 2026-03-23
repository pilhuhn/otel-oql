package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config holds observability configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	OTLPEndpoint   string // gRPC endpoint (e.g., "localhost:4317")
	TenantID       string // Tenant ID to use for self-observability
	Enabled        bool   // Enable/disable observability
}

// Observability holds the OpenTelemetry providers
type Observability struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	tracer         trace.Tracer
	meter          metric.Meter
	config         Config

	// Metrics
	requestCounter      metric.Int64Counter
	requestDuration     metric.Float64Histogram
	ingestionCounter    metric.Int64Counter
	ingestionSize       metric.Int64Histogram
	queryCounter        metric.Int64Counter
	queryDuration       metric.Float64Histogram
	errorCounter        metric.Int64Counter
	kafkaPublishCounter metric.Int64Counter
}

// New initializes OpenTelemetry with OTLP exporters
func New(ctx context.Context, cfg Config) (*Observability, error) {
	if !cfg.Enabled {
		fmt.Println("Observability disabled")
		return &Observability{config: cfg}, nil
	}

	fmt.Printf("Initializing observability: service=%s endpoint=%s tenant-id=%s\n",
		cfg.ServiceName, cfg.OTLPEndpoint, cfg.TenantID)

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create gRPC connection with tenant-id in metadata
	conn, err := grpc.DialContext(ctx, cfg.OTLPEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(tenantInterceptor(cfg.TenantID)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	// Setup trace exporter
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Setup tracer provider
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Setup metric exporter
	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	// Setup meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(10*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	// Create tracer and meter
	tracer := tracerProvider.Tracer(cfg.ServiceName)
	meter := meterProvider.Meter(cfg.ServiceName)

	// Create metrics
	requestCounter, _ := meter.Int64Counter(
		"otel_oql.requests.total",
		metric.WithDescription("Total number of requests"),
	)
	requestDuration, _ := meter.Float64Histogram(
		"otel_oql.request.duration",
		metric.WithDescription("Request duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	ingestionCounter, _ := meter.Int64Counter(
		"otel_oql.ingestion.total",
		metric.WithDescription("Total number of signals ingested"),
	)
	ingestionSize, _ := meter.Int64Histogram(
		"otel_oql.ingestion.size",
		metric.WithDescription("Size of ingestion batch"),
	)
	queryCounter, _ := meter.Int64Counter(
		"otel_oql.queries.total",
		metric.WithDescription("Total number of queries"),
	)
	queryDuration, _ := meter.Float64Histogram(
		"otel_oql.query.duration",
		metric.WithDescription("Query duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	errorCounter, _ := meter.Int64Counter(
		"otel_oql.errors.total",
		metric.WithDescription("Total number of errors"),
	)
	kafkaPublishCounter, _ := meter.Int64Counter(
		"otel_oql.kafka.published.total",
		metric.WithDescription("Total number of messages published to Kafka"),
	)

	fmt.Println("✅ Observability initialized successfully")

	return &Observability{
		tracerProvider:      tracerProvider,
		meterProvider:       meterProvider,
		tracer:              tracer,
		meter:               meter,
		config:              cfg,
		requestCounter:      requestCounter,
		requestDuration:     requestDuration,
		ingestionCounter:    ingestionCounter,
		ingestionSize:       ingestionSize,
		queryCounter:        queryCounter,
		queryDuration:       queryDuration,
		errorCounter:        errorCounter,
		kafkaPublishCounter: kafkaPublishCounter,
	}, nil
}

// tenantInterceptor adds tenant-id to outgoing gRPC metadata
func tenantInterceptor(tenantID string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Add tenant-id to metadata
		md := metadata.New(map[string]string{
			"tenant-id": tenantID,
		})
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// Shutdown gracefully shuts down the observability providers
func (o *Observability) Shutdown(ctx context.Context) error {
	if !o.config.Enabled {
		return nil
	}

	fmt.Println("Shutting down observability...")
	var err error
	if o.tracerProvider != nil {
		if e := o.tracerProvider.Shutdown(ctx); e != nil {
			err = e
		}
	}
	if o.meterProvider != nil {
		if e := o.meterProvider.Shutdown(ctx); e != nil {
			err = e
		}
	}
	return err
}

// Tracer returns the tracer instance
func (o *Observability) Tracer() trace.Tracer {
	if o.tracer == nil {
		return otel.Tracer("noop")
	}
	return o.tracer
}

// RecordRequest records an HTTP request metric
func (o *Observability) RecordRequest(ctx context.Context, endpoint string, duration time.Duration, statusCode int) {
	if !o.config.Enabled {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("endpoint", endpoint),
		attribute.Int("status_code", statusCode),
	}

	o.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	o.requestDuration.Record(ctx, float64(duration.Milliseconds()), metric.WithAttributes(attrs...))
}

// RecordIngestion records an ingestion event
func (o *Observability) RecordIngestion(ctx context.Context, signalType string, count int64) {
	if !o.config.Enabled {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("signal_type", signalType),
	}

	o.ingestionCounter.Add(ctx, count, metric.WithAttributes(attrs...))
	o.ingestionSize.Record(ctx, count, metric.WithAttributes(attrs...))
}

// RecordQuery records a query execution
func (o *Observability) RecordQuery(ctx context.Context, queryType string, duration time.Duration, success bool) {
	if !o.config.Enabled {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("query_type", queryType),
		attribute.Bool("success", success),
	}

	o.queryCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	o.queryDuration.Record(ctx, float64(duration.Milliseconds()), metric.WithAttributes(attrs...))
}

// RecordError records an error occurrence
func (o *Observability) RecordError(ctx context.Context, errorType string, component string) {
	if !o.config.Enabled {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("error_type", errorType),
		attribute.String("component", component),
	}

	o.errorCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordKafkaPublish records a Kafka publish event
func (o *Observability) RecordKafkaPublish(ctx context.Context, topic string, count int64) {
	if !o.config.Enabled {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("topic", topic),
	}

	o.kafkaPublishCounter.Add(ctx, count, metric.WithAttributes(attrs...))
}
