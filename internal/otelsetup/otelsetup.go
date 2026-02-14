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

	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

const (
	serviceName = "fleetpkg-mcp"
)

// SetupMetrics initializes the OpenTelemetry metrics pipeline based on
// the OTEL_METRICS_EXPORTER environment variable.
//
// Supported values:
//   - "" or "none": No metrics (returns no-op shutdown)
//   - "console": Outputs metrics to stdout (for development)
//   - "otlp": Exports metrics via OTLP HTTP (for production)
//
// Returns a shutdown function that should be called on application exit.
func SetupMetrics(ctx context.Context) (shutdown func(context.Context) error, err error) {
	exporter := os.Getenv("OTEL_METRICS_EXPORTER")

	// Default to no metrics if not configured.
	if exporter == "" || exporter == "none" {
		return func(context.Context) error { return nil }, nil
	}

	// Build resource with service info.
	version := getServiceVersion()
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on configuration.
	var metricExporter metric.Exporter
	switch exporter {
	case "console":
		metricExporter, err = stdoutmetric.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create console exporter: %w", err)
		}
	case "otlp":
		metricExporter, err = otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported OTEL_METRICS_EXPORTER value: %q (supported: none, console, otlp)", exporter)
	}

	// Create periodic reader for the exporter.
	reader := metric.NewPeriodicReader(metricExporter,
		metric.WithInterval(30*time.Second),
	)

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
