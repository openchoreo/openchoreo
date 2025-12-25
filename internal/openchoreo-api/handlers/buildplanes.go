// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// There is only one buildplane per org

func (h *Handler) GetBuildPlane(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("GetBuildPlane handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		log.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", "INVALID_ORG_NAME")
		return
	}

	// Call service to get build plane
	buildPlane, err := h.services.BuildPlaneService.GetBuildPlane(ctx, orgName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to view build plane", "org", orgName)
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

// ListBuildPlanes retrieves all build planes for an organization
func (h *Handler) ListBuildPlanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("ListBuildPlanes handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		log.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", "INVALID_ORG_NAME")
		return
	}

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		log.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	// Call service to list build planes
	result, err := h.services.BuildPlaneService.ListBuildPlanes(ctx, orgName, opts)
	if err != nil {
		if errors.Is(err, services.ErrContinueTokenExpired) {
			log.Warn("Continue token expired")
			writeErrorResponse(w, http.StatusGone,
				"Continue token has expired. Please restart listing from the beginning.",
				services.CodeContinueTokenExpired)
			return
		}
		if errors.Is(err, services.ErrInvalidContinueToken) {
			log.Warn("Invalid continue token provided")
			writeErrorResponse(w, http.StatusBadRequest,
				"Invalid continue token provided",
				services.CodeInvalidContinueToken)
			return
		}
		log.Error("Failed to list build planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list build planes", "INTERNAL_ERROR")
		return
	}

	// Success response with build planes list
	log.Debug("Listed build planes successfully", "count", len(result.Items), "hasMore", result.Metadata.HasMore)
	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}
