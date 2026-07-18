// Package tracing wires up the one trace path this service exposes: an
// HTTP-server span for every request, with a child span around the
// Postgres call it makes. See DECISIONS.md "Tracing" for the collector
// wiring this is meant to feed in a real deployment.
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Init sets the global TracerProvider and returns a shutdown func to flush
// on exit. With otlpEndpoint empty, it exports to stdout as JSON - a real
// trace path exists and is inspectable with zero extra infrastructure,
// which is what lets this be demonstrated without standing up a collector.
// Point otlpEndpoint at one (e.g. "otel-collector:4318") to export for
// real instead.
func Init(ctx context.Context, serviceName, otlpEndpoint string) (func(context.Context) error, error) {
	exporter, err := newExporter(ctx, otlpEndpoint)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

func newExporter(ctx context.Context, otlpEndpoint string) (sdktrace.SpanExporter, error) {
	if otlpEndpoint == "" {
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	return otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithInsecure(),
	)
}

// Tracer is the one tracer the service needs - handlers and the DB layer
// both start spans from it, rather than each package minting its own.
func Tracer() trace.Tracer {
	return otel.Tracer("bank-service")
}
