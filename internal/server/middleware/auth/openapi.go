// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"net/http"
)

// OpenAPIAuth wraps an authentication middleware to respect OpenAPI security definitions.
//
// When using oapi-codegen generated servers, the generated code sets a scopes context key
// for endpoints that require authentication. Public endpoints (with `security: []` in the
// OpenAPI spec) do not have this key set.
//
// This wrapper checks for the presence of the scopes context key:
//   - If nil: endpoint is public, skip authentication
//   - If present (even empty []string{}): endpoint requires authentication
//
// Parameters:
//   - authMiddleware: the underlying authentication middleware (e.g., jwt.Middleware)
//   - scopesContextKey: the context key used by oapi-codegen (e.g., gen.BearerAuthScopes)
//
// Example usage:
//
//	jwtMW := jwt.Middleware(jwtConfig)
//	authMW := auth.OpenAPIAuth(jwtMW, gen.BearerAuthScopes)
//
//	handler := gen.HandlerWithOptions(server, gen.StdHTTPServerOptions{
//	    Middlewares: []gen.MiddlewareFunc{authMW},
//	})
func OpenAPIAuth(
	authMiddleware func(http.Handler) http.Handler,
	scopesContextKey any,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Pre-wrap with auth middleware for protected endpoints
		protected := authMiddleware(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this is a public endpoint (no security requirement in OpenAPI spec)
			if r.Context().Value(scopesContextKey) == nil {
				// Public endpoint - skip authentication
				next.ServeHTTP(w, r)
				return
			}

			// Protected endpoint - apply authentication middleware
			protected.ServeHTTP(w, r)
		})
	}
}
