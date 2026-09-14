// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// GetPlatformLogs handles GET /api/v1alpha1/platform-logs.
func (h *Handler) GetPlatformLogs(
	ctx context.Context,
	request gen.GetPlatformLogsRequestObject,
) (gen.GetPlatformLogsResponseObject, error) {
	req, err := toTypesPlatformLogsQuery(request.Params)
	if err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if err := ValidatePlatformLogsQueryRequest(req); err != nil {
		h.logger.Debug("Platform logs request validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.platformLogsService == nil {
		h.logger.Error("Platform logs service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1PlatformLogsServiceNotReady,
			"Platform logs service is not initialized",
		), nil
	}

	result, err := h.platformLogsService.QueryPlatformLogs(ctx, req)
	if err != nil {
		return h.platformLogsError(err), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// platformLogsError maps platform logs service errors onto responses.
func (h *Handler) platformLogsError(err error) gen.GetPlatformLogsResponseObject {
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
	case errors.Is(err, service.ErrPlatformLogsNotSupported):
		h.logger.Warn("Logs adapter does not support platform logs")
		return errorResponse(
			http.StatusNotImplemented,
			gen.NotImplemented,
			types.ErrorCodeV1PlatformLogsNotSupported,
			"The configured logs adapter does not support platform logs",
		)
	}

	errorCode := types.ErrorCodeV1PlatformLogsInternalGeneric
	if errors.Is(err, service.ErrPlatformLogsRetrieval) {
		errorCode = types.ErrorCodeV1PlatformLogsRetrievalFailed
	}
	h.logger.Error("Failed to retrieve platform logs", "error", err)
	return errorResponse(
		http.StatusInternalServerError,
		gen.InternalServerError,
		errorCode,
		"Failed to retrieve platform logs",
	)
}

// --- filter values ---

// GetPlatformLogFilterValues handles
// GET /api/v1alpha1/platform-logs/filter-values.
func (h *Handler) GetPlatformLogFilterValues(
	ctx context.Context,
	request gen.GetPlatformLogFilterValuesRequestObject,
) (gen.GetPlatformLogFilterValuesResponseObject, error) {
	req, err := toTypesPlatformLogFilterValuesQuery(request.Params)
	if err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if err := ValidatePlatformLogFilterValuesRequest(req); err != nil {
		h.logger.Debug("Platform log filter values request validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.platformLogsService == nil {
		h.logger.Error("Platform log filter values service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1PlatformLogFilterValuesServiceNotReady,
			"Platform log filter values service is not initialized",
		), nil
	}

	result, err := h.platformLogsService.QueryPlatformLogFilterValues(ctx, req)
	if err != nil {
		return h.platformLogFilterValuesError(err), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// platformLogFilterValuesError maps filter values service errors onto responses.
func (h *Handler) platformLogFilterValuesError(err error) gen.GetPlatformLogFilterValuesResponseObject {
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
	case errors.Is(err, service.ErrPlatformLogFilterValuesNotSupported):
		h.logger.Warn("Logs adapter cannot enumerate platform log filter values")
		return errorResponse(
			http.StatusNotImplemented,
			gen.NotImplemented,
			types.ErrorCodeV1PlatformLogFilterValuesNotSupported,
			"The configured logs adapter does not support platform log filter values",
		)
	}

	errorCode := types.ErrorCodeV1PlatformLogsInternalGeneric
	if errors.Is(err, service.ErrPlatformLogFilterValuesRetrieval) {
		errorCode = types.ErrorCodeV1PlatformLogFilterValuesRetrievalFailed
	}
	h.logger.Error("Failed to retrieve platform log filter values", "error", err)
	return errorResponse(
		http.StatusInternalServerError,
		gen.InternalServerError,
		errorCode,
		"Failed to retrieve platform log filter values",
	)
}
