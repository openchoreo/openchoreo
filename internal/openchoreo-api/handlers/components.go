// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func (h *Handler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("CreateComponent handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	if orgName == "" || projectName == "" {
		logger.Warn("Organization name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and project name are required", "INVALID_PARAMS")
		return
	}

	// Parse request body
	var req models.CreateComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}
	defer r.Body.Close()

	// Call service to create component
	component, err := h.services.ComponentService.CreateComponent(ctx, orgName, projectName, &req)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentAlreadyExists) {
			logger.Warn("Component already exists", "org", orgName, "project", projectName, "component", req.Name)
			writeErrorResponse(w, http.StatusConflict, "Component already exists", services.CodeComponentExists)
			return
		}
		logger.Error("Failed to create component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Component created successfully", "org", orgName, "project", projectName, "component", component.Name)
	writeSuccessResponse(w, http.StatusCreated, component)
}

func (h *Handler) ListComponents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListComponents handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	if orgName == "" || projectName == "" {
		logger.Warn("Organization name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and project name are required", "INVALID_PARAMS")
		return
	}

	// Call service to list components
	components, err := h.services.ComponentService.ListComponents(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		logger.Error("Failed to list components", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Convert to slice of values for the list response
	componentValues := make([]*models.ComponentResponse, len(components))
	copy(componentValues, components)

	// Success response with pagination info (simplified for now)
	logger.Debug("Listed components successfully", "org", orgName, "project", projectName, "count", len(components))
	writeListResponse(w, componentValues, len(components), 1, len(components))
}

func (h *Handler) GetComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponent handler called")

	// Extract query parameters
	include := r.URL.Query().Get("include")
	additionalResources := []string{}
	if include != "" {
		additionalResources = strings.Split(include, ",")
	}

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if orgName == "" || projectName == "" || componentName == "" {
		logger.Warn("Organization name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	// Call service to get component
	component, err := h.services.ComponentService.GetComponent(ctx, orgName, projectName, componentName, additionalResources)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to get component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved component successfully", "org", orgName, "project", projectName, "component", componentName)
	writeSuccessResponse(w, http.StatusOK, component)
}

func (h *Handler) GetComponentBinding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentBinding handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if orgName == "" || projectName == "" || componentName == "" {
		logger.Warn("Organization name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	// Extract environments from query parameter (supports multiple values, optional)
	environments := r.URL.Query()["environment"]

	// Call service to get component bindings
	bindings, err := h.services.ComponentService.GetComponentBindings(ctx, orgName, projectName, componentName, environments)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to get component bindings", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	envCount := len(environments)
	if envCount == 0 {
		logger.Debug("Retrieved component bindings for all pipeline environments successfully", "org", orgName, "project", projectName, "component", componentName, "count", len(bindings))
	} else {
		logger.Debug("Retrieved component bindings successfully", "org", orgName, "project", projectName, "component", componentName, "environments", environments, "count", len(bindings))
	}
	writeListResponse(w, bindings, len(bindings), 1, len(bindings))
}

func (h *Handler) PromoteComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("PromoteComponent handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if orgName == "" || projectName == "" || componentName == "" {
		logger.Warn("Organization name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	// Parse request body
	var req models.PromoteComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}
	defer r.Body.Close()

	// Sanitize input
	req.Sanitize()

	promoteReq := &services.PromoteComponentPayload{
		PromoteComponentRequest: req,
		ComponentName:           componentName,
		ProjectName:             projectName,
		OrgName:                 orgName,
	}

	// Call service to promote component
	bindings, err := h.services.ComponentService.PromoteComponent(ctx, promoteReq)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrDeploymentPipelineNotFound) {
			logger.Warn("Deployment pipeline not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Deployment pipeline not found", services.CodeDeploymentPipelineNotFound)
			return
		}
		if errors.Is(err, services.ErrInvalidPromotionPath) {
			logger.Warn("Invalid promotion path", "source", req.SourceEnvironment, "target", req.TargetEnvironment)
			writeErrorResponse(w, http.StatusBadRequest, "Invalid promotion path", services.CodeInvalidPromotionPath)
			return
		}
		if errors.Is(err, services.ErrBindingNotFound) {
			logger.Warn("Source binding not found", "org", orgName, "project", projectName, "component", componentName, "environment", req.SourceEnvironment)
			writeErrorResponse(w, http.StatusNotFound, "Source binding not found", services.CodeBindingNotFound)
			return
		}
		logger.Error("Failed to promote component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Component promoted successfully", "org", orgName, "project", projectName, "component", componentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment, "bindingsCount", len(bindings))
	writeListResponse(w, bindings, len(bindings), 1, len(bindings))
}
