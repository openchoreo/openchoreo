// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) ListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListWorkflowRuns handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	wfRuns, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, orgName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to list workflow runs", "org", orgName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		logger.Error("Failed to list WorkflowRuns", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Listed WorkflowRuns successfully", "org", orgName, "count", len(wfRuns))
	writeListResponse(w, wfRuns, len(wfRuns), 1, len(wfRuns))
}

func (h *Handler) GetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetWorkflowRun handler called")

	orgName := r.PathValue("orgName")
	runName := r.PathValue("runName")
	if orgName == "" || runName == "" {
		logger.Warn("Organization name and run name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and run name are required", services.CodeInvalidInput)
		return
	}

	wfRun, err := h.services.WorkflowRunService.GetWorkflowRun(ctx, orgName, runName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view workflow run", "org", orgName, "run", runName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			logger.Warn("WorkflowRun not found", "org", orgName, "run", runName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow run not found", services.CodeWorkflowRunNotFound)
			return
		}
		logger.Error("Failed to get WorkflowRun", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved WorkflowRun successfully", "org", orgName, "run", runName)
	writeSuccessResponse(w, http.StatusOK, wfRun)
}

func (h *Handler) CreateWorkflowRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("CreateWorkflowRun handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	var req models.CreateWorkflowRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Sanitize and validate the request
	req.Sanitize()
	if err := req.Validate(); err != nil {
		logger.Warn("Invalid request", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	wfRun, err := h.services.WorkflowRunService.CreateWorkflowRun(ctx, orgName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to create workflow run", "org", orgName, "workflow", req.WorkflowName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowNotFound) {
			logger.Warn("Referenced workflow not found", "org", orgName, "workflow", req.WorkflowName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow not found", services.CodeWorkflowNotFound)
			return
		}
		if errors.Is(err, services.ErrWorkflowRunAlreadyExists) {
			logger.Warn("WorkflowRun already exists", "org", orgName, "workflow", req.WorkflowName)
			writeErrorResponse(w, http.StatusConflict, "Workflow run already exists", services.CodeWorkflowRunAlreadyExists)
			return
		}
		logger.Error("Failed to create WorkflowRun", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Created WorkflowRun successfully", "org", orgName, "run", wfRun.Name, "workflow", req.WorkflowName)
	writeSuccessResponse(w, http.StatusCreated, wfRun)
}