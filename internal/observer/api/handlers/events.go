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

// QueryEvents handles POST /api/v1/events/query
func (h *Handler) QueryEvents(
	ctx context.Context,
	request gen.QueryEventsRequestObject,
) (gen.QueryEventsResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	req, err := toTypesEventsQuery(*request.Body)
	if err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	// Validate request
	if err := ValidateEventsQueryRequest(req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.eventsService == nil {
		h.logger.Error("Events service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1EventsServiceNotReady,
			"Events service is not initialized",
		), nil
	}

	result, err := h.eventsService.QueryEvents(ctx, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		}
		if errors.Is(err, service.ErrEventsNotImplemented) {
			return errorResponse(
				http.StatusNotImplemented,
				gen.NotImplemented,
				types.ErrorCodeV1EventsNotImplemented,
				"Events query is not implemented by the configured logs adapter",
			), nil
		}
		h.logger.Error("Failed to query events", "error", err)
		errorCode := types.ErrorCodeV1EventsInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			return errorResponse(
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			), nil
		case errors.Is(err, service.ErrEventsResolveSearchScope):
			errorCode = types.ErrorCodeV1EventsResolverFailed
		case errors.Is(err, service.ErrEventsRetrieval):
			errorCode = types.ErrorCodeV1EventsRetrievalFailed
		}
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve events",
		), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}
