// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/server/pkg/logging"
	"github.com/openchoreo/openchoreo/server/pkg/trace"
)

func WithLogging(next http.Handler, logger *logging.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract request data
		reqMethod := r.Method
		reqURL := r.RequestURI
		reqHost := r.Host
		reqUserAgent := r.UserAgent()
		reqSize := r.ContentLength
		reqClientIP := r.RemoteAddr
		reqContentType := r.Header.Get("Content-Type")

		// Update response writer to capture response data
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		respContentType := w.Header().Get("Content-Type")
		duration := time.Since(start)

		go func() {
			statusCode := recorder.Status()

			message := "Logging middleware"

			attrs := []any{
				slog.String("request-id", trace.CorrelationIDFromContext(r.Context())),
				slog.String("method", reqMethod),
				slog.String("url", reqURL),
				slog.String("host", reqHost),
				slog.String("user_agent", reqUserAgent),
				slog.Int64("request_size", reqSize),
				slog.String("client_ip", reqClientIP),
				slog.String("content_type", reqContentType),
				slog.Int("status", statusCode),
				slog.String("response_content_type", respContentType),
				slog.String("duration", duration.String()),
			}

			if statusCode >= 500 {
				logger.Warn(message, attrs...)
			} else {
				logger.Info(message, attrs...)
			}
		}()
	})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	// If WriteHeader has not been called, make it 200
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) Status() int {
	return r.statusCode
}
