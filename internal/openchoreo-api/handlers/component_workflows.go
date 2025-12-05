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

	cwfs, err := h.services.ComponentWorkflowService.ListComponentWorkflows(ctx, orgName)
	if err != nil {
		log.Error("Failed to list ComponentWorkflows", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	log.Debug("Listed ComponentWorkflows successfully", "org", orgName, "count", len(cwfs))
	writeListResponse(w, cwfs, len(cwfs), 1, len(cwfs))
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

// TriggerComponentWorkflow triggers a new component workflow run
func (h *Handler) TriggerComponentWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Info("TriggerWorkflow handler called")

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

	// Call service to list component workflow runs
	workflowRuns, err := h.services.ComponentWorkflowService.ListComponentWorkflowRuns(ctx, orgName, projectName, componentName)
	if err != nil {
		log.Error("Failed to list component workflow runs", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list component workflow runs", "INTERNAL_ERROR")
		return
	}

	// Success response
	writeListResponse(w, workflowRuns, len(workflowRuns), 1, len(workflowRuns))
}
