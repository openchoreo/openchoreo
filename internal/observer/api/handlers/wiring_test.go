// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// TestObserverMiddlewaresLeaveHealthPublic drives the real public chain,
// including auth.OpenAPIAuth, and asserts the spec decides which routes need a
// token.
//
// Before the migration /health was registered outside the JWT-protected route
// group by hand. Now it goes through the same chain as everything else, and stays
// public only because the generated wrapper sets no scopes context key for it —
// which happens only because observer-api.yaml marks it `security: []`.
// Kubernetes probes send no Authorization header, so a mistake here takes the
// liveness probe down.
//
// The stub auth middleware rejects everything it is given, so a route reaching it
// returns 401. That makes the two assertions below opposites: /health must not
// reach it, and a protected operation must.
func TestObserverMiddlewaresLeaveHealthPublic(t *testing.T) {
	t.Parallel()

	rejectAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}

	healthSvc := servicemocks.NewMockHealthChecker(t)
	healthSvc.On("Check", mock.Anything).Return(nil).Maybe()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		healthService: healthSvc,
		logsService:   servicemocks.NewMockLogsQuerier(t),
	}

	logger := noopLogger()
	mws, err := ObserverMiddlewares(ObserverMiddlewareOptions{
		Logger:         logger,
		AuthMiddleware: auth.OpenAPIAuth(rejectAll, gen.BearerAuthScopes),
	})
	require.NoError(t, err)

	strict := gen.NewStrictHandlerWithOptions(h, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  StrictRequestErrorHandler(logger),
		ResponseErrorHandlerFunc: StrictResponseErrorHandler(logger),
	})
	srv := gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		Middlewares:      mws,
		ErrorHandlerFunc: ParamBindingErrorHandler(logger),
	})

	t.Run("health is public", func(t *testing.T) {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

		assert.Equal(t, http.StatusOK, rr.Code,
			"/health must bypass auth; check `security: []` on the health operation")
	})

	t.Run("a query operation requires auth", func(t *testing.T) {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/logs/query",
			strings.NewReader(`{}`)))

		assert.Equal(t, http.StatusUnauthorized, rr.Code,
			"a protected operation must reach the auth middleware")
	})
}

// TestRequireJSONContentType covers the check the migration would otherwise have
// dropped: httputil.BindJSON refused a non-JSON Content-Type, and the generated
// strict layer decodes unconditionally.
func TestRequireJSONContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{"json is accepted", "application/json", http.StatusOK},
		{"json with charset is accepted", "application/json; charset=utf-8", http.StatusOK},
		// Absent Content-Type was allowed by BindJSON; preserved.
		{"absent is accepted", "", http.StatusOK},
		{"form encoding is rejected", "application/x-www-form-urlencoded", http.StatusBadRequest},
		{"text is rejected", "text/plain", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// A body that would otherwise validate, so a 400 can only come from
			// the Content-Type check.
			body := `{"searchScope":{"namespace":"ns"},` +
				`"startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-02T00:00:00Z"}`

			svc := servicemocks.NewMockLogsQuerier(t)
			svc.On("QueryLogs", mock.Anything, mock.Anything).
				Return(&types.LogsQueryResponse{}, nil).Maybe()

			h := &Handler{
				baseHandler: baseHandler{logger: noopLogger()},
				logsService: svc,
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", strings.NewReader(body))
			// httptest.NewRequest defaults no Content-Type; set only when the
			// case calls for one.
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			} else {
				req.Header.Del("Content-Type")
			}

			rr := serve(t, h, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
		})
	}
}

