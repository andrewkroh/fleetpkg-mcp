// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package slogutil

import (
	"context"
	"log/slog"
)

// Context keys for user information.
type contextKey int

const (
	userLoginKey contextKey = iota
	userEmailKey
	forwardedUserKey
)

// WithUserLogin adds the user login to the context.
func WithUserLogin(ctx context.Context, login string) context.Context {
	return context.WithValue(ctx, userLoginKey, login)
}

// UserLogin retrieves the user login from the context.
func UserLogin(ctx context.Context) string {
	if v, ok := ctx.Value(userLoginKey).(string); ok {
		return v
	}
	return ""
}

// WithUserEmail adds the user email to the context.
func WithUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, userEmailKey, email)
}

// UserEmail retrieves the user email from the context.
func UserEmail(ctx context.Context) string {
	if v, ok := ctx.Value(userEmailKey).(string); ok {
		return v
	}
	return ""
}

// WithForwardedUser adds the forwarded user to the context.
func WithForwardedUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, forwardedUserKey, user)
}

// ForwardedUser retrieves the forwarded user from the context.
func ForwardedUser(ctx context.Context) string {
	if v, ok := ctx.Value(forwardedUserKey).(string); ok {
		return v
	}
	return ""
}

// UserContextHandler wraps a slog.Handler to automatically add user
// information from the context to log records.
type UserContextHandler struct {
	inner slog.Handler
}

// NewUserContextHandler creates a new UserContextHandler wrapping the given handler.
func NewUserContextHandler(inner slog.Handler) *UserContextHandler {
	return &UserContextHandler{inner: inner}
}

// Enabled implements slog.Handler.
func (h *UserContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler. It extracts user information from the context
// and adds it as attributes to the log record.
func (h *UserContextHandler) Handle(ctx context.Context, record slog.Record) error {
	// Extract user information from context and add as attributes.
	var attrs []slog.Attr

	if login := UserLogin(ctx); login != "" {
		attrs = append(attrs, slog.String("user_login", login))
	}
	if email := UserEmail(ctx); email != "" {
		attrs = append(attrs, slog.String("user_email", email))
	}
	if user := ForwardedUser(ctx); user != "" {
		attrs = append(attrs, slog.String("forwarded_user", user))
	}

	if len(attrs) > 0 {
		record.AddAttrs(attrs...)
	}

	return h.inner.Handle(ctx, record)
}

// WithAttrs implements slog.Handler.
func (h *UserContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &UserContextHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler.
func (h *UserContextHandler) WithGroup(name string) slog.Handler {
	return &UserContextHandler{inner: h.inner.WithGroup(name)}
}
