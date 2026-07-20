// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// GetComponentCosts handles
// GET /api/v1alpha1/costs/namespaces/{namespace}/environments/{environment}
func (h *Handler) GetComponentCosts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	req := types.CostQueryRequest{
		Namespace:   r.PathValue("namespace"),
		Environment: r.PathValue("environment"),
		Project:     query.Get("project"),
		Component:   query.Get("component"),
		StartTime:   query.Get("startTime"),
		EndTime:     query.Get("endTime"),
		Granularity: query.Get("granularity"),
	}

	if err := ValidateCostQueryRequest(&req); err != nil {
		h.logger.Debug("Cost query request validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	if h.finOpsService == nil {
		h.logger.Error("FinOps service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			"",
			"FinOps service is not initialized",
		)
		return
	}

	result, err := h.finOpsService.GetComponentCosts(r.Context(), &req)
	if err != nil {
		h.writeFinOpsError(w, err, "Failed to retrieve costs")
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetRecommendations handles
// GET /api/v1alpha1/costs/namespaces/{namespace}/environments/{environment}/recommendations
func (h *Handler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	req := types.RecommendationQueryRequest{
		Namespace:   r.PathValue("namespace"),
		Environment: r.PathValue("environment"),
		Project:     query.Get("project"),
		Component:   query.Get("component"),
		StartTime:   query.Get("startTime"),
		EndTime:     query.Get("endTime"),
	}

	if err := ValidateRecommendationQueryRequest(&req); err != nil {
		h.logger.Debug("Recommendation query request validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	if h.finOpsService == nil {
		h.logger.Error("FinOps service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			"",
			"FinOps service is not initialized",
		)
		return
	}

	result, err := h.finOpsService.GetRecommendations(r.Context(), &req)
	if err != nil {
		h.writeFinOpsError(w, err, "Failed to retrieve recommendations")
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// writeFinOpsError maps FinOps service errors to HTTP responses.
func (h *Handler) writeFinOpsError(w http.ResponseWriter, err error, genericMessage string) {
	if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
		h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
		return
	}
	if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
		h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
		return
	}
	if errors.Is(err, service.ErrFinOpsInvalidRequest) {
		h.logger.Debug("Invalid finops request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	message := genericMessage
	switch {
	case errors.Is(err, service.ErrScopeAuthFailed):
		message = "Failed to authenticate scope resolution request"
	case errors.Is(err, service.ErrFinOpsResolveScope):
		message = "Failed to resolve resource names to UIDs"
	case errors.Is(err, service.ErrFinOpsRetrieval):
		message = "Failed to retrieve data from the FinOps adapter"
	}

	h.logger.Error("Failed to query cost data through the FinOps adapter", "error", err)
	h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "", message)
}