// TestNonSpecRoutesCoexistWithGeneratedRoutes covers the one thing the handler
// tests cannot: main.go registers /mcp on the base mux and then layers the
// generated routes onto the *same* mux. http.ServeMux panics on conflicting
// patterns at registration time, so a future spec path that collided with a
// non-spec route would take the process down at startup.
//
// The handler tests each use a fresh mux, so none of them would catch it. This
// test has already earned its keep: it panicked when
// /.well-known/oauth-protected-resource moved into the spec while still being
// hand-registered here.
func TestNonSpecRoutesCoexistWithGeneratedRoutes(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	// /mcp is the only route cmd/observer still registers by hand.
	mux.Handle("/mcp", http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))

	logger := noopLogger()
	mws, err := ObserverMiddlewares(ObserverMiddlewareOptions{
		Logger:         logger,
		AuthMiddleware: passThroughAuth,
	})
	require.NoError(t, err)

	healthSvc := servicemocks.NewMockHealthChecker(t)
	healthSvc.On("Check", mock.Anything).Return(nil).Maybe()
	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		healthService: healthSvc,
	}

	// Layering the generated routes onto the same mux must not panic.
	strict := gen.NewStrictHandlerWithOptions(h, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  StrictRequestErrorHandler(logger),
		ResponseErrorHandlerFunc: StrictResponseErrorHandler(logger),
	})
	srv := gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter:       mux,
		Middlewares:      mws,
		ErrorHandlerFunc: ParamBindingErrorHandler(logger),
	})

	// Both kinds of route still resolve to their own handler.
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"non-spec mcp", http.MethodPost, "/mcp", http.StatusTeapot},
		{"generated health", http.MethodGet, "/health", http.StatusOK},
		{"generated oauth metadata", http.MethodGet, "/.well-known/oauth-protected-resource", http.StatusOK},
	} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, tc.wantStatus, rr.Code, "%s (%s %s)", tc.name, tc.method, tc.path)
	}
}

// TestPublicRouterDoesNotServeInternalOperations is the mirror of
// TestInternalRouterServesOnlyItsOwnOperations, and together they are what make
// the spec split load-bearing.
//
// Before the split, one spec described both ports. Handing that single generated
// router to two muxes would have put all eighteen operations on both — exposing
// alert-rule CRUD on the public port. Each port now serves exactly its own spec's
// operations, by construction rather than by discipline.
func TestPublicRouterDoesNotServeInternalOperations(t *testing.T) {
	t.Parallel()

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}}
	srv := newPublicServer(t, h)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1alpha1/alerts/sources/log/rules"},
		{http.MethodGet, "/api/v1alpha1/alerts/sources/log/rules/r1"},
		{http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/r1"},
		{http.MethodDelete, "/api/v1alpha1/alerts/sources/log/rules/r1"},
		{http.MethodPost, "/api/v1alpha1/alerts/webhook"},
	} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))

		assert.Equal(t, http.StatusNotFound, rr.Code,
			"%s %s is an internal operation and must not be served on the public port",
			tc.method, tc.path)
	}
}

// TestObserverMiddlewaresRequireAuthMiddleware guards against wiring the public
// chain with no auth at all, which would silently make every operation public.
func TestObserverMiddlewaresRequireAuthMiddleware(t *testing.T) {
	t.Parallel()

	_, err := ObserverMiddlewares(ObserverMiddlewareOptions{Logger: noopLogger()})
	require.Error(t, err)
}

