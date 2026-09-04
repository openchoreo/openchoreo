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

// QueryMetrics handles POST /api/v1/metrics/query
func (h *Handler) QueryMetrics(
	ctx context.Context,
	request gen.QueryMetricsRequestObject,
) (gen.QueryMetricsResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	req, err := toTypesMetricsQuery(*request.Body)
	if err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	// Validate request
	if err := ValidateMetricsQueryRequest(req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	// Guard against misconfigured deployments.
	if h.metricsService == nil {
		h.logger.Error("Metrics service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1MetricsServiceNotReady,
			"Metrics service is not initialized",
		), nil
	}

	result, err := h.metricsService.QueryMetrics(ctx, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		}
		h.logger.Error("Failed to query metrics", "error", err)
		errorCode := types.ErrorCodeV1MetricsInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			return errorResponse(
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			), nil
		case errors.Is(err, service.ErrMetricsInvalidRequest):
			h.logger.Debug("Invalid metrics request", "error", err)
			return errorResponse(http.StatusBadRequest, gen.BadRequest, errorCode, err.Error()), nil
		case errors.Is(err, service.ErrMetricsResolveSearchScope):
			errorCode = types.ErrorCodeV1MetricsResolverFailed
		case errors.Is(err, service.ErrMetricsRetrieval):
			errorCode = types.ErrorCodeV1MetricsRetrievalFailed
		}
		h.logger.Error("Failed to query metrics", "error", err)
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve metrics",
		), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}

// QueryRuntimeTopology handles POST /api/v1alpha1/metrics/runtime-topology.
func (h *Handler) QueryRuntimeTopology(
	ctx context.Context,
	request gen.QueryRuntimeTopologyRequestObject,
) (gen.QueryRuntimeTopologyResponseObject, error) {
	if request.Body == nil {
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	req, err := toTypesRuntimeTopology(*request.Body)
	if err != nil {
		h.logger.Error("Failed to bind runtime topology request", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", "Invalid request format"), nil
	}

	if err := ValidateRuntimeTopologyRequest(req); err != nil {
		h.logger.Debug("Runtime topology validation failed", "error", err)
		return errorResponse(http.StatusBadRequest, gen.BadRequest, "", err.Error()), nil
	}

	// Guard against misconfigured deployments.
	if h.metricsService == nil {
		h.logger.Error("Metrics service is not initialized")
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1MetricsServiceNotReady,
			"Metrics service is not initialized",
		), nil
	}

	result, err := h.metricsService.QueryRuntimeTopology(ctx, req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			return errorResponse(http.StatusForbidden, gen.Forbidden, "", "Access denied"), nil
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			return errorResponse(http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized"), nil
		}
		errorCode := types.ErrorCodeV1RuntimeTopologyInternalGeneric
		switch {
		case errors.Is(err, service.ErrScopeAuthFailed):
			return errorResponse(
				http.StatusInternalServerError,
				gen.InternalServerError,
				types.ErrorCodeV1ScopeAuthFailed,
				"",
			), nil
		case errors.Is(err, service.ErrRuntimeTopologyInvalidRequest):
			h.logger.Debug("Invalid runtime topology request", "error", err)
			return errorResponse(http.StatusBadRequest, gen.BadRequest, errorCode, err.Error()), nil
		case errors.Is(err, service.ErrRuntimeTopologyResolveSearchScope):
			errorCode = types.ErrorCodeV1RuntimeTopologyResolverFailed
		case errors.Is(err, service.ErrRuntimeTopologyRetrieval):
			errorCode = types.ErrorCodeV1RuntimeTopologyRetrievalFailed
		}
		h.logger.Error("Failed to query runtime topology", "error", err)
		return errorResponse(
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve runtime topology",
		), nil
	}

	return jsonResponse(http.StatusOK, result), nil
}
