// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net/http"
)

// SubjectContext contains the authenticated subject's type and entitlements
type SubjectContext struct {
	ID                string   // Unique identifier for the subject
	Type              string   // Type of subject (user, service_account, etc.)
	EntitlementClaim  string   // The claim name used for entitlements (e.g., "groups", "scopes")
	EntitlementValues []string // The entitlement values extracted from the claim
}

// Middleware defines the interface that all authentication middlewares must implement
type Middleware interface {
	// Handler returns an HTTP middleware handler that:
	// 1. Authenticates the request (validates credentials)
	// 2. Resolves the SubjectContext (user type and entitlements)
	// 3. Stores the SubjectContext in the request context
	Handler(next http.Handler) http.Handler
}

// contextKey is a type for context keys to avoid collisions
type contextKey string

const (
	// subjectContextKey is the context key for storing SubjectContext
	subjectContextKey contextKey = "subject_context"
)

// GetSubjectContext retrieves the SubjectContext from the request context
func GetSubjectContext(r *http.Request) (*SubjectContext, bool) {
	ctx, ok := r.Context().Value(subjectContextKey).(*SubjectContext)
	return ctx, ok
}

// GetSubjectContextFromContext retrieves the SubjectContext from a context.Context
func GetSubjectContextFromContext(ctx context.Context) (*SubjectContext, bool) {
	subjectCtx, ok := ctx.Value(subjectContextKey).(*SubjectContext)
	return subjectCtx, ok
}

// SetSubjectContext stores the SubjectContext in the request context
func SetSubjectContext(ctx context.Context, subjectCtx *SubjectContext) context.Context {
	return context.WithValue(ctx, subjectContextKey, subjectCtx)
}
