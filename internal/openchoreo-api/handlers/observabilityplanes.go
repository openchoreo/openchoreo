// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
)

// ListObservabilityPlanes retrieves all observability planes for an organization
func (h *Handler) ListObservabilityPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("ListObservabilityPlanes handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		log.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", "INVALID_ORG_NAME")
		return
	}

	// Call service to list observability planes
	observabilityPlanes, err := h.services.ObservabilityPlaneService.ListObservabilityPlanes(ctx, orgName)
	if err != nil {
		log.Error("Failed to list observability planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list observability planes", "INTERNAL_ERROR")
		return
	}

	// Success response with observability planes list
	writeSuccessResponse(w, http.StatusOK, observabilityPlanes)
}
