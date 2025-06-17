// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/openchoreo/openchoreo/server/pkg/trace"
)

func WithTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get correlation ID from header if it exists
		correlationID := r.Header.Get(trace.HeaderCorrelationID)

		// If no correlation ID exists, generate a new one
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Add correlation ID to response headers
		w.Header().Set(trace.HeaderCorrelationID, correlationID)

		// Add correlation ID to context
		r = r.WithContext(trace.WithCorrelationID(r.Context(), correlationID))
		next.ServeHTTP(w, r)
	})
}
