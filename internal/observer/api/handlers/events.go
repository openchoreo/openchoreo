// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryEvents handles POST /api/v1/events/query
func (h *Handler) QueryEvents(w http.ResponseWriter, r *http.Request) {
	var req types.EventsQueryRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid request format")
		return
	}

	// Validate request
	if err := ValidateEventsQueryRequest(&req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	ctx := r.Context()
	if h.eventsService == nil {
		h.logger.Error("Events service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1EventsServiceNotReady,
			"Events service is not initialized",
		)
		return
	}
	result, err := h.eventsService.QueryEvents(ctx, &req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
			return
		}
		h.logger.Error("Failed to query events", "error", err)
		errorCode := types.ErrorCodeV1EventsInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			h.writeErrorResponse(
				w,
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			)
			return
		case errors.Is(err, service.ErrEventsResolveSearchScope):
			errorCode = types.ErrorCodeV1EventsResolverFailed
		case errors.Is(err, service.ErrEventsRetrieval):
			errorCode = types.ErrorCodeV1EventsRetrievalFailed
		}
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve events",
		)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}
