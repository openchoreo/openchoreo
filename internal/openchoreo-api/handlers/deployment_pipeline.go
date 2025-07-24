// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func (h *Handler) GetProjectDeploymentPipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetProjectDeploymentPipeline handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	if orgName == "" || projectName == "" {
		logger.Warn("Organization name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and project name are required", "INVALID_PARAMS")
		return
	}

	// Call service to get project deployment pipeline
	pipeline, err := h.services.DeploymentPipelineService.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrDeploymentPipelineNotFound) {
			logger.Warn("Deployment pipeline not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Deployment pipeline not found", services.CodeDeploymentPipelineNotFound)
			return
		}
		logger.Error("Failed to get project deployment pipeline", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved project deployment pipeline successfully", "org", orgName, "project", projectName, "pipeline", pipeline.Name)
	writeSuccessResponse(w, http.StatusOK, pipeline)
}