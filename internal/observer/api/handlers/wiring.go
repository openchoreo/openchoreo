// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	observermiddleware "github.com/openchoreo/openchoreo/internal/observer/middleware"
)

// The generated servers default every error they raise before (or around) a
// handler to plain text via http.Error:
//
//   - StdHTTPServerOptions.ErrorHandlerFunc      — parameter binding failures
//   - StrictHTTPServerOptions.RequestErrorHandlerFunc  — undecodable JSON body
//   - StrictHTTPServerOptions.ResponseErrorHandlerFunc — response write failures
//
// Every observer error the handlers themselves produce is a gen.ErrorResponse
// JSON body. Leaving the defaults in place would mean a request that fails
// validation *inside* a handler gets JSON while one that fails a step earlier
// gets plain text, for no reason a client could predict. The three functions
// below replace all of them, so the error contract is uniform regardless of
// which layer rejected the request.

// writeGeneratedError renders a gen.ErrorResponse for an error raised by the
// generated server rather than by a handler.
func writeGeneratedError(
	logger *slog.Logger,
	w http.ResponseWriter,
	status int,
	title gen.ErrorResponseTitle,
	errorCode string,
	err error,
) {
	message := err.Error()
	payload := gen.ErrorResponse{
		Title:     &title,
		ErrorCode: &errorCode,
		Message:   &message,
	}
	if writeErr := httputil.WriteJSON(w, status, payload); writeErr != nil {
		logger.Error("Failed to write generated-server error response",
			"error", writeErr, "original_error", err)
	}
}

// ParamBindingErrorHandler renders parameter-binding failures (a missing
// required query parameter, an unparseable path parameter) as gen.ErrorResponse.
//
// Pass as StdHTTPServerOptions.ErrorHandlerFunc. Without it, a request such as
// a FinOps cost query missing startTime returns a plain-text 400 — the one place
// this migration would otherwise not be behavior-preserving, since today that
// case is caught inside the handler and returned as JSON.
//
// Currently unreachable on the internal port: both internal path parameters
// bind into a plain string and the spec's sourceType enum generates no runtime
// check, so binding cannot fail. It becomes reachable with the public API's
// FinOps operations, whose startTime/endTime are required non-pointer query
// parameters.
//
// It logs the rejection itself rather than leaving that to the logger
// middleware, because the generated wrapper calls ErrorHandlerFunc *before*
// applying HandlerMiddlewares — so these responses bypass both the logger and
// recovery. Do not remove the log line on the assumption the chain covers it.
func ParamBindingErrorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		// Info, not Debug: this is the only record of the request, since the
		// access log never sees it (see the doc comment above).
		logger.Info("Rejected request during parameter binding",
			"method", r.Method, "path", r.URL.Path, "error", err)
		writeGeneratedError(logger, w, http.StatusBadRequest,
			gen.BadRequest, "INVALID_PARAMETER", err)
	}
}

// StrictRequestErrorHandler renders request-decoding failures — in practice a
// malformed JSON body — as gen.ErrorResponse.
//
// Pass as StrictHTTPServerOptions.RequestErrorHandlerFunc. The error code
// matches what the handlers used to return for this case when they decoded
// bodies themselves, so the contract is unchanged.
func StrictRequestErrorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Debug("Rejected request during body decoding",
			"method", r.Method, "path", r.URL.Path, "error", err)
		writeGeneratedError(logger, w, http.StatusBadRequest,
			gen.BadRequest, "INVALID_REQUEST_BODY", err)
	}
}

// StrictResponseErrorHandler renders response-writing failures as
// gen.ErrorResponse.
//
// Pass as StrictHTTPServerOptions.ResponseErrorHandlerFunc. Reaching this means
// a Visit*Response failed or a handler returned an unexpected type, both of
// which are server faults, so it logs at error level.
func StrictResponseErrorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("Failed to write handler response",
			"method", r.Method, "path", r.URL.Path, "error", err)
		writeGeneratedError(logger, w, http.StatusInternalServerError,
			gen.InternalServerError, "RESPONSE_WRITE_FAILED", err)
	}
}

// InternalMiddlewareOptions carries the dependencies InternalMiddlewares needs.
type InternalMiddlewareOptions struct {
	Logger *slog.Logger
}

// InternalMiddlewares returns the ordered middleware chain for the generated
// internal OpenAPI routes (port 8081).
//
// oapi-codegen applies these last-to-first, so the last entry is outermost:
//
//	logger → recovery → handler
//
// This reproduces the chain the internal mux carried before the migration,
// where middleware.NewRouteBuilder(internalMux).With(logger, recovery) applied
// the same two in the same order.
//
// There is deliberately no auth middleware here. The internal API declares no
// security scheme, because the internal port has no JWT layer and the
// ObservabilityAlertRule controller that drives alert rule CRUD sends no
// Authorization header. See TestInternalSpecDeclaresNoSecurity, and do not add
// auth here without the controller-side token work that must accompany it.
//
// This is the single definition of the chain — main.go supplies dependencies but
// owns no ordering, so a wiring test can drive exactly what production runs.
func InternalMiddlewares(opts InternalMiddlewareOptions) []internalgen.MiddlewareFunc {
	return []internalgen.MiddlewareFunc{
		observermiddleware.Recovery(opts.Logger),
		observermiddleware.Logger(opts.Logger),
	}
}
