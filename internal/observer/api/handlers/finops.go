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
)

// GetComponentCosts handles
// GET /api/v1alpha1/costs/namespaces/{namespace}/environments/{environment}
//
// startTime and endTime are declared required in the spec, so a missing one is
// now rejected by the generated wrapper before this runs -- see
// ParamBindingErrorHandler, which renders that as gen.ErrorResponse rather than
// the generated default's plain text.
func (h *Handler) GetComponentCosts(
	ctx context.Context,
	request gen.GetComponentCostsRequestObject,
) (gen.GetComponentCostsResponseObject, error) {
	req := toTypesCostQuery(request.Namespace, request.Environment, request.Params)

	if err := ValidateCostQueryRequest(req); err != nil {
		h.logger.Debug("Cost query request validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.finOpsService == nil {
		h.logger.Error("FinOps service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			"",
			"FinOps service is not initialized",
		), nil
	}

	result, err := h.finOpsService.GetComponentCosts(ctx, req)
	if err != nil {
		return h.finOpsError(err, "Failed to retrieve costs"), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// GetRecommendations handles
// GET /api/v1alpha1/costs/namespaces/{namespace}/environments/{environment}/recommendations
func (h *Handler) GetRecommendations(
	ctx context.Context,
	request gen.GetRecommendationsRequestObject,
) (gen.GetRecommendationsResponseObject, error) {
	req := toTypesRecommendationQuery(request.Namespace, request.Environment, request.Params)

	if err := ValidateRecommendationQueryRequest(req); err != nil {
		h.logger.Debug("Recommendation query request validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	if h.finOpsService == nil {
		h.logger.Error("FinOps service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			"",
			"FinOps service is not initialized",
		), nil
	}

	result, err := h.finOpsService.GetRecommendations(ctx, req)
	if err != nil {
		return h.finOpsError(err, "Failed to retrieve recommendations"), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// finOpsError maps FinOps service errors to the shared response type.
func (h *Handler) finOpsError(err error, genericMessage string) apiResponse {
	if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
		return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied")
	}
	if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
		return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
	}
	if errors.Is(err, service.ErrFinOpsInvalidRequest) {
		h.logger.Debug("Invalid finops request", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error())
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
	return errorResponse(http.StatusInternalServerError, gen.InternalServerError, "", message)
}
