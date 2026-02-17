// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) GetBuildPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("GetBuildPlane handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
		return
	}

	// Call service to get build plane
	buildPlane, err := h.services.BuildPlaneService.GetBuildPlane(ctx, namespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to view build plane", "namespace", namespaceName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		log.Error("Failed to get build plane", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get build plane", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusOK, buildPlane)
}

// ListBuildPlanes retrieves all build planes for an namespace
func (h *Handler) ListBuildPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("ListBuildPlanes handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
		return
	}

	// Call service to list build planes
	buildPlanes, err := h.services.BuildPlaneService.ListBuildPlanes(ctx, namespaceName)
	if err != nil {
		log.Error("Failed to list build planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list build planes", "INTERNAL_ERROR")
		return
	}

	// Success response with build planes list
	writeSuccessResponse(w, http.StatusOK, buildPlanes)
}
