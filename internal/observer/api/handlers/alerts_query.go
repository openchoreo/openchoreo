// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

// QueryAlerts handles POST /api/v1alpha1/alerts/query
func (h *Handler) QueryAlerts(
	ctx context.Context,
	request gen.QueryAlertsRequestObject,
) (gen.QueryAlertsResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required"), nil
	}
	req := *request.Body

	if err := ValidateAlertsQueryRequest(&req); err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error()), nil
	}

	if h.alertIncidentService == nil {
		return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
			"SERVICE_NOT_READY", "alerts querier is not initialized"), nil
	}

	resp, err := h.alertIncidentService.QueryAlerts(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			return errorResponse(http.StatusServiceUnavailable, gen.InternalServerError,
				"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable"), nil
		case errors.Is(err, service.ErrAlertsResolveSearchScope):
			if errors.Is(err, service.ErrScopeNotFound) {
				return errorResponse(http.StatusBadRequest, gen.BadRequest,
					"SCOPE_NOT_FOUND", "one or more resources in the search scope were not found"), nil
			}
			h.logger.Error("Failed to resolve alerts search scope", "error", err)
			return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
				"RESOLVE_SCOPE_FAILED", "failed to resolve search scope"), nil
		default:
			h.logger.Error("Failed to query alerts", "error", err)
			return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
				"QUERY_ALERTS_FAILED", "failed to query alerts"), nil
		}
	}

	return jsonResponse(http.StatusOK, resp), nil
}

// QueryIncidents handles POST /api/v1alpha1/incidents/query
func (h *Handler) QueryIncidents(
	ctx context.Context,
	request gen.QueryIncidentsRequestObject,
) (gen.QueryIncidentsResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required"), nil
	}
	req := *request.Body

	if err := ValidateIncidentsQueryRequest(&req); err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error()), nil
	}

	if h.alertIncidentService == nil {
		return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
			"SERVICE_NOT_READY", "incidents querier is not initialized"), nil
	}

	resp, err := h.alertIncidentService.QueryIncidents(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			return errorResponse(http.StatusServiceUnavailable, gen.InternalServerError,
				"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable"), nil
		default:
			h.logger.Error("Failed to query incidents", "error", err)
			return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
				"QUERY_INCIDENTS_FAILED", "failed to query incidents"), nil
		}
	}

	return jsonResponse(http.StatusOK, resp), nil
}

// UpdateIncident handles PUT /api/v1alpha1/incidents/{incidentId}
//
// incidentId is bound by the generated router. The TrimSpace-and-reject check is
// kept because {incidentId} can still match a whitespace-only segment, which
// the mux treats as non-empty.
func (h *Handler) UpdateIncident(
	ctx context.Context,
	request gen.UpdateIncidentRequestObject,
) (gen.UpdateIncidentResponseObject, error) {
	id := strings.TrimSpace(request.IncidentId)
	if id == "" {
		return errorResponse(http.StatusBadRequest, gen.BadRequest,
			"INVALID_INCIDENT_ID", "incidentId path parameter is required"), nil
	}

	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required"), nil
	}
	req := *request.Body

	if err := ValidateIncidentPutRequest(&req); err != nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error()), nil
	}

	if h.alertIncidentService == nil {
		return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
			"SERVICE_NOT_READY", "incident update service is not initialized"), nil
	}

	resp, err := h.alertIncidentService.UpdateIncident(ctx, id, req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			return errorResponse(http.StatusServiceUnavailable, gen.InternalServerError,
				"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable"), nil
		case errors.Is(err, incidententry.ErrIncidentNotFound):
			return errorResponse(http.StatusNotFound, gen.NotFound,
				"INCIDENT_NOT_FOUND", "incident not found"), nil
		case errors.Is(err, incidententry.ErrInvalidStatusTransition):
			return errorResponse(http.StatusBadRequest, gen.BadRequest,
				"INVALID_STATUS_TRANSITION", err.Error()), nil
		default:
			h.logger.Error("Failed to update incident", "error", err)
			return errorResponse(http.StatusInternalServerError, gen.InternalServerError,
				"UPDATE_INCIDENT_FAILED", "failed to update incident"), nil
		}
	}

	return jsonResponse(http.StatusOK, resp), nil
}
