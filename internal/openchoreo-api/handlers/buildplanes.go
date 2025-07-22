package handlers

import (
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"net/http"
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

	// Call service to list build planes
	buildPlanes, err := h.services.BuildPlaneService.ListBuildPlanes(ctx, orgName)
	if err != nil {
		log.Error("Failed to list build planes", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list build planes", "INTERNAL_ERROR")
		return
	}

	// Success response with build planes list
	writeSuccessResponse(w, http.StatusOK, buildPlanes)
}
