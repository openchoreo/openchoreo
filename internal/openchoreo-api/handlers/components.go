// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("CreateComponent handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	if namespaceName == "" || projectName == "" {
		logger.Warn("Namespace name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and project name are required", "INVALID_PARAMS")
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

	setAuditResource(ctx, "component", req.Name, req.Name)
	addAuditMetadataBatch(ctx, map[string]any{
		"organization": namespaceName,
		"project":      projectName,
	})

	// Call service to create component
	component, err := h.services.ComponentService.CreateComponent(ctx, namespaceName, projectName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to create component", "namespace", namespaceName, "project", projectName, "component", req.Name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentAlreadyExists) {
			logger.Warn("Component already exists", "namespace", namespaceName, "project", projectName, "component", req.Name)
			writeErrorResponse(w, http.StatusConflict, "Component already exists", services.CodeComponentExists)
			return
		}
		logger.Error("Failed to create component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Component created successfully", "namespace", namespaceName, "project", projectName, "component", component.Name)
	writeSuccessResponse(w, http.StatusCreated, component)
}

func (h *Handler) ListComponents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListComponents handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	if namespaceName == "" || projectName == "" {
		logger.Warn("Namespace name and project name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and project name are required", "INVALID_PARAMS")
		return
	}

	// Call service to list components
	components, err := h.services.ComponentService.ListComponents(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
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
	logger.Debug("Listed components successfully", "namespace", namespaceName, "project", projectName, "count", len(components))
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
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	// Call service to get component
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, projectName, componentName, additionalResources)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view component", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to get component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved component successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	writeSuccessResponse(w, http.StatusOK, component)
}

func (h *Handler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Info("DeleteComponent handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	// Call service to delete component
	err := h.services.ComponentService.DeleteComponent(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to delete component", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to delete component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response - 204 No Content for successful delete
	logger.Info("Component deleted successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PatchComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("PatchComponent handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", services.CodeInvalidParams)
		return
	}

	var req models.PatchComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	setAuditResource(ctx, "component", componentName, componentName)
	addAuditMetadataBatch(ctx, map[string]any{
		"namespace": namespaceName,
		"project":   projectName,
	})

	component, err := h.services.ComponentService.PatchComponent(ctx, namespaceName, projectName, componentName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to update component", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to patch component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Patched component successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	writeSuccessResponse(w, http.StatusOK, component)
}

func (h *Handler) GetComponentSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentSchema handler called")

	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", services.CodeInvalidInput)
		return
	}

	schema, err := h.services.ComponentService.GetComponentSchema(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentTypeNotFound) {
			logger.Warn("ComponentType not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentType not found", services.CodeComponentTypeNotFound)
			return
		}
		logger.Error("Failed to get component schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved component schema successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	writeSuccessResponse(w, http.StatusOK, schema)
}

func (h *Handler) UpdateComponentWorkflowParameters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("UpdateComponentWorkflowParameters handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	// Parse request body
	var req models.UpdateComponentWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}

	setAuditResource(ctx, "component", componentName, componentName)
	addAuditMetadataBatch(ctx, map[string]any{
		"namespace": namespaceName,
		"project":   projectName,
	})

	// Call service to update workflow parameters
	component, err := h.services.ComponentService.UpdateComponentWorkflowParameters(ctx, namespaceName, projectName, componentName, &req)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrWorkflowSchemaInvalid) {
			logger.Warn("Invalid workflow parameters", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusBadRequest, "Invalid workflow parameters", services.CodeWorkflowSchemaInvalid)
			return
		}
		logger.Error("Failed to update component workflow parameters", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, err.Error(), services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, component)
}

func (h *Handler) GetComponentBinding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentBinding handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	// Extract environments from query parameter (supports multiple values, optional)
	environments := r.URL.Query()["environment"]

	// Call service to get component bindings
	bindings, err := h.services.ComponentService.GetComponentBindings(ctx, namespaceName, projectName, componentName, environments)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
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
		logger.Debug("Retrieved component bindings for all pipeline environments successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(bindings))
	} else {
		logger.Debug("Retrieved component bindings successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "environments", environments, "count", len(bindings))
	}
	writeListResponse(w, bindings, len(bindings), 1, len(bindings))
}

