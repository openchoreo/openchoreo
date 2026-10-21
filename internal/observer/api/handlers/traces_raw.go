// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryRawTraces handles POST /api/v1alpha1/traces/raw-query. The body is the
// tracing backend's native request, as JSON or as text; the response is the
// backend's own response, unchanged.
func (h *Handler) QueryRawTraces(
	ctx context.Context,
	request gen.QueryRawTracesRequestObject,
) (gen.QueryRawTracesResponseObject, error) {
	var req types.RawTracesQueryRequest
	switch {
	case request.JSONBody != nil:
		req = types.RawTracesQueryRequest{ContentType: "application/json", Body: *request.JSONBody}
		if trimmed := bytes.TrimSpace(req.Body); len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			return errorResponse(http.StatusBadRequest, gen.BadRequest,
				types.ErrorCodeV1TracesInvalidRequest, "the request body is empty"), nil
		}
	case request.TextBody != nil:
		req = types.RawTracesQueryRequest{ContentType: "text/plain", Body: []byte(*request.TextBody)}
		if strings.TrimSpace(*request.TextBody) == "" {
			return errorResponse(http.StatusBadRequest, gen.BadRequest,
				types.ErrorCodeV1TracesInvalidRequest, "the request body is empty"), nil
		}
	default:
		return errorResponse(http.StatusBadRequest, gen.BadRequest, types.ErrorCodeV1TracesInvalidRequest,
			"a request body is required, as application/json or text/plain"), nil
	}

	if h.tracesService == nil {
		h.logger.Error("Traces service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1TracesServiceNotReady,
			"Traces service is not initialized",
		), nil
	}

	// Authorization is enforced by the service layer.
	result, err := h.tracesService.QueryRawTraces(ctx, &req)
	if err != nil {
		return h.rawTracesErrorResponse(err), nil
	}

	return jsonResponse(http.StatusOK, result.Body), nil
}

func (h *Handler) rawTracesErrorResponse(err error) apiResponse {
	var rejected *service.RawTracesRejectedError
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
	case errors.Is(err, service.ErrRawTracesInvalidRequest):
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			types.ErrorCodeV1TracesInvalidRequest, strings.TrimPrefix(err.Error(), service.ErrRawTracesInvalidRequest.Error()+": "))
	case errors.As(err, &rejected):
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			types.ErrorCodeV1TracesRawQueryRejected, rejected.Detail)
	case errors.Is(err, service.ErrRawTracesResultTooLarge):
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			types.ErrorCodeV1TracesRawQueryResultTooLarge,
			"The result is larger than the maximum response size; narrow the query or add a limit")
	case errors.Is(err, service.ErrRawTracesRequestTooLarge):
		return errorResponse(http.StatusRequestEntityTooLarge, gen.PayloadTooLarge,
			types.ErrorCodeV1TracesRawQueryRequestTooLarge, "The request is larger than the tracing backend accepts")
	case errors.Is(err, service.ErrRawTracesUnsupportedMediaType):
		return errorResponse(http.StatusUnsupportedMediaType, gen.UnsupportedMediaType,
			types.ErrorCodeV1TracesRawQueryMediaType, "The tracing backend does not accept raw queries in this media type")
	case errors.Is(err, service.ErrRawTracesNotSupported):
		return errorResponse(http.StatusNotImplemented, gen.NotImplemented,
			types.ErrorCodeV1TracesRawQueryNotSupported, "The tracing backend does not support raw queries")
	case errors.Is(err, service.ErrRawTracesTimeout):
		return errorResponse(http.StatusGatewayTimeout, gen.GatewayTimeout,
			types.ErrorCodeV1TracesRawQueryTimeout, "The raw query ran past the query timeout")
	}

	h.logger.Error("Failed to run raw trace query", "error", err)
	errorCode := types.ErrorCodeV1TracesInternalGeneric
	if errors.Is(err, service.ErrTracesRetrieval) {
		errorCode = types.ErrorCodeV1TracesRetrievalFailed
	}
	return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
		errorCode, "Failed to run the raw trace query")
}
