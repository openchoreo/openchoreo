// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) GetProjectDeploymentPipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetProjectDeploymentPipeline handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	if namespaceName == "" || projectName == "" {
		logger.Warn("Namespace name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and project name are required", "INVALID_PARAMS")
		return
	}

	// Call service to get project deployment pipeline
	pipeline, err := h.services.DeploymentPipelineService.GetProjectDeploymentPipeline(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view deployment pipeline", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrDeploymentPipelineNotFound) {
			logger.Warn("Deployment pipeline not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Deployment pipeline not found", services.CodeDeploymentPipelineNotFound)
			return
		}
		logger.Error("Failed to get project deployment pipeline", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved project deployment pipeline successfully", "namespace", namespaceName, "project", projectName, "pipeline", pipeline.Name)
	writeSuccessResponse(w, http.StatusOK, pipeline)
}
