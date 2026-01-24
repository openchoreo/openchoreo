// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// responseWriter wraps http.ResponseWriter to capture status code and bytes written
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Middleware returns an HTTP middleware that logs access logs and enriches context with request ID
func Middleware(baseLogger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Get or generate request ID (UUID v7 for time-ordered tracing)
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				if id, err := uuid.NewV7(); err == nil {
					requestID = id.String()
				} else {
					// Fallback to v4 if v7 generation fails
					requestID = uuid.New().String()
				}
			}

			// Set X-Request-ID header for downstream middleware
			r.Header.Set("X-Request-ID", requestID)

			// Wrap response writer to capture status and bytes
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status if WriteHeader is not called
				bytes:          0,
			}

			// Create context logger with minimal fields
			reqLogger := baseLogger.With(
				slog.String("request_id", requestID),
			)

			ctx := WithLogger(r.Context(), reqLogger)
			next.ServeHTTP(rw, r.WithContext(ctx))

			// Log access log with additional fields after request completes
			duration := time.Since(start)
			baseLogger.Info("ACCESS-LOG",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
				slog.String("request_id", requestID),
				slog.Int("status", rw.statusCode),
				slog.Int("bytes", rw.bytes),
				slog.Duration("duration", duration),
			)
		})
	}
}

// LoggerMiddleware is an alias for Middleware for backward compatibility
func LoggerMiddleware(baseLogger *slog.Logger) func(http.Handler) http.Handler {
	return Middleware(baseLogger)
}
