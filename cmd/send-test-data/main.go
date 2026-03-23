package main

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	ctx := context.Background()

	// Create OTLP HTTP exporter
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("localhost:4318"),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithHeaders(map[string]string{
			"tenant-id": "0",
		}),
	)
	if err != nil {
		fmt.Printf("Failed to create exporter: %v\n", err)
		return
	}

	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("payment-service"),
		),
	)
	if err != nil {
		fmt.Printf("Failed to create resource: %v\n", err)
		return
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			fmt.Printf("Error shutting down tracer provider: %v\n", err)
		}
	}()

	// Create tracer
	tracer := tp.Tracer("test-tracer")

	// Create test span
	fmt.Println("📤 Sending test span...")
	_, span := tracer.Start(ctx, "checkout",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(time.Now()),
	)
	span.SetAttributes(
		semconv.HTTPMethod("POST"),
		semconv.HTTPStatusCode(200),
		semconv.HTTPRoute("/api/checkout"),
	)
	span.End(trace.WithTimestamp(time.Now().Add(150 * time.Millisecond)))

	// Force flush
	if err := tp.ForceFlush(ctx); err != nil {
		fmt.Printf("Failed to flush: %v\n", err)
		return
	}

	fmt.Println("✅ Test span sent successfully!")
	fmt.Println("")
	fmt.Println("Wait a few seconds for Pinot to process, then query:")
	fmt.Println("  curl -X POST http://localhost:8080/query \\")
	fmt.Println("    -H 'X-Tenant-ID: 0' \\")
	fmt.Println("    -H 'Content-Type: application/json' \\")
	fmt.Println("    -d '{\"query\": \"signal=spans | where tenant_id == 0 | limit 10\"}'")
}
