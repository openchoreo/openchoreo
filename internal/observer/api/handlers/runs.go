// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// QueryRuns handles POST /api/v1/scheduled-tasks/runs/query
func (h *Handler) QueryRuns(w http.ResponseWriter, r *http.Request) {
	var req types.RunsQueryRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid request format")
		return
	}

	if err := validateRunsQueryRequest(&req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	ctx := r.Context()
	if h.runsService == nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "", "Runs service is not initialized")
		return
	}

	result, err := h.runsService.QueryRuns(ctx, &req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
			return
		}
		if errors.Is(err, service.ErrRunsInvalidRequest) {
			h.logger.Debug("Invalid runs request", "error", err)
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
			return
		}
		h.logger.Error("Failed to query runs", "error", err)
		if errors.Is(err, service.ErrRunsResolveSearchScope) {
			h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, types.ErrorCodeV1LogsResolverFailed, "Failed to resolve search scope")
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "", "Failed to retrieve runs")
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// QueryRetries handles POST /api/v1/scheduled-tasks/runs/{jobName}/retries/query
func (h *Handler) QueryRetries(w http.ResponseWriter, r *http.Request) {
	jobName := r.PathValue("jobName")
	if jobName == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "jobName path parameter is required")
		return
	}

	var req types.RetriesQueryRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid request format")
		return
	}

	if err := validateRetriesQueryRequest(&req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	ctx := r.Context()
	if h.runsService == nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "", "Runs service is not initialized")
		return
	}

	result, err := h.runsService.QueryRetries(ctx, jobName, &req)
	if err != nil {
		if errors.Is(err, observerAuthz.ErrAuthzForbidden) {
			h.writeErrorResponse(w, http.StatusForbidden, gen.Forbidden, "", "Access denied")
			return
		}
		if errors.Is(err, observerAuthz.ErrAuthzUnauthorized) {
			h.writeErrorResponse(w, http.StatusUnauthorized, gen.Unauthorized, "", "Unauthorized")
			return
		}
		if errors.Is(err, service.ErrRunsInvalidRequest) {
			h.logger.Debug("Invalid retries request", "error", err)
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
			return
		}
		h.logger.Error("Failed to query retries", "error", err)
		if errors.Is(err, service.ErrRunsResolveSearchScope) {
			h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, types.ErrorCodeV1LogsResolverFailed, "Failed to resolve search scope")
			return
		}
		h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "", "Failed to retrieve retries")
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// validateRunsQueryRequest validates the runs query request.
func validateRunsQueryRequest(req *types.RunsQueryRequest) error {
	if req.SearchScope == nil {
		return fmt.Errorf("searchScope is required")
	}
	if req.SearchScope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	if req.StartTime == "" {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime == "" {
		return fmt.Errorf("endTime is required")
	}
	if req.SearchScope.Component == "" {
		return fmt.Errorf("searchScope.component is required for run queries")
	}
	if req.SearchScope.Environment == "" {
		return fmt.Errorf("searchScope.environment is required for run queries")
	}
	return nil
}

// validateRetriesQueryRequest validates the retries query request. It mirrors the
// runs validation minus the time-range checks, since retries default to a 30-day
// lookback when no explicit window is provided.
func validateRetriesQueryRequest(req *types.RetriesQueryRequest) error {
	if req.SearchScope == nil {
		return fmt.Errorf("searchScope is required")
	}
	if req.SearchScope.Namespace == "" {
		return fmt.Errorf("searchScope.namespace is required")
	}
	if req.SearchScope.Component == "" {
		return fmt.Errorf("searchScope.component is required for retry queries")
	}
	if req.SearchScope.Environment == "" {
		return fmt.Errorf("searchScope.environment is required for retry queries")
	}
	return nil
}
