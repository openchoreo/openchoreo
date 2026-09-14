// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// newPublicServer builds the public API the way cmd/observer does: the generated
// router from observer-api.yaml, the composed middleware chain, and both strict
// error handlers.
//
// It calls the real ObserverMiddlewares rather than assembling a chain of its
// own, so the ordering has exactly one definition. Listing the middlewares here
// instead would mean the handler tests run an ordering main.go does not own.
//
// The only substitution is authentication: a pass-through stands in for
// auth.OpenAPIAuth so these tests exercise handler behavior instead of
// re-testing the JWT middleware. The chain's shape, including where auth sits in
// it, is still the production one. Auth itself is covered by
// TestObserverMiddlewaresLeaveHealthPublic, which drives the real
// auth.OpenAPIAuth.
//
// Driving the router rather than calling handler methods means routing, path and
// query parameter binding, and body decoding are all exercised alongside the
// handler — so a spec change that breaks a route or a parameter fails here.
func newPublicServer(t *testing.T, h *Handler) http.Handler {
	t.Helper()
	return newPublicServerWithAuth(t, h, passThroughAuth)
}

// newPublicServerWithAuth is newPublicServer with a caller-supplied auth
// middleware, for the tests that need to observe what auth does rather than
// bypass it.
func newPublicServerWithAuth(
	t *testing.T,
	h *Handler,
	authMiddleware func(http.Handler) http.Handler,
) http.Handler {
	t.Helper()
	return newPublicServerWithAudit(t, h, authMiddleware, noopAuditEmitter(t))
}

// newPublicServerWithAudit is newPublicServerWithAuth with a caller-supplied
// audit emitter, for the tests that assert on emitted events.
func newPublicServerWithAudit(
	t *testing.T,
	h *Handler,
	authMiddleware func(http.Handler) http.Handler,
	auditEmitter *audit.Emitter,
) http.Handler {
	t.Helper()

	logger := noopLogger()

	mws, err := ObserverMiddlewares(ObserverMiddlewareOptions{
		Logger:         logger,
		AuthMiddleware: authMiddleware,
		AuditEmitter:   auditEmitter,
		AuditEnabled:   true,
	})
	require.NoError(t, err)

	strict := gen.NewStrictHandlerWithOptions(
		h,
		nil,
		gen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  StrictRequestErrorHandler(logger),
			ResponseErrorHandlerFunc: StrictResponseErrorHandler(logger),
		},
	)

	return gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		Middlewares:      mws,
		ErrorHandlerFunc: ParamBindingErrorHandler(logger),
	})
}

// passThroughAuth stands in for auth.OpenAPIAuth in handler tests: it keeps the
// chain's shape without requiring a token.
func passThroughAuth(next http.Handler) http.Handler { return next }

// serve routes one request through the generated public router.
//
// Path parameters come from the request URL, so tests do not call
// req.SetPathValue — the mux sets them during routing, overwriting anything a
// test had put there.
func serve(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rr := httptest.NewRecorder()
	newPublicServer(t, h).ServeHTTP(rr, req)
	return rr
}
