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
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
)

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
