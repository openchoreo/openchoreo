// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
)

// ListClusterObservabilityPlanes handles GET /api/v1/clusterobservabilityplanes
func (h *Handler) ListClusterObservabilityPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	observabilityPlanes, err := h.services.ClusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster observability planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list cluster observability planes", services.CodeInternalError)
		return
	}

	writeListResponse(w, observabilityPlanes, len(observabilityPlanes), 1, len(observabilityPlanes))
}
