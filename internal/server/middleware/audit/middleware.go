// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// Middleware handles audit logging for HTTP requests
type Middleware struct {
	logger   *Logger
	resolver *ActionResolver
}

// NewMiddleware creates a new audit middleware
func NewMiddleware(logger *Logger, resolver *ActionResolver) *Middleware {
	return &Middleware{
		logger:   logger,
		resolver: resolver,
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Handler returns the HTTP middleware handler
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to resolve action from request
		actionDef := m.resolver.Resolve(r)
		if actionDef == nil {
			// No audit action defined for this route, skip audit logging
			next.ServeHTTP(w, r)
			return
		}

		// Create mutable audit data container and add to context
		auditData := &AuditData{}
		ctx := context.WithValue(r.Context(), auditDataKey, auditData)

		// Wrap response writer to capture status code
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			written:        false,
		}

		// Extract actor from authentication context
		actor := m.extractActor(r)

		// Get request ID from logger context
		requestID := getRequestID(r)

		// Get source IP
		sourceIP := getSourceIP(r)

		// Process request with audit-enabled context
		next.ServeHTTP(rw, r.WithContext(ctx))

		// Determine result based on status code
		result := determineResult(rw.statusCode)

		// Get resource and metadata from audit data (may be nil)
		resource := auditData.Resource
		metadata := auditData.Metadata

		// Create and emit audit event
		event := &Event{
			Actor:     actor,
			Action:    actionDef.Action,
			Category:  actionDef.Category,
			Resource:  resource,
			Result:    result,
			RequestID: requestID,
			SourceIP:  sourceIP,
			Metadata:  metadata,
		}

		m.logger.LogEvent(event)
	})
}

// extractActor extracts actor information from the authentication context
func (m *Middleware) extractActor(r *http.Request) Actor {
	// Try to get subject context from authentication middleware
	subjectCtx, ok := auth.GetSubjectContextFromContext(r.Context())
	if !ok || subjectCtx == nil {
		return Actor{
			Type: "anonymous",
			ID:   "anonymous",
		}
	}

	// Determine actor type based on subject type
	actorType := subjectCtx.Type
	if actorType == "" {
		actorType = "user"
	}

	// Extract actor ID from entitlement values (first value is typically the user ID/email)
	actorID := "unknown"
	if len(subjectCtx.EntitlementValues) > 0 {
		actorID = subjectCtx.ID
	}

	return Actor{
		Type: actorType,
		Entitlements: map[string]any{
			subjectCtx.EntitlementClaim: subjectCtx.EntitlementValues,
		},
		ID: actorID,
	}
}

// getRequestID extracts or generates the request ID
func getRequestID(r *http.Request) string {
	// Try to get it from X-Request-ID header (set by logger middleware)
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		// Generate a new UUID v7 for request correlation
		if id, err := uuid.NewV7(); err == nil {
			requestID = id.String()
		} else {
			// Fallback to v4 if v7 generation fails
			requestID = uuid.New().String()
		}
	}
	return requestID
}

// getSourceIP extracts the client IP address from the request
func getSourceIP(r *http.Request) string {
	// Check X-Forwarded-For header first (proxy/load balancer)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// determineResult maps HTTP status code to audit result
func determineResult(statusCode int) Result {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return ResultSuccess
	case statusCode == 401 || statusCode == 403:
		return ResultDenied
	default:
		return ResultFailure
	}
}
