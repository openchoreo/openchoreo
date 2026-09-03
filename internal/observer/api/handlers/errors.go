// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"log/slog"
	"mime"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
)

// The generated servers default every error they raise before (or around) a
// handler to plain text via http.Error:
//
//   - StdHTTPServerOptions.ErrorHandlerFunc      — parameter binding failures
//   - StrictHTTPServerOptions.RequestErrorHandlerFunc  — undecodable JSON body
//   - StrictHTTPServerOptions.ResponseErrorHandlerFunc — response write failures
//
// Every observer error a handler itself produces is a gen.ErrorResponse JSON
// body. The three functions below replace all of those defaults, so the error
// contract is the same regardless of which layer rejected the request.

// errorPayload builds a gen.ErrorResponse. Shared by the handler-facing
// errorResponse in response.go and by the generated-server hooks below, so both
// emit the same shape.
func errorPayload(title gen.ErrorResponseTitle, errorCode, message string) gen.ErrorResponse {
	return gen.ErrorResponse{
		Title:     &title,
		ErrorCode: &errorCode,
		Message:   &message,
	}
}

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
	payload := errorPayload(title, errorCode, err.Error())
	if writeErr := httputil.WriteJSON(w, status, payload); writeErr != nil {
		logger.Error("Failed to write generated-server error response",
			"error", writeErr, "original_error", err)
	}
}

// ParamBindingErrorHandler renders parameter-binding failures (a missing
// required query parameter, an unparseable path parameter) as gen.ErrorResponse.
//
// Pass as StdHTTPServerOptions.ErrorHandlerFunc. Without it, a request such as
// a FinOps cost query missing startTime returns a plain-text 400 rather than
// observer's JSON error shape.
//
// Unreachable on the internal port: both internal path parameters bind into a
// plain string and the spec's sourceType enum generates no runtime check, so
// binding cannot fail there. The public FinOps operations reach it, since their
// startTime/endTime are required non-pointer query parameters.
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
// Pass as StrictHTTPServerOptions.RequestErrorHandlerFunc.
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

// RequireJSONContentType rejects a request that carries a body with a
// Content-Type other than application/json.
//
// The generated strict layer decodes bodies unconditionally, so without this a
// form-encoded or text/plain POST would be parsed as JSON. It is applied
// uniformly to every body-bearing public operation, all of which declare
// `content: application/json` in the spec.
//
// An absent Content-Type is allowed. The rejection is a 400 rather than the
// arguably more correct 415 because the spec declares no 415 response and
// gen.ErrorResponseTitle has no value for one.
func RequireJSONContentType(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Content-Type")
			// No body, or no declared type: nothing to reject.
			if raw == "" || r.ContentLength == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// ParseMediaType handles parameters such as "; charset=utf-8".
			mediaType, _, err := mime.ParseMediaType(raw)
			if err == nil && mediaType == "application/json" {
				next.ServeHTTP(w, r)
				return
			}

			logger.Debug("Rejected request with non-JSON Content-Type",
				"method", r.Method, "path", r.URL.Path, "content_type", raw)
			payload := errorPayload(gen.BadRequest, "", "Invalid request format")
			if writeErr := httputil.WriteJSON(w, http.StatusBadRequest, payload); writeErr != nil {
				logger.Error("Failed to write Content-Type error response", "error", writeErr)
			}
		})
	}
}
