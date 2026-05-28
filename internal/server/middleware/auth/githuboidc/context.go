// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package githuboidc

import "context"

type ctxKey struct{}

// WithClaims returns a child context that carries the verified GitHub Actions
// OIDC claims. Used by the middleware to publish claims and by downstream
// services (e.g. workload registration) to consume them.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, claims)
}

// ClaimsFromContext returns the verified GitHub Actions OIDC claims if the
// request was authenticated via the GitHub OIDC middleware.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(ctxKey{}).(*Claims)
	return c, ok && c != nil
}
