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

// QueryLogs handles POST /api/v1/logs/query
func (h *Handler) QueryLogs(w http.ResponseWriter, r *http.Request) {
	var req types.LogsQueryRequest
	if err := httputil.BindJSON(r, &req); err != nil {
		h.logger.Error("Failed to bind request", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid request format")
		return
	}

	// Validate request
	if err := ValidateLogsQueryRequest(&req); err != nil {
		h.logger.Debug("Validation failed", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", err.Error())
		return
	}

	// Determine authorization context based on search scope
	var resourceType observerAuthz.ResourceType
	var resourceName string
	var hierarchy authzcore.ResourceHierarchy

	if req.SearchScope.Component != nil {
		scope := req.SearchScope.Component
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
	} else if req.SearchScope.Workflow != nil {
		scope := req.SearchScope.Workflow
		if scope.WorkflowRunName != "" {
			resourceType = observerAuthz.ResourceTypeWorkflowRun
			resourceName = scope.WorkflowRunName
			hierarchy = authzcore.ResourceHierarchy{
				Namespace: scope.Namespace,
			}
		} else {
			resourceType = observerAuthz.ResourceTypeNamespace
			resourceName = scope.Namespace
			hierarchy = authzcore.ResourceHierarchy{
				Namespace: scope.Namespace,
			}
		}
	} else {
		h.logger.Error("Invalid search scope", "searchScope", req.SearchScope)
		h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "", "Invalid search scope")
		return
	}

	// AUTHORIZATION CHECK
	if err := observerAuthz.CheckAuthorization(
		r.Context(),
		h.logger,
		h.authzPDP,
		observerAuthz.ActionViewLogs,
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
			types.ErrorCodeV1LogsAuthzInternal,
			"Authorization check failed",
		)
		return
	}

	ctx := r.Context()
	if h.logsService == nil {
		h.logger.Error("Logs service is not initialized")
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			types.ErrorCodeV1LogsServiceNotReady,
			"Logs service is not initialized",
		)
		return
	}
	result, err := h.logsService.QueryLogs(ctx, &req)
	if err != nil {
		h.logger.Error("Failed to query logs", "error", err)
		errorCode := types.ErrorCodeV1LogsInternalGeneric
		switch {
		case errors.Is(err, service.ErrLogsResolveSearchScope):
			errorCode = types.ErrorCodeV1LogsResolverFailed
		case errors.Is(err, service.ErrLogsRetrieval):
			errorCode = types.ErrorCodeV1LogsRetrievalFailed
		}
		h.writeErrorResponse(
			w,
			http.StatusInternalServerError,
			gen.InternalServerError,
			errorCode,
			"Failed to retrieve logs",
		)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}