func (h *Handler) PromoteComponent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("PromoteComponent handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
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

	setAuditResource(ctx, "component", componentName, componentName)
	addAuditMetadataBatch(ctx, map[string]any{
		"organization":       namespaceName,
		"project":            projectName,
		"source_environment": req.SourceEnvironment,
		"target_environment": req.TargetEnvironment,
	})

	promoteReq := &services.PromoteComponentPayload{
		PromoteComponentRequest: req,
		ComponentName:           componentName,
		ProjectName:             projectName,
		NamespaceName:           namespaceName,
	}

	targetReleaseBinding, err := h.services.ComponentService.PromoteComponent(ctx, promoteReq)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to promote component", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrDeploymentPipelineNotFound) {
			logger.Warn("Deployment pipeline not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Deployment pipeline not found", services.CodeDeploymentPipelineNotFound)
			return
		}
		if errors.Is(err, services.ErrInvalidPromotionPath) {
			logger.Warn("Invalid promotion path", "source", req.SourceEnvironment, "target", req.TargetEnvironment)
			writeErrorResponse(w, http.StatusBadRequest, "Invalid promotion path", services.CodeInvalidPromotionPath)
			return
		}
		if errors.Is(err, services.ErrReleaseBindingNotFound) {
			logger.Warn("Source release binding not found", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", req.SourceEnvironment)
			writeErrorResponse(w, http.StatusNotFound, "Source release binding not found", services.CodeReleaseBindingNotFound)
			return
		}
		logger.Error("Failed to promote component", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Component promoted successfully", "namespace", namespaceName, "project", projectName, "component", componentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment)
	writeSuccessResponse(w, http.StatusOK, targetReleaseBinding)
}

func (h *Handler) UpdateComponentBinding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("UpdateComponentBinding handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	bindingName := r.PathValue("bindingName")
	if namespaceName == "" || projectName == "" || componentName == "" || bindingName == "" {
		logger.Warn("Namespace name, project name, component name, and binding name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, component name, and binding name are required", "INVALID_PARAMS")
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

	setAuditResource(ctx, "component_binding", bindingName, bindingName)
	addAuditMetadataBatch(ctx, map[string]any{
		"organization": namespaceName,
		"project":      projectName,
		"component":    componentName,
	})

	// Call service to update component binding
	binding, err := h.services.ComponentService.UpdateComponentBinding(ctx, namespaceName, projectName, componentName, bindingName, &req)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrBindingNotFound) {
			logger.Warn("Binding not found", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)
			writeErrorResponse(w, http.StatusNotFound, "Binding not found", services.CodeBindingNotFound)
			return
		}
		logger.Error("Failed to update component binding", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Component binding updated successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)
	writeSuccessResponse(w, http.StatusOK, binding)
}

func (h *Handler) GetComponentObserverURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentObserverURL handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	environmentName := r.PathValue("environmentName")

	if namespaceName == "" || projectName == "" || componentName == "" || environmentName == "" {
		logger.Warn("All path parameters are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace, project, component, and environment names are required", "INVALID_PARAMS")
		return
	}

	// Call service to get observer URL
	observerResponse, err := h.services.ComponentService.GetComponentObserverURL(ctx, namespaceName, projectName, componentName, environmentName)
	if err != nil {
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Error in retrieving the log URL: Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Error in retrieving the log URL: Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrEnvironmentNotFound) {
			logger.Warn("Error in retrieving the log URL: Environment not found", "namespace", namespaceName, "environment", environmentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Environment not found", services.CodeEnvironmentNotFound)
			return
		}
		if errors.Is(err, services.ErrDataPlaneNotFound) {
			logger.Warn("Error in retrieving the log URL: DataPlane not found", "namespace", namespaceName, "environment", environmentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: DataPlane not found", services.CodeDataPlaneNotFound)
			return
		}
		logger.Error("Error in retrieving the log URL: Failed to get component observer URL", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Error in retrieving the log URL: Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved component observer URL successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environmentName)
	writeSuccessResponse(w, http.StatusOK, observerResponse)
}

func (h *Handler) GetBuildObserverURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetBuildObserverURL handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")

	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("All parameters are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace, project, and component names are required", "INVALID_PARAMS")
		return
	}

	// Call service to get build observer URL
	observerResponse, err := h.services.ComponentService.GetBuildObserverURL(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Error in retrieving the log URL: Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Error in retrieving the log URL: Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Error in retrieving the log URL: Project not found", services.CodeProjectNotFound)
			return
		}
		logger.Error("Error in retrieving the log URL: Failed to get build observer URL", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Error in retrieving the log URL: Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Retrieved build observer URL successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	writeSuccessResponse(w, http.StatusOK, observerResponse)
}

func (h *Handler) CreateComponentRelease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("CreateComponentRelease handler called")

	defer r.Body.Close()
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	var req models.CreateComponentReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}

	req.Sanitize()

	componentRelease, err := h.services.ComponentService.CreateComponentRelease(ctx, namespaceName, projectName, componentName, req.ReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to create component release", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrWorkloadNotFound) {
			logger.Warn("Workload not found - component not built", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusBadRequest, "Component has not been built yet", services.CodeWorkloadNotFound)
			return
		}
		logger.Error("Failed to create component release", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Component release created successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "release", componentRelease.Name)
	writeSuccessResponse(w, http.StatusCreated, componentRelease)
}

