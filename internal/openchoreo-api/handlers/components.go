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
		logger.Warn("Organization and project names are required")
		writeErrorResponse(w, http.StatusBadRequest,
			"Organization and project names are required", services.CodeInvalidInput)
		return
	}

	// Check for cursor-based pagination
	cursor, limit, useCursor, err := parseCursorParams(r)
	if err != nil {
		errorCode := services.GetPaginationErrorCode(err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), errorCode)
		return
	}

	if useCursor {
		// only validate non-empty cursors
		if cursor != "" {
			if err := validateCursorWithContext(cursor); err != nil {
				logger.Warn("Invalid cursor", "error", err, "ip", r.RemoteAddr)
				writeErrorResponse(w, http.StatusBadRequest,
					"Invalid cursor parameter", services.CodeInvalidCursorFormat)
				return
			}
		}

		components, nextCursor, err := h.services.ComponentService.ListComponentsWithCursor(
			ctx, orgName, projectName, cursor, limit)
		if err != nil {
			if errors.Is(err, services.ErrProjectNotFound) {
				writeErrorResponse(w, http.StatusNotFound,
					"Project not found", services.CodeProjectNotFound)
				return
			}
			if errors.Is(err, services.ErrContinueTokenExpired) {
				writeTokenExpiredError(w)
				return
			}
			if errors.Is(err, services.ErrInvalidCursorFormat) {
				logger.Error("Invalid cursor format", "error", err, "ip", r.RemoteAddr)
				writeErrorResponse(w, http.StatusBadRequest,
					"Invalid cursor format", services.CodeInvalidCursorFormat)
				return
			}
			if strings.Contains(err.Error(), "service unavailable") {
				writeErrorResponse(w, http.StatusServiceUnavailable,
					"Service temporarily unavailable", services.CodeInternalError)
				return
			}
			logger.Error("Failed to list components with cursor", "error", err)
			writeErrorResponse(w, http.StatusInternalServerError,
				"Internal server error", services.CodeInternalError)
			return
		}

		writeCursorListResponse(w, components, nextCursor)
		return
	}

	// Legacy mode
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

func (h *Handler) UpdateComponentBinding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("UpdateComponentBinding handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	bindingName := r.PathValue("bindingName")
	if orgName == "" || projectName == "" || componentName == "" || bindingName == "" {
		logger.Warn("Organization name, project name, component name, and binding name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name, project name, component name, and binding name are required", "INVALID_PARAMS")
		return
	}

	// Parse request body
	var req models.UpdateBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}
	defer r.Body.Close()

	// Validate the request
	if err := req.Validate(); err != nil {
		logger.Warn("Invalid request", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		return
	}

	// Call service to update component binding
	binding, err := h.services.ComponentService.UpdateComponentBinding(ctx, orgName, projectName, componentName, bindingName, &req)
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
		if errors.Is(err, services.ErrBindingNotFound) {
			logger.Warn("Binding not found", "org", orgName, "project", projectName, "component", componentName, "binding", bindingName)
			writeErrorResponse(w, http.StatusNotFound, "Binding not found", services.CodeBindingNotFound)
			return
		}
		logger.Error("Failed to update component binding", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Component binding updated successfully", "org", orgName, "project", projectName, "component", componentName, "binding", bindingName)
	writeSuccessResponse(w, http.StatusOK, binding)
}

func (h *Handler) GetComponentObserverURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentObserverURL handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	environmentName := r.PathValue("environmentName")

	if orgName == "" || projectName == "" || componentName == "" || environmentName == "" {
		logger.Warn("All path parameters are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization, project, component, and environment names are required", "INVALID_PARAMS")
		return
	}

	// Call service to get observer URL
	observerResponse, err := h.services.ComponentService.GetComponentObserverURL(ctx, orgName, projectName, componentName, environmentName)
	if err != nil {
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Error in retrieving the log URL: Component not found", "org", orgName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Error in retrieving the log URL: Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrEnvironmentNotFound) {
			logger.Warn("Error in retrieving the log URL: Environment not found", "org", orgName, "environment", environmentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Environment not found", services.CodeEnvironmentNotFound)
			return
		}
		if errors.Is(err, services.ErrDataPlaneNotFound) {
			logger.Warn("Error in retrieving the log URL: DataPlane not found", "org", orgName, "environment", environmentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: DataPlane not found", services.CodeDataPlaneNotFound)
			return
		}
		logger.Error("Error in retrieving the log URL: Failed to get component observer URL", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Error in retrieving the log URL: Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved component observer URL successfully", "org", orgName, "project", projectName, "component", componentName, "environment", environmentName)
	writeSuccessResponse(w, http.StatusOK, observerResponse)
}

func (h *Handler) GetBuildObserverURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetBuildObserverURL handler called")

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	if orgName == "" || projectName == "" || componentName == "" {
		logger.Warn("All parameters are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization, project, and component names are required", "INVALID_PARAMS")
		return
	}

	// Call service to get build observer URL
	observerResponse, err := h.services.ComponentService.GetBuildObserverURL(ctx, orgName, projectName, componentName)
	if err != nil {
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Error in retrieving the log URL: Component not found", "org", orgName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Error in retrieving the log URL: Project not found", "org", orgName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Project not found", services.CodeProjectNotFound)
			return
		}
		logger.Error("Error in retrieving the log URL: Failed to get build observer URL", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Error in retrieving the log URL: Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved build observer URL successfully", "org", orgName, "project", projectName, "component", componentName)
	writeSuccessResponse(w, http.StatusOK, observerResponse)
}
