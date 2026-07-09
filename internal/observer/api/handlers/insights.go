// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// insightsMaxQueryTimeRange caps DORA queries. Deliberately much larger than the 30-day
// raw-event cap: metric rollups are durable and outlive raw-event retention.
const insightsMaxQueryTimeRange = 400 * 24 * time.Hour

// QueryDoraMetrics handles POST /api/v1alpha1/insights/dora/query
func (h *Handler) QueryDoraMetrics(w http.ResponseWriter, r *http.Request) {
	var req gen.DoraMetricsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest,
			"INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if err := ValidateDoraMetricsQueryRequest(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if h.insightsService == nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError,
			"SERVICE_NOT_READY", "insights querier is not initialized")
		return
	}

	resp, err := h.insightsService.QueryDoraMetrics(r.Context(), req)
	if err != nil {
		h.writeInsightsError(w, err, "QUERY_DORA_METRICS_FAILED", "failed to query DORA metrics")
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// QueryDoraDeployments handles POST /api/v1alpha1/insights/dora/deployments/query
func (h *Handler) QueryDoraDeployments(w http.ResponseWriter, r *http.Request) {
	var req gen.DoraDeploymentsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest,
			"INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
		return
	}

	if err := ValidateDoraDeploymentsQueryRequest(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if h.insightsService == nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError,
			"SERVICE_NOT_READY", "insights querier is not initialized")
		return
	}

	resp, err := h.insightsService.QueryDoraDeployments(r.Context(), req)
	if err != nil {
		h.writeInsightsError(w, err, "QUERY_DORA_DEPLOYMENTS_FAILED", "failed to query deployments")
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// writeInsightsError maps service-layer errors to HTTP responses, mirroring the
// alerts/incidents handlers.
func (h *Handler) writeInsightsError(w http.ResponseWriter, err error, errorCode, message string) {
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
	case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
		errors.Is(err, observerAuthz.ErrAuthzTimeout):
		h.writeErrorResponse(w, http.StatusServiceUnavailable, gen.InternalServerError,
			"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")
	case errors.Is(err, service.ErrScopeNotFound):
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest,
			"SCOPE_NOT_FOUND", "one or more resources in the search scope were not found")
	case errors.Is(err, service.ErrScopeResolutionFailed):
		h.logger.Error("Failed to resolve insights search scope", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError,
			"RESOLVE_SCOPE_FAILED", "failed to resolve search scope")
	default:
		h.logger.Error("Insights query failed", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, errorCode, message)
	}
}

// ValidateDoraMetricsQueryRequest validates the DoraMetricsQueryRequest.
func ValidateDoraMetricsQueryRequest(req *gen.DoraMetricsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if err := validateDoraScopeAndWindow(&req.SearchScope, req.StartTime, req.EndTime); err != nil {
		return err
	}
	if req.Granularity != nil {
		granularity := string(*req.Granularity)
		valid := []string{"daily", "weekly", "monthly"}
		if granularity != "" && !slices.Contains(valid, granularity) {
			return fmt.Errorf("granularity must be one of: %s", strings.Join(valid, ", "))
		}
	}
	if req.Metrics != nil {
		valid := []string{"deploymentFrequency", "leadTime", "changeFailureRate", "mttr"}
		for _, m := range *req.Metrics {
			if !slices.Contains(valid, string(m)) {
				return fmt.Errorf("metrics must be a subset of: %s", strings.Join(valid, ", "))
			}
		}
	}
	return nil
}

// ValidateDoraDeploymentsQueryRequest validates the DoraDeploymentsQueryRequest.
func ValidateDoraDeploymentsQueryRequest(req *gen.DoraDeploymentsQueryRequest) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if err := validateDoraScopeAndWindow(&req.SearchScope, req.StartTime, req.EndTime); err != nil {
		return err
	}
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return fmt.Errorf("limit must be a positive integer greater than zero")
		}
		if *req.Limit > config.MaxLimit {
			return fmt.Errorf("limit cannot exceed %d", config.MaxLimit)
		}
	}
	if req.SortOrder != nil {
		order := string(*req.SortOrder)
		if order != sortOrderAsc && order != defaultSortOrder {
			return fmt.Errorf("sortOrder must be either 'asc' or 'desc'")
		}
	}
	return nil
}

func validateDoraScopeAndWindow(scope *gen.ComponentSearchScope, startTime, endTime time.Time) error {
	if startTime.IsZero() {
		return fmt.Errorf("startTime is required")
	}
	if endTime.IsZero() {
		return fmt.Errorf("endTime is required")
	}
	if !endTime.After(startTime) {
		return fmt.Errorf("endTime must be after startTime")
	}
	if endTime.Sub(startTime) > insightsMaxQueryTimeRange {
		return fmt.Errorf("query time range cannot exceed %d days",
			int(insightsMaxQueryTimeRange/(24*time.Hour)))
	}

	scope.Namespace = strings.TrimSpace(scope.Namespace)
	if scope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	if scope.Component != nil && strings.TrimSpace(*scope.Component) != "" &&
		(scope.Project == nil || strings.TrimSpace(*scope.Project) == "") {
		return fmt.Errorf("searchScope.project is required when searchScope.component is provided")
	}
	return nil
}
