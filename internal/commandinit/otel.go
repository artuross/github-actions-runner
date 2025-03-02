package commandinit

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

type ShutdownFunc func(ctx context.Context) error

func noopShutdown(_ context.Context) error {
	return nil
}

func NewOpenTelemetry(ctx context.Context, serviceName string) (trace.TracerProvider, ShutdownFunc, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithCompressor("gzip"))
	if err != nil {
		return nil, noopShutdown, fmt.Errorf("create OTEL exporter: %w", err)
	}

	resource, err := sdkresource.New(
		ctx,
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, noopShutdown, fmt.Errorf("create OTEL resource: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBlocking(),
			sdktrace.WithMaxQueueSize(10000),
			sdktrace.WithMaxExportBatchSize(1000),
		),
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	return tracerProvider, tracerProvider.Shutdown, nil
}
