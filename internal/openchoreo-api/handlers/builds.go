package handlers

import (
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"net/http"
)

func (h *Handler) ListBuildTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("ListBuildTemplates handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		log.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", "INVALID_ORG_NAME")
		return
	}

	// Call service to list build templates
	templates, err := h.services.BuildService.ListBuildTemplates(ctx, orgName)
	if err != nil {
		log.Error("Failed to list build templates", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list build templates", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusOK, templates)
}

func (h *Handler) TriggerBuild(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("TriggerBuild handler called")

	// Extract parameters from URL path
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	commit := r.URL.Query().Get("commit")

	if orgName == "" {
		log.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", "INVALID_ORG_NAME")
		return
	}

	if projectName == "" {
		log.Warn("Project name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Project name is required", "INVALID_PROJECT_NAME")
		return
	}

	if componentName == "" {
		log.Warn("Component name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Component name is required", "INVALID_COMPONENT_NAME")
		return
	}

	build, err := h.services.BuildService.TriggerBuild(ctx, orgName, projectName, componentName, commit)
	if err != nil {
		log.Error("Failed to trigger build", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to trigger build", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusCreated, build)
}

func (h *Handler) ListBuilds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("ListBuilds handler called")

	// Extract parameters from URL path
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	if orgName == "" {
		log.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", "INVALID_ORG_NAME")
		return
	}

	if projectName == "" {
		log.Warn("Project name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Project name is required", "INVALID_PROJECT_NAME")
		return
	}

	if componentName == "" {
		log.Warn("Component name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Component name is required", "INVALID_COMPONENT_NAME")
		return
	}

	// Call service to list builds
	builds, err := h.services.BuildService.ListBuilds(ctx, orgName, projectName, componentName)
	if err != nil {
		log.Error("Failed to list builds", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list builds", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusOK, builds)
}
