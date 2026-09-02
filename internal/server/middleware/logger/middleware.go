// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logger

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// maxLoggedRequestIDLen bounds how much of a rejected X-Request-ID header
// value is written to the WARN log below. The header itself carries no
// length limit of its own (up to http.Server's MaxHeaderBytes, 1MB by
// default), so logging it verbatim would let a client put an arbitrarily
// large, attacker-controlled string into a log line on every request that
// sends a malformed ID.
const maxLoggedRequestIDLen = 64

// truncateForLog bounds s to at most maxLen bytes for safe inclusion in a log
// line, marking that truncation happened rather than silently cutting it.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

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

			// Get or generate request ID (UUID v7 for time-ordered tracing).
			// A client-supplied value must parse as a UUID: this is the first
			// point a request passes through, so it's the single place that
			// normalizes X-Request-ID for every downstream consumer — the
			// access log below and, on REST/MCP, the audit envelope built from
			// the same header (see audit.RequestIDFromHeader, which repeats
			// this validation as defense-in-depth and as the sole normalizer
			// for the exec/wirelogs routes this middleware doesn't wrap).
			// Normalizing here means both logs agree on one request ID instead
			// of an invalid client value passing through to the access log
			// while audit silently replaces it with a different generated one.
			requestID := r.Header.Get("X-Request-ID")
			if requestID != "" {
				if _, err := uuid.Parse(requestID); err != nil {
					baseLogger.Warn("rejected client-supplied X-Request-ID: not a valid UUID",
						slog.String("path", r.URL.Path), slog.String("value", truncateForLog(requestID, maxLoggedRequestIDLen)))
					requestID = ""
				}
			}
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
