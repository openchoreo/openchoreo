// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"net/http"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
)

func (h *Handler) GetWorkloads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("GetWorkloads handler called")

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

	// Call service to get workloads
	workloads, err := h.services.ComponentService.GetComponentWorkloads(ctx, orgName, projectName, componentName)
	if err != nil {
		log.Error("Failed to get workloads", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get workloads", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusOK, workloads)
}

func (h *Handler) CreateWorkload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("CreateWorkload handler called")

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

	// Parse request body
	var workloadSpec openchoreov1alpha1.WorkloadSpec
	if err := json.NewDecoder(r.Body).Decode(&workloadSpec); err != nil {
		log.Warn("Invalid request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_REQUEST_BODY")
		return
	}

	// Call service to create/update workload
	createdWorkload, err := h.services.ComponentService.CreateComponentWorkload(ctx, orgName, projectName, componentName, &workloadSpec)
	if err != nil {
		log.Error("Failed to create workload", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create workload", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusCreated, createdWorkload)
}
