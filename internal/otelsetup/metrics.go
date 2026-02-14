// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package otelsetup

import (
	"context"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the OpenTelemetry metric instruments.
type Metrics struct {
	httpRequestsTotal   metric.Int64Counter
	httpRequestDuration metric.Float64Histogram
	toolCallsTotal      metric.Int64Counter
	toolCallDuration    metric.Float64Histogram
	sqlQueriesTotal     metric.Int64Counter
	sqlQueryDuration    metric.Float64Histogram
	dbInitDuration      metric.Float64Gauge
	errorsTotal         metric.Int64Counter
}

// NewMetrics creates and registers all metric instruments.
// Returns nil if metrics are disabled.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter("fleetpkg-mcp")

	httpRequestsTotal, err := meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	httpRequestDuration, err := meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	toolCallsTotal, err := meter.Int64Counter(
		"mcp_tool_calls_total",
		metric.WithDescription("Total number of MCP tool calls"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, err
	}

	toolCallDuration, err := meter.Float64Histogram(
		"mcp_tool_call_duration_seconds",
		metric.WithDescription("MCP tool call duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	sqlQueriesTotal, err := meter.Int64Counter(
		"sql_queries_total",
		metric.WithDescription("Total number of SQL queries executed"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		return nil, err
	}

	sqlQueryDuration, err := meter.Float64Histogram(
		"sql_query_duration_seconds",
		metric.WithDescription("SQL query duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	dbInitDuration, err := meter.Float64Gauge(
		"db_init_duration_seconds",
		metric.WithDescription("Duration of the last database initialization in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	errorsTotal, err := meter.Int64Counter(
		"errors_total",
		metric.WithDescription("Total number of errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		httpRequestsTotal:   httpRequestsTotal,
		httpRequestDuration: httpRequestDuration,
		toolCallsTotal:      toolCallsTotal,
		toolCallDuration:    toolCallDuration,
		sqlQueriesTotal:     sqlQueriesTotal,
		sqlQueryDuration:    sqlQueryDuration,
		dbInitDuration:      dbInitDuration,
		errorsTotal:         errorsTotal,
	}, nil
}

// RecordHTTPRequest records metrics for an HTTP request.
func (m *Metrics) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	if m == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.String("status_code", strconv.Itoa(statusCode)),
	}

	m.httpRequestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.httpRequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordToolCall records metrics for an MCP tool call.
func (m *Metrics) RecordToolCall(ctx context.Context, toolName string, success bool, duration time.Duration) {
	if m == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("tool_name", toolName),
		attribute.Bool("success", success),
	}

	m.toolCallsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.toolCallDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordSQLQuery records metrics for a SQL query.
func (m *Metrics) RecordSQLQuery(ctx context.Context, success bool, duration time.Duration) {
	if m == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.Bool("success", success),
	}

	m.sqlQueriesTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.sqlQueryDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordDBInit records metrics for database initialization.
func (m *Metrics) RecordDBInit(ctx context.Context, duration time.Duration) {
	if m == nil {
		return
	}

	m.dbInitDuration.Record(ctx, duration.Seconds())
}

// RecordError records an error metric.
func (m *Metrics) RecordError(ctx context.Context, errorType string) {
	if m == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("type", errorType),
	}

	m.errorsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
}
