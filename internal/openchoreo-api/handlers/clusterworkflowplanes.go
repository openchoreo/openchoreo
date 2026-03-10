// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
)

// ListClusterWorkflowPlanes handles GET /api/v1/clusterworkflowplanes
func (h *Handler) ListClusterWorkflowPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workflowPlanes, err := h.services.ClusterWorkflowPlaneService.ListClusterWorkflowPlanes(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster workflow planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list cluster workflow planes", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, workflowPlanes)
}
