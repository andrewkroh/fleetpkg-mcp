// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package app

import (
	"net/http"
	"time"

	"github.com/andrewkroh/fleetpkg-mcp/internal/otelsetup"
	"github.com/andrewkroh/fleetpkg-mcp/internal/slogutil"
)

// userContextMiddleware extracts user information from HTTP headers
// and adds it to the request context for logging.
func userContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract user headers and add to context.
		if login := r.Header.Get("X-Auth-User-Login"); login != "" {
			ctx = slogutil.WithUserLogin(ctx, login)
		}
		if email := r.Header.Get("X-Auth-User-Email"); email != "" {
			ctx = slogutil.WithUserEmail(ctx, email)
		}
		if user := r.Header.Get("X-Forwarded-User"); user != "" {
			ctx = slogutil.WithForwardedUser(ctx, user)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and delegates to the wrapped ResponseWriter.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// metricsMiddleware records HTTP request metrics.
func metricsMiddleware(metrics *otelsetup.Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code.
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		// Record metrics after request completes.
		metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, rw.statusCode, time.Since(start))
	})
}
