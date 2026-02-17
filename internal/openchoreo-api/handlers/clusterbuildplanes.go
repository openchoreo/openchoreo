// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
)

// ListClusterBuildPlanes handles GET /api/v1/clusterbuildplanes
func (h *Handler) ListClusterBuildPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	buildPlanes, err := h.services.ClusterBuildPlaneService.ListClusterBuildPlanes(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster build planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list cluster build planes", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, buildPlanes)
}
