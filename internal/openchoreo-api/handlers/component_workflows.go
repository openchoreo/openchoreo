// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListComponentWorkflows lists ComponentWorkflow templates in an organization
func (h *Handler) ListComponentWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Debug("ListComponentWorkflows handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		log.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		log.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	result, err := h.services.ComponentWorkflowService.ListComponentWorkflows(ctx, orgName, opts)
	if err != nil {
		if errors.Is(err, services.ErrContinueTokenExpired) {
			log.Warn("Continue token expired")
			writeErrorResponse(w, http.StatusGone,
				"Continue token has expired. Please restart listing from the beginning.",
				services.CodeContinueTokenExpired)
			return
		}
		if errors.Is(err, services.ErrInvalidContinueToken) {
			log.Warn("Invalid continue token provided")
			writeErrorResponse(w, http.StatusBadRequest,
				"Invalid continue token provided",
				services.CodeInvalidContinueToken)
			return
		}
		log.Error("Failed to list ComponentWorkflows", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	log.Debug("Listed ComponentWorkflows successfully", "org", orgName, "count", len(result.Items), "hasMore", result.Metadata.HasMore)
	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}

// GetComponentWorkflowSchema retrieves the schema for a ComponentWorkflow template
func (h *Handler) GetComponentWorkflowSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Debug("GetComponentWorkflowSchema handler called")

	orgName := r.PathValue("orgName")
	cwName := r.PathValue("cwName")
	if orgName == "" || cwName == "" {
		log.Warn("Organization name and ComponentWorkflow name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and ComponentWorkflow name are required", services.CodeInvalidInput)
		return
	}

	schema, err := h.services.ComponentWorkflowService.GetComponentWorkflowSchema(ctx, orgName, cwName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to view component workflow schema", "org", orgName, "componentWorkflow", cwName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrComponentWorkflowNotFound) {
			log.Warn("ComponentWorkflow not found", "org", orgName, "name", cwName)
			writeErrorResponse(w, http.StatusNotFound, "ComponentWorkflow not found", services.CodeComponentWorkflowNotFound)
			return
		}
		log.Error("Failed to get ComponentWorkflow schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	log.Debug("Retrieved ComponentWorkflow schema successfully", "org", orgName, "name", cwName)
	writeSuccessResponse(w, http.StatusOK, schema)
}

// CreateComponentWorkflowRun creates a new component workflow run
func (h *Handler) CreateComponentWorkflowRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("CreateComponentWorkflowRun handler called")

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

	workflowRun, err := h.services.ComponentWorkflowService.TriggerWorkflow(ctx, orgName, projectName, componentName, commit)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to trigger component workflow", "org", orgName, "project", projectName, "component", componentName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		// Check for invalid commit SHA error
		if errors.Is(err, services.ErrInvalidCommitSHA) {
			log.Warn("Invalid commit SHA provided", "commit", commit)
			writeErrorResponse(w, http.StatusBadRequest,
				fmt.Sprintf("Invalid commit SHA format: '%s'. Commit SHA must be 7-40 hexadecimal characters", commit),
				services.CodeInvalidCommitSHA)
			return
		}
		log.Error("Failed to trigger component workflow", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to trigger component workflow", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusCreated, workflowRun)
}

// ListComponentWorkflowRuns lists component workflow runs for a specific component
func (h *Handler) ListComponentWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("ListComponentWorkflowRuns handler called")

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

	// Extract and validate list parameters
	opts, err := extractListParams(r.URL.Query())
	if err != nil {
		log.Warn("Invalid list parameters", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	// Call service to list component workflow runs
	result, err := h.services.ComponentWorkflowService.ListComponentWorkflowRuns(ctx, orgName, projectName, componentName, opts)
	if err != nil {
		if errors.Is(err, services.ErrContinueTokenExpired) {
			log.Warn("Continue token expired")
			writeErrorResponse(w, http.StatusGone,
				"Continue token has expired. Please restart listing from the beginning.",
				services.CodeContinueTokenExpired)
			return
		}
		if errors.Is(err, services.ErrInvalidContinueToken) {
			log.Warn("Invalid continue token provided")
			writeErrorResponse(w, http.StatusBadRequest,
				"Invalid continue token provided",
				services.CodeInvalidContinueToken)
			return
		}

		// List operations don't check for ErrForbidden here - the service already filtered unauthorized items
		log.Error("Failed to list component workflow runs", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list component workflow runs", services.CodeInternalError)
		return
	}

	// Success response
	writeListResponse(w, result.Items, result.Metadata.ResourceVersion, result.Metadata.Continue)
}

// GetComponentWorkflowRun retrieves a specific component workflow run
func (h *Handler) GetComponentWorkflowRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("GetComponentWorkflowRun handler called")

	// Extract parameters from URL path
	orgName := r.PathValue("orgName")
	projectName := r.PathValue("projectName")
	componentName := r.PathValue("componentName")
	runName := r.PathValue("runName")

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

	if runName == "" {
		log.Warn("Workflow run name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Workflow run name is required", "INVALID_RUN_NAME")
		return
	}

	// Call service to get component workflow run
	workflowRun, err := h.services.ComponentWorkflowService.GetComponentWorkflowRun(ctx, orgName, projectName, componentName, runName)
	if err != nil {
		if errors.Is(err, services.ErrComponentWorkflowRunNotFound) {
			log.Warn("Component workflow run not found", "org", orgName, "project", projectName, "component", componentName, "run", runName)
			writeErrorResponse(w, http.StatusNotFound, "Component workflow run not found", services.CodeComponentWorkflowRunNotFound)
			return
		}
		log.Error("Failed to get component workflow run", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get component workflow run", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeSuccessResponse(w, http.StatusOK, workflowRun)
}
