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

// QueryLogs handles POST /api/v1/logs/query
func (h *Handler) QueryLogs(
	ctx context.Context,
	request gen.QueryLogsRequestObject,
) (gen.QueryLogsResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	req, err := toTypesLogsQuery(*request.Body)
	if err != nil {
		// Reached when the searchScope oneOf is malformed or mixes scopes --
		// types.SearchScope.UnmarshalJSON rejects it. Pre-migration this came
		// out of httputil.BindJSON with the same status and message.
		h.logger.Error("Failed to bind request", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	// Validate request
	if err := ValidateLogsQueryRequest(req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.logsService == nil {
		h.logger.Error("Logs service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1LogsServiceNotReady,
			"Logs service is not initialized",
		), nil
	}

	result, err := h.logsService.QueryLogs(ctx, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		}
		h.logger.Error("Failed to query logs", "error", err)
		errorCode := types.ErrorCodeV1LogsInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			return errorResponse(
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			), nil
		case errors.Is(err, service.ErrLogsResolveSearchScope):
			errorCode = types.ErrorCodeV1LogsResolverFailed
		case errors.Is(err, service.ErrLogsRetrieval):
			errorCode = types.ErrorCodeV1LogsRetrievalFailed
		}
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve logs",
		), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}
