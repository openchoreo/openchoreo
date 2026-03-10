// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

// QueryAlerts handles POST /api/v1alpha1/alerts/query
func (h *Handler) QueryAlerts(w http.ResponseWriter, r *http.Request) {
	var req gen.AlertsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if err := ValidateAlertsQueryRequest(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if h.alertIncidentService == nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "SERVICE_NOT_READY", "alerts querier is not initialized")
		return
	}

	resp, err := h.alertIncidentService.QueryAlerts(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			h.writeErrorResponse(w, http.StatusServiceUnavailable, gen.InternalServerError, "AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")
		case errors.Is(err, service.ErrAlertsResolveSearchScope):
			if errors.Is(err, service.ErrScopeNotFound) {
				h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "SCOPE_NOT_FOUND", "one or more resources in the search scope were not found")
			} else {
				h.logger.Error("Failed to resolve alerts search scope", "error", err)
				h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "RESOLVE_SCOPE_FAILED", "failed to resolve search scope")
			}
		default:
			h.logger.Error("Failed to query alerts", "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "QUERY_ALERTS_FAILED", "failed to query alerts")
		}
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// QueryIncidents handles POST /api/v1alpha1/incidents/query
func (h *Handler) QueryIncidents(w http.ResponseWriter, r *http.Request) {
	var req gen.IncidentsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if err := ValidateIncidentsQueryRequest(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if h.alertIncidentService == nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "SERVICE_NOT_READY", "incidents querier is not initialized")
		return
	}

	resp, err := h.alertIncidentService.QueryIncidents(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			h.writeErrorResponse(w, http.StatusServiceUnavailable, gen.InternalServerError, "AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")
		default:
			h.logger.Error("Failed to query incidents", "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "QUERY_INCIDENTS_FAILED", "failed to query incidents")
		}
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// UpdateIncident handles PUT /api/v1alpha1/incidents/{incidentId}
// Note: the incident ID is read from the path via r.PathValue to be compatible with http.ServeMux routing.
func (h *Handler) UpdateIncident(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("incidentId"))
	if id == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_INCIDENT_ID", "incidentId path parameter is required")
		return
	}

	var req gen.IncidentPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if err := ValidateIncidentPutRequest(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if h.alertIncidentService == nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "SERVICE_NOT_READY", "incident update service is not initialized")
		return
	}

	resp, err := h.alertIncidentService.UpdateIncident(r.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, observerAuthz.ErrAuthzForbidden):
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
		case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
		case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
			errors.Is(err, observerAuthz.ErrAuthzTimeout):
			h.writeErrorResponse(w, http.StatusServiceUnavailable, gen.InternalServerError, "AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")
		case errors.Is(err, incidententry.ErrIncidentNotFound):
			h.writeErrorResponse(w, http.StatusNotFound, gen.NotFound, "INCIDENT_NOT_FOUND", "incident not found")
		case errors.Is(err, incidententry.ErrInvalidStatusTransition):
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_STATUS_TRANSITION", err.Error())
		default:
			h.logger.Error("Failed to update incident", "error", err)
			h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "UPDATE_INCIDENT_FAILED", "failed to update incident")
		}
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}