// TestInternalMiddlewareOrdering drives the real composer and pins the ordering
// that InternalMiddlewares claims: logger → recovery → handler.
//
// This is worth a test rather than a comment because the ordering is *inverted*
// relative to the rest of the codebase. middleware.Chain treats the first entry
// as outermost, so the pre-migration wiring read
// `.With(loggerMiddleware, recoveryMiddleware)`. oapi-codegen applies its
// slice in the opposite direction — it wraps in order, so the LAST entry ends up
// outermost — which is why InternalMiddlewares returns {recovery, logger}.
//
// "Correcting" that slice to the intuitive logger-first order would put recovery
// outside logger, and a panicking handler would then unwind past the logger
// without ever being logged. Both orderings return a 500 and neither panics the
// test, so only asserting that the logger *observed the request* distinguishes
// them.
func TestInternalMiddlewareOrdering(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// A handler that panics, reached through the composed chain.
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	var wrapped http.Handler = panicking
	for _, mw := range InternalMiddlewares(InternalMiddlewareOptions{Logger: logger}) {
		wrapped = mw(wrapped)
	}

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/webhook", nil))

	// Recovery converted the panic into a response instead of crashing.
	require.Equal(t, http.StatusInternalServerError, rr.Code,
		"recovery middleware must convert a handler panic into a 500")

	// Recovery logged the panic. True in either ordering, so this alone proves
	// nothing about order — asserted only to show recovery ran.
	require.Contains(t, logs.String(), "Panic recovered")

	// The discriminating assertion. Logger emits "HTTP request" *after* calling
	// next, so that line survives a panicking handler only when logger is
	// OUTSIDE recovery: recovery absorbs the panic and returns normally, and
	// control then unwinds back through logger.
	//
	// Reverse InternalMiddlewares and recovery becomes outermost — the panic
	// unwinds straight past logger, its post-next logging never runs, and the
	// request vanishes from the access log while still returning a 500. That is
	// the silent failure this test exists to catch.
	assert.Contains(t, logs.String(), "HTTP request",
		"logger must sit outside recovery, so a panicking request is still access-logged")
}

// TestInternalMiddlewaresAppliedByGeneratedRouter confirms the chain is actually
// attached when routes are registered the way cmd/observer registers them —
// InternalMiddlewares being correct is no use if HandlerWithOptions is wired
// without it.
func TestInternalMiddlewaresAppliedByGeneratedRouter(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// panicHandler satisfies the strict interface for the one operation this
	// test exercises and panics on it; the rest are never called.
	strict := internalgen.NewStrictHandlerWithOptions(
		&panicOnWebhookHandler{},
		nil,
		internalgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  StrictRequestErrorHandler(logger),
			ResponseErrorHandlerFunc: StrictResponseErrorHandler(logger),
		},
	)

	srv := internalgen.HandlerWithOptions(strict, internalgen.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		Middlewares:      InternalMiddlewares(InternalMiddlewareOptions{Logger: logger}),
		ErrorHandlerFunc: ParamBindingErrorHandler(logger),
	})

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, webhookURL,
		strings.NewReader(`{"ruleName":"r","ruleNamespace":"ns"}`)))

	assert.Equal(t, http.StatusInternalServerError, rr.Code,
		"recovery must be attached to the generated routes")
	assert.Contains(t, logs.String(), "HTTP request",
		"logger must be attached to the generated routes, outside recovery")
}

// panicOnWebhookHandler implements the internal strict interface, panicking on
// the webhook operation so the middleware chain around it can be observed.
type panicOnWebhookHandler struct{}

var _ internalgen.StrictServerInterface = (*panicOnWebhookHandler)(nil)

func (*panicOnWebhookHandler) CreateAlertRule(
	_ context.Context, _ internalgen.CreateAlertRuleRequestObject,
) (internalgen.CreateAlertRuleResponseObject, error) {
	panic("not exercised by this test")
}

func (*panicOnWebhookHandler) DeleteAlertRule(
	_ context.Context, _ internalgen.DeleteAlertRuleRequestObject,
) (internalgen.DeleteAlertRuleResponseObject, error) {
	panic("not exercised by this test")
}

func (*panicOnWebhookHandler) GetAlertRule(
	_ context.Context, _ internalgen.GetAlertRuleRequestObject,
) (internalgen.GetAlertRuleResponseObject, error) {
	panic("not exercised by this test")
}

func (*panicOnWebhookHandler) UpdateAlertRule(
	_ context.Context, _ internalgen.UpdateAlertRuleRequestObject,
) (internalgen.UpdateAlertRuleResponseObject, error) {
	panic("not exercised by this test")
}

func (*panicOnWebhookHandler) HandleAlertWebhook(
	_ context.Context, _ internalgen.HandleAlertWebhookRequestObject,
) (internalgen.HandleAlertWebhookResponseObject, error) {
	panic("boom")
}
