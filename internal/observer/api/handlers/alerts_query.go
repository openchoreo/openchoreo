// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

// QueryAlerts handles POST /api/v1alpha1/alerts/query
//
// Returns the generated typed responses rather than the apiResponse passthrough:
// the service already speaks gen.* and every status this handler produces is
// declared in the spec, so the compiler checks both the body shape and the
// status against the contract.
func (h *Handler) QueryAlerts(
	ctx context.Context,
	request gen.QueryAlertsRequestObject,
) (gen.QueryAlertsResponseObject, error) {
	if request.Body == nil {
		return gen.QueryAlerts400JSONResponse(errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required")), nil
	}
	req := *request.Body

	if err := ValidateAlertsQueryRequest(&req); err != nil {
		return gen.QueryAlerts400JSONResponse(errorPayload(gen.BadRequest,
			"VALIDATION_ERROR", err.Error())), nil
	}

	if h.alertIncidentService == nil {
		return gen.QueryAlerts500JSONResponse(errorPayload(gen.InternalServerError,
			"SERVICE_NOT_READY", "alerts querier is not initialized")), nil
	}

	resp, err := h.alertIncidentService.QueryAlerts(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			return gen.QueryAlerts403JSONResponse(errorPayload(gen.Forbidden, "", "Access denied")), nil
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			return gen.QueryAlerts401JSONResponse(errorPayload(gen.Unauthorized, "", "Unauthorized")), nil
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			return gen.QueryAlerts503JSONResponse(errorPayload(gen.InternalServerError,
				"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")), nil
		case errors.Is(err, service.ErrAlertsResolveSearchScope):
			if errors.Is(err, service.ErrScopeNotFound) {
				return gen.QueryAlerts400JSONResponse(errorPayload(gen.BadRequest,
					"SCOPE_NOT_FOUND", "one or more resources in the search scope were not found")), nil
			}
			h.logger.Error("Failed to resolve alerts search scope", "error", err)
			return gen.QueryAlerts500JSONResponse(errorPayload(gen.InternalServerError,
				"RESOLVE_SCOPE_FAILED", "failed to resolve search scope")), nil
		default:
			h.logger.Error("Failed to query alerts", "error", err)
			return gen.QueryAlerts500JSONResponse(errorPayload(gen.InternalServerError,
				"QUERY_ALERTS_FAILED", "failed to query alerts")), nil
		}
	}

	if resp == nil {
		h.logger.Error("Failed to query alerts", "error", errNilServiceResponse)
		return gen.QueryAlerts500JSONResponse(errorPayload(gen.InternalServerError,
			"QUERY_ALERTS_FAILED", "failed to query alerts")), nil
	}

	return gen.QueryAlerts200JSONResponse(*resp), nil
}

// QueryIncidents handles POST /api/v1alpha1/incidents/query
func (h *Handler) QueryIncidents(
	ctx context.Context,
	request gen.QueryIncidentsRequestObject,
) (gen.QueryIncidentsResponseObject, error) {
	if request.Body == nil {
		return gen.QueryIncidents400JSONResponse(errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required")), nil
	}
	req := *request.Body

	if err := ValidateIncidentsQueryRequest(&req); err != nil {
		return gen.QueryIncidents400JSONResponse(errorPayload(gen.BadRequest,
			"VALIDATION_ERROR", err.Error())), nil
	}

	if h.alertIncidentService == nil {
		return gen.QueryIncidents500JSONResponse(errorPayload(gen.InternalServerError,
			"SERVICE_NOT_READY", "incidents querier is not initialized")), nil
	}

	resp, err := h.alertIncidentService.QueryIncidents(ctx, req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			return gen.QueryIncidents403JSONResponse(errorPayload(gen.Forbidden, "", "Access denied")), nil
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			return gen.QueryIncidents401JSONResponse(errorPayload(gen.Unauthorized, "", "Unauthorized")), nil
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			return gen.QueryIncidents503JSONResponse(errorPayload(gen.InternalServerError,
				"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")), nil
		default:
			h.logger.Error("Failed to query incidents", "error", err)
			return gen.QueryIncidents500JSONResponse(errorPayload(gen.InternalServerError,
				"QUERY_INCIDENTS_FAILED", "failed to query incidents")), nil
		}
	}

	if resp == nil {
		h.logger.Error("Failed to query incidents", "error", errNilServiceResponse)
		return gen.QueryIncidents500JSONResponse(errorPayload(gen.InternalServerError,
			"QUERY_INCIDENTS_FAILED", "failed to query incidents")), nil
	}

	return gen.QueryIncidents200JSONResponse(*resp), nil
}

// UpdateIncident handles PUT /api/v1alpha1/incidents/{incidentId}
//
// incidentId is bound by the generated router, which rejects an empty path
// segment but not a whitespace-only one — hence the TrimSpace check below.
func (h *Handler) UpdateIncident(
	ctx context.Context,
	request gen.UpdateIncidentRequestObject,
) (gen.UpdateIncidentResponseObject, error) {
	id := strings.TrimSpace(request.IncidentId)
	if id == "" {
		return gen.UpdateIncident400JSONResponse(errorPayload(gen.BadRequest,
			"INVALID_INCIDENT_ID", "incidentId path parameter is required")), nil
	}

	if request.Body == nil {
		return gen.UpdateIncident400JSONResponse(errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required")), nil
	}
	req := *request.Body

	if err := ValidateIncidentPutRequest(&req); err != nil {
		return gen.UpdateIncident400JSONResponse(errorPayload(gen.BadRequest,
			"VALIDATION_ERROR", err.Error())), nil
	}

	if h.alertIncidentService == nil {
		return gen.UpdateIncident500JSONResponse(errorPayload(gen.InternalServerError,
			"SERVICE_NOT_READY", "incident update service is not initialized")), nil
	}

	resp, err := h.alertIncidentService.UpdateIncident(ctx, id, req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			return gen.UpdateIncident403JSONResponse(errorPayload(gen.Forbidden, "", "Access denied")), nil
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			return gen.UpdateIncident401JSONResponse(errorPayload(gen.Unauthorized, "", "Unauthorized")), nil
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			return gen.UpdateIncident503JSONResponse(errorPayload(gen.InternalServerError,
				"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")), nil
		case errors.Is(err, incidententry.ErrIncidentNotFound):
			return gen.UpdateIncident404JSONResponse(errorPayload(gen.NotFound,
				"INCIDENT_NOT_FOUND", "incident not found")), nil
		case errors.Is(err, incidententry.ErrInvalidStatusTransition):
			return gen.UpdateIncident400JSONResponse(errorPayload(gen.BadRequest,
				"INVALID_STATUS_TRANSITION", err.Error())), nil
		default:
			h.logger.Error("Failed to update incident", "error", err)
			return gen.UpdateIncident500JSONResponse(errorPayload(gen.InternalServerError,
				"UPDATE_INCIDENT_FAILED", "failed to update incident")), nil
		}
	}

	if resp == nil {
		h.logger.Error("Failed to update incident", "error", errNilServiceResponse)
		return gen.UpdateIncident500JSONResponse(errorPayload(gen.InternalServerError,
			"UPDATE_INCIDENT_FAILED", "failed to update incident")), nil
	}

	return gen.UpdateIncident200JSONResponse(*resp), nil
}
