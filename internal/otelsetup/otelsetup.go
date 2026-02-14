// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package otelsetup

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

const (
	defaultServiceName = "fleetpkg-mcp"
)

// SetupMetrics initializes the OpenTelemetry metrics pipeline.
//
// Metrics are only enabled when OTEL_METRICS_EXPORTER is explicitly set
// to a value other than "none". The autoexport package handles exporter
// selection based on standard OTel environment variables (e.g.,
// OTEL_EXPORTER_OTLP_PROTOCOL for gRPC vs HTTP).
//
// Returns a shutdown function that should be called on application exit.
func SetupMetrics(ctx context.Context) (shutdown func(context.Context) error, err error) {
	exporterEnv := os.Getenv("OTEL_METRICS_EXPORTER")

	// Quiet Opt-In: Only initialize if the user explicitly set an exporter.
	// This prevents OTel from defaulting to 'otlp' and logging connection errors.
	if exporterEnv == "" || exporterEnv == "none" {
		return func(context.Context) error { return nil }, nil
	}

	// Use autoexport to create a metric reader based on standard OTel env vars
	// (e.g., OTEL_EXPORTER_OTLP_PROTOCOL for gRPC vs HTTP).
	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric reader: %w", err)
	}

	// Build resource with service info. Standard OTel env vars
	// (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES) are picked up
	// automatically by resource.Default().
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(defaultServiceName),
			semconv.ServiceVersion(getServiceVersion()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create and register the meter provider.
	provider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(reader),
	)
	otel.SetMeterProvider(provider)

	// Start Go runtime metrics collection.
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(15 * time.Second)); err != nil {
		provider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to start runtime metrics: %w", err)
	}

	// Start host/process metrics collection.
	if err := host.Start(); err != nil {
		provider.Shutdown(ctx)
		return nil, fmt.Errorf("failed to start host metrics: %w", err)
	}

	return provider.Shutdown, nil
}

// getServiceVersion returns the module version from build info.
func getServiceVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}