func (h *Handler) ListComponentReleases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListComponentReleases handler called")

	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	releases, err := h.services.ComponentService.ListComponentReleases(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to list component releases", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Listed component releases successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(releases))
	writeListResponse(w, releases, len(releases), 1, len(releases))
}

func (h *Handler) GetComponentRelease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentRelease handler called")

	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	releaseName := r.PathValue("releaseName")
	if namespaceName == "" || projectName == "" || componentName == "" || releaseName == "" {
		logger.Warn("Namespace name, project name, component name, and release name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, component name, and release name are required", "INVALID_PARAMS")
		return
	}

	release, err := h.services.ComponentService.GetComponentRelease(ctx, namespaceName, projectName, componentName, releaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view component release", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentReleaseNotFound) {
			logger.Warn("Component release not found", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
			writeErrorResponse(w, http.StatusNotFound, "Component release not found", services.CodeComponentReleaseNotFound)
			return
		}
		logger.Error("Failed to get component release", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved component release successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
	writeSuccessResponse(w, http.StatusOK, release)
}

func (h *Handler) GetComponentReleaseSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetComponentReleaseSchema handler called")

	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	releaseName := r.PathValue("releaseName")
	if namespaceName == "" || projectName == "" || componentName == "" || releaseName == "" {
		logger.Warn("Namespace name, project name, component name, and release name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, component name, and release name are required", services.CodeInvalidInput)
		return
	}

	schema, err := h.services.ComponentService.GetComponentReleaseSchema(ctx, namespaceName, projectName, componentName, releaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view component release schema", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentReleaseNotFound) {
			logger.Warn("Component release not found", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
			writeErrorResponse(w, http.StatusNotFound, "Component release not found", services.CodeComponentReleaseNotFound)
			return
		}
		logger.Error("Failed to get component release schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved component release schema successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
	writeSuccessResponse(w, http.StatusOK, schema)
}

func (h *Handler) GetEnvironmentRelease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetReleaseResources handler called")

	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	environmentName := r.PathValue("environmentName")
	if namespaceName == "" || projectName == "" || componentName == "" || environmentName == "" {
		logger.Warn("Namespace name, project name, component name, and environment name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, component name, and environment name are required", services.CodeInvalidInput)
		return
	}

	release, err := h.services.ComponentService.GetEnvironmentRelease(ctx, namespaceName, projectName, componentName, environmentName)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrReleaseNotFound) {
			logger.Warn("Release not found", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environmentName)
			writeErrorResponse(w, http.StatusNotFound, "Release not found", services.CodeReleaseNotFound)
			return
		}
		logger.Error("Failed to get release", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved release successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environmentName, "resourceCount", len(release.Spec.Resources))
	writeSuccessResponse(w, http.StatusOK, release)
}

