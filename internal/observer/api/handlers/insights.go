// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
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

// errNilInsightsResponse marks the case where the insights service returns
// neither a response nor an error, which would otherwise be a nil dereference.
var errNilInsightsResponse = errors.New("insights service returned no response and no error")

// QueryDoraMetrics handles POST /api/v1alpha1/insights/dora/query
//
// Returns the generated typed responses rather than the apiResponse passthrough:
// the service already speaks gen.* and every status this handler produces is
// declared in the spec, so the compiler checks both the body shape and the
// status against the contract.
func (h *Handler) QueryDoraMetrics(
	ctx context.Context,
	request gen.QueryDoraMetricsRequestObject,
) (gen.QueryDoraMetricsResponseObject, error) {
	if request.Body == nil {
		return gen.QueryDoraMetrics400JSONResponse(errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required")), nil
	}
	req := *request.Body

	if err := ValidateDoraMetricsQueryRequest(&req); err != nil {
		return gen.QueryDoraMetrics400JSONResponse(errorPayload(gen.BadRequest,
			"VALIDATION_ERROR", err.Error())), nil
	}

	if h.insightsService == nil {
		return gen.QueryDoraMetrics500JSONResponse(errorPayload(gen.InternalServerError,
			"SERVICE_NOT_READY", "insights querier is not initialized")), nil
	}

	resp, err := h.insightsService.QueryDoraMetrics(ctx, req)
	if err == nil && resp == nil {
		err = errNilInsightsResponse
	}
	if err != nil {
		status, payload := h.insightsErrorPayload(err, "QUERY_DORA_METRICS_FAILED", "failed to query DORA metrics")
		switch status {
		case http.StatusBadRequest:
			return gen.QueryDoraMetrics400JSONResponse(payload), nil
		case http.StatusUnauthorized:
			return gen.QueryDoraMetrics401JSONResponse(payload), nil
		case http.StatusForbidden:
			return gen.QueryDoraMetrics403JSONResponse(payload), nil
		case http.StatusServiceUnavailable:
			return gen.QueryDoraMetrics503JSONResponse(payload), nil
		default:
			return gen.QueryDoraMetrics500JSONResponse(payload), nil
		}
	}

	return gen.QueryDoraMetrics200JSONResponse(*resp), nil
}

// QueryDoraDeployments handles POST /api/v1alpha1/insights/dora/deployments/query
func (h *Handler) QueryDoraDeployments(
	ctx context.Context,
	request gen.QueryDoraDeploymentsRequestObject,
) (gen.QueryDoraDeploymentsResponseObject, error) {
	if request.Body == nil {
		return gen.QueryDoraDeployments400JSONResponse(errorPayload(gen.BadRequest, "INVALID_REQUEST_BODY",
			"invalid request body: request body is required")), nil
	}
	req := *request.Body

	if err := ValidateDoraDeploymentsQueryRequest(&req); err != nil {
		return gen.QueryDoraDeployments400JSONResponse(errorPayload(gen.BadRequest,
			"VALIDATION_ERROR", err.Error())), nil
	}

	if h.insightsService == nil {
		return gen.QueryDoraDeployments500JSONResponse(errorPayload(gen.InternalServerError,
			"SERVICE_NOT_READY", "insights querier is not initialized")), nil
	}

	resp, err := h.insightsService.QueryDoraDeployments(ctx, req)
	if err == nil && resp == nil {
		err = errNilInsightsResponse
	}
	if err != nil {
		status, payload := h.insightsErrorPayload(err, "QUERY_DORA_DEPLOYMENTS_FAILED", "failed to query deployments")
		switch status {
		case http.StatusBadRequest:
			return gen.QueryDoraDeployments400JSONResponse(payload), nil
		case http.StatusUnauthorized:
			return gen.QueryDoraDeployments401JSONResponse(payload), nil
		case http.StatusForbidden:
			return gen.QueryDoraDeployments403JSONResponse(payload), nil
		case http.StatusServiceUnavailable:
			return gen.QueryDoraDeployments503JSONResponse(payload), nil
		default:
			return gen.QueryDoraDeployments500JSONResponse(payload), nil
		}
	}

	return gen.QueryDoraDeployments200JSONResponse(*resp), nil
}

// insightsErrorPayload maps a service-layer error to the status and body the
// caller should see. The mapping is shared by both insights operations, which
// declare the same statuses; each caller turns the status into its own generated
// response type, so the status stays checked against the spec.
func (h *Handler) insightsErrorPayload(err error, errorCode, message string) (int, gen.ErrorResponse) {
	switch {
	case errors.Is(err, observerAuthz.ErrAuthzForbidden):
		return http.StatusForbidden, errorPayload(gen.Forbidden, "", "Access denied")
	case errors.Is(err, observerAuthz.ErrAuthzUnauthorized):
		return http.StatusUnauthorized, errorPayload(gen.Unauthorized, "", "Unauthorized")
	case errors.Is(err, observerAuthz.ErrAuthzServiceUnavailable),
		errors.Is(err, observerAuthz.ErrAuthzTimeout):
		return http.StatusServiceUnavailable, errorPayload(gen.InternalServerError,
			"AUTHZ_UNAVAILABLE", "authorization service temporarily unavailable")
	case errors.Is(err, service.ErrScopeNotFound):
		return http.StatusBadRequest, errorPayload(gen.BadRequest,
			"SCOPE_NOT_FOUND", "one or more resources in the search scope were not found")
	case errors.Is(err, service.ErrScopeResolutionFailed):
		h.logger.Error("Failed to resolve insights search scope", "error", err)
		return http.StatusInternalServerError, errorPayload(gen.InternalServerError,
			"RESOLVE_SCOPE_FAILED", "failed to resolve search scope")
	default:
		h.logger.Error("Insights query failed", "error", err)
		return http.StatusInternalServerError, errorPayload(gen.InternalServerError, errorCode, message)
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
