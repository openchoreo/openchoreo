// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) GetWorkflowPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("GetWorkflowPlane handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
		return
	}

	// Call service to get workflow plane
	workflowPlane, err := h.services.WorkflowPlaneService.GetWorkflowPlane(ctx, namespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to view workflow plane", "namespace", namespaceName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		log.Error("Failed to get workflow plane", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get workflow plane", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusOK, workflowPlane)
}

// ListWorkflowPlanes retrieves all workflow planes for an namespace
func (h *Handler) ListWorkflowPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("ListWorkflowPlanes handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
		return
	}

	// Call service to list workflow planes
	workflowPlanes, err := h.services.WorkflowPlaneService.ListWorkflowPlanes(ctx, namespaceName)
	if err != nil {
		log.Error("Failed to list workflow planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list workflow planes", "INTERNAL_ERROR")
		return
	}

	// Success response with workflow planes list
	writeSuccessResponse(w, http.StatusOK, workflowPlanes)
}
