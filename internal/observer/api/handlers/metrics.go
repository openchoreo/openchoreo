// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryMetrics handles POST /api/v1/metrics/query
func (h *Handler) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	var req types.MetricsQueryRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid request format")
		return
	}

	// Validate request
	if err := ValidateMetricsQueryRequest(&req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	// Determine authorization context from search scope
	scope := req.SearchScope
	var resourceType observerAuthz.ResourceType
	var resourceName string
	var hierarchy authzcore.ResourceHierarchy

	if scope.Component != "" {
		resourceType = observerAuthz.ResourceTypeComponent
		resourceName = scope.Component
		hierarchy = authzcore.ResourceHierarchy{
			Namespace: scope.Namespace,
			Project:   scope.Project,
			Component: scope.Component,
		}
	} else if scope.Project != "" {
		resourceType = observerAuthz.ResourceTypeProject
		resourceName = scope.Project
		hierarchy = authzcore.ResourceHierarchy{
			Namespace: scope.Namespace,
			Project:   scope.Project,
		}
	} else {
		resourceType = observerAuthz.ResourceTypeNamespace
		resourceName = scope.Namespace
		hierarchy = authzcore.ResourceHierarchy{
			Namespace: scope.Namespace,
		}
	}

	// AUTHORIZATION CHECK
	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewMetrics,
		resourceType,
		resourceName,
		hierarchy,
	); err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
			return
		}
		h.logger.Error("Authorization check failed", "error", err)
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1MetricsAuthzInternal,
			"Authorization check failed",
		)
		return
	}

	ctx := r.Context()
	// Guard against misconfigured deployments after authz.
	if h.metricsService == nil {
		h.logger.Error("Metrics service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1MetricsServiceNotReady,
			"Metrics service is not initialized",
		)
		return
	}
	result, err := h.metricsService.QueryMetrics(ctx, &req)
	if err != nil {
		errorCode := types.ErrorCodeV1MetricsInternalGeneric
		switch {
		case errors.Is(err, service.ErrMetricsInvalidRequest):
			h.logger.Debug("Invalid metrics request", "error", err)
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, errorCode, err.Error())
			return
		case errors.Is(err, service.ErrMetricsResolveSearchScope):
			errorCode = types.ErrorCodeV1MetricsResolverFailed
		case errors.Is(err, service.ErrMetricsRetrieval):
			errorCode = types.ErrorCodeV1MetricsRetrievalFailed
		}
		h.logger.Error("Failed to query metrics", "error", err)
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve metrics",
		)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}
