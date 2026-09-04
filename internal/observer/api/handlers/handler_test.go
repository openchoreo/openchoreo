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
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// TestObserverMiddlewaresLeaveHealthPublic drives the real public chain,
// including auth.OpenAPIAuth, and asserts the spec decides which routes need a
// token.
//
// /health goes through the same chain as everything else, and stays public only
// because the generated wrapper sets no scopes context key for it — which
// happens only because observer-api.yaml marks it `security: []`.
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
// as outermost. oapi-codegen applies its slice in the opposite direction — it
// wraps in order, so the LAST entry ends up outermost — which is why
// InternalMiddlewares returns {recovery, logger}.
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

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	var wrapped http.Handler = panicking
	for _, mw := range InternalMiddlewares(InternalMiddlewareOptions{Logger: logger}) {
		wrapped = mw(wrapped)
	}

	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/webhook", nil))

	require.Equal(t, http.StatusInternalServerError, rr.Code,
		"recovery middleware must convert a handler panic into a 500")

	// True in either ordering, so this proves nothing about order — asserted
	// only to show recovery ran.
	require.Contains(t, logs.String(), "Panic recovered")

	// The discriminating assertion. Logger emits "HTTP request" *after* calling
	// next, so that line survives a panicking handler only when logger is
	// OUTSIDE recovery: recovery absorbs the panic and returns normally, and
	// control then unwinds back through logger. With recovery outermost the
	// panic unwinds straight past logger and the request vanishes from the
	// access log while still returning a 500.
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
