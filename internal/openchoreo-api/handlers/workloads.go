// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) GetWorkloads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract parameters from URL path
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
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
	workloads, err := h.services.ComponentService.GetComponentWorkloads(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to view workloads", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			log.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			log.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		log.Error("Failed to get workloads", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get workloads", services.CodeInternalError)
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusOK, workloads)
}

func (h *Handler) CreateWorkload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract parameters from URL path
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", "INVALID_NAMESPACE_NAME")
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

	setAuditResource(ctx, "workload", "", "")
	addAuditMetadataBatch(ctx, map[string]any{
		"namespace": namespaceName,
		"project":   projectName,
		"component": componentName,
	})

	// Call service to create/update workload
	createdWorkload, err := h.services.ComponentService.CreateComponentWorkload(ctx, namespaceName, projectName, componentName, &workloadSpec)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to create workload", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			log.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			log.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		log.Error("Failed to create workload", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create workload", services.CodeInternalError)
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusCreated, createdWorkload)
}
