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