func (h *Handler) PatchReleaseBinding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("PatchReleaseBinding handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	bindingName := r.PathValue("bindingName")
	if namespaceName == "" || projectName == "" || componentName == "" || bindingName == "" {
		logger.Warn("Namespace name, project name, component name, and binding name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, component name, and binding name are required", "INVALID_PARAMS")
		return
	}

	defer r.Body.Close()
	var req models.PatchReleaseBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}

	binding, err := h.services.ComponentService.PatchReleaseBinding(ctx, namespaceName, projectName, componentName, bindingName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to update release binding", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrReleaseBindingNotFound) {
			logger.Warn("Release binding not found", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)
			writeErrorResponse(w, http.StatusNotFound, "Release binding not found", services.CodeReleaseBindingNotFound)
			return
		}
		logger.Error("Failed to patch release binding", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Patched release binding successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)
	writeSuccessResponse(w, http.StatusOK, binding)
}

func (h *Handler) ListReleaseBindings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListReleaseBindings handler called")

	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	environments := r.URL.Query()["environment"]

	bindings, err := h.services.ComponentService.ListReleaseBindings(ctx, namespaceName, projectName, componentName, environments)
	if err != nil {
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to list release bindings", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Listed release bindings successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(bindings))
	writeListResponse(w, bindings, len(bindings), 1, len(bindings))
}

func (h *Handler) DeployRelease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("DeployRelease handler called")

	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", "INVALID_PARAMS")
		return
	}

	var req models.DeployReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", "INVALID_JSON")
		return
	}
	defer r.Body.Close()

	req.Sanitize()
	if err := req.Validate(); err != nil {
		logger.Warn("Invalid request", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), "INVALID_REQUEST")
		return
	}

	binding, err := h.services.ComponentService.DeployRelease(ctx, namespaceName, projectName, componentName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to deploy component", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentReleaseNotFound) {
			logger.Warn("Component release not found", "namespace", namespaceName, "project", projectName, "component", componentName, "release", req.ReleaseName)
			writeErrorResponse(w, http.StatusNotFound, "Component release not found", services.CodeComponentReleaseNotFound)
			return
		}
		logger.Error("Failed to deploy release", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Deployed release successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "release", req.ReleaseName, "environment", binding.Environment)
	writeSuccessResponse(w, http.StatusCreated, binding)
}

func (h *Handler) ListComponentTraits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListComponentTraits handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", services.CodeInvalidParams)
		return
	}

	// Call service to list component traits
	traits, err := h.services.ComponentService.ListComponentTraits(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view component traits", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		logger.Error("Failed to list component traits", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Listed component traits successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(traits))
	writeListResponse(w, traits, len(traits), 1, len(traits))
}

func (h *Handler) UpdateComponentTraits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("UpdateComponentTraits handler called")

	// Extract path parameters
	namespaceName := r.PathValue("namespaceName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	if namespaceName == "" || projectName == "" || componentName == "" {
		logger.Warn("Namespace name, project name, and component name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name, project name, and component name are required", services.CodeInvalidParams)
		return
	}

	// Parse request body
	var req models.UpdateComponentTraitsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Invalid JSON body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}
	defer r.Body.Close()

	// Sanitize and validate request
	req.Sanitize()
	if err := req.Validate(); err != nil {
		logger.Warn("Invalid request", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	setAuditResource(ctx, "component", componentName, componentName)
	addAuditMetadataBatch(ctx, map[string]any{
		"organization": namespaceName,
		"project":      projectName,
	})

	// Call service to update component traits
	traits, err := h.services.ComponentService.UpdateComponentTraits(ctx, namespaceName, projectName, componentName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to update component traits", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			writeErrorResponse(w, http.StatusNotFound, "Project not found", services.CodeProjectNotFound)
			return
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusNotFound, "Component not found", services.CodeComponentNotFound)
			return
		}
		if errors.Is(err, services.ErrTraitNotFound) {
			logger.Warn("Trait not found", "namespace", namespaceName, "project", projectName, "component", componentName, "error", err)
			writeErrorResponse(w, http.StatusNotFound, err.Error(), services.CodeTraitNotFound)
			return
		}
		logger.Error("Failed to update component traits", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Success response
	logger.Debug("Updated component traits successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(traits))
	writeListResponse(w, traits, len(traits), 1, len(traits))
}
