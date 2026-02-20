// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) ListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListWorkflowRuns handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		logger.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	wfRuns, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, namespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to list workflow runs", "org", namespaceName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		logger.Error("Failed to list WorkflowRuns", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Listed WorkflowRuns successfully", "org", namespaceName, "count", len(wfRuns))
	writeListResponse(w, wfRuns, len(wfRuns), 1, len(wfRuns))
}

func (h *Handler) GetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetWorkflowRun handler called")

	namespaceName := r.PathValue("namespaceName")
	runName := r.PathValue("runName")
	if namespaceName == "" || runName == "" {
		logger.Warn("Namespace name and run name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and run name are required", services.CodeInvalidInput)
		return
	}

	wfRun, err := h.services.WorkflowRunService.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view workflow run", "org", namespaceName, "run", runName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			logger.Warn("WorkflowRun not found", "org", namespaceName, "run", runName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow run not found", services.CodeWorkflowRunNotFound)
			return
		}
		logger.Error("Failed to get WorkflowRun", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved WorkflowRun successfully", "org", namespaceName, "run", runName)
	writeSuccessResponse(w, http.StatusOK, wfRun)
}

func (h *Handler) GetWorkflowRunStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetWorkflowRunStatus handler called")

	namespaceName := r.PathValue("namespaceName")
	runName := r.PathValue("runName")
	if namespaceName == "" || runName == "" {
		logger.Error("Namespace name and run name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and run name are required", services.CodeInvalidInput)
		return
	}

	status, err := h.services.WorkflowRunService.GetWorkflowRunStatus(ctx, namespaceName, runName, h.config.ClusterGateway.URL)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view workflow run status", "org", namespaceName, "run", runName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			logger.Warn("WorkflowRun not found", "org", namespaceName, "run", runName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow run not found", services.CodeWorkflowRunNotFound)
			return
		}
		logger.Error("Failed to get WorkflowRun status", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved WorkflowRun status successfully", "org", namespaceName, "run", runName)
	writeSuccessResponse(w, http.StatusOK, status)
}

func (h *Handler) CreateWorkflowRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("CreateWorkflowRun handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		logger.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
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

	wfRun, err := h.services.WorkflowRunService.CreateWorkflowRun(ctx, namespaceName, &req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to create workflow run", "org", namespaceName, "workflow", req.WorkflowName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowNotFound) {
			logger.Warn("Referenced workflow not found", "org", namespaceName, "workflow", req.WorkflowName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow not found", services.CodeWorkflowNotFound)
			return
		}
		if errors.Is(err, services.ErrWorkflowRunAlreadyExists) {
			logger.Warn("WorkflowRun already exists", "org", namespaceName, "workflow", req.WorkflowName)
			writeErrorResponse(w, http.StatusConflict, "Workflow run already exists", services.CodeWorkflowRunAlreadyExists)
			return
		}
		logger.Error("Failed to create WorkflowRun", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Created WorkflowRun successfully", "org", namespaceName, "run", wfRun.Name, "workflow", req.WorkflowName)
	writeSuccessResponse(w, http.StatusCreated, wfRun)
}

func (h *Handler) GetWorkflowRunLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Debug("GetWorkflowRunLogs handler called")

	namespaceName := r.PathValue("namespaceName")
	runName := r.PathValue("runName")
	stepName := r.URL.Query().Get("step")

	// Parse sinceSeconds parameter (optional, in seconds)
	var sinceSeconds *int64
	if sinceSecondsStr := r.URL.Query().Get("sinceSeconds"); sinceSecondsStr != "" {
		parsed, err := strconv.ParseInt(sinceSecondsStr, 10, 64)
		if err != nil || parsed < 0 {
			log.Error("Invalid sinceSeconds parameter", "sinceSeconds", sinceSecondsStr, "error", err)
			writeErrorResponse(w, http.StatusBadRequest, "Invalid sinceSeconds parameter: must be a non-negative integer", services.CodeInvalidInput)
			return
		}
		sinceSeconds = &parsed
	}

	if namespaceName == "" {
		log.Error("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	if runName == "" {
		log.Error("Workflow run name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Workflow run name is required", services.CodeInvalidInput)
		return
	}

	log = log.With("namespace", namespaceName, "run", runName, "step", stepName, "sinceSeconds", sinceSeconds)

	logs, err := h.services.WorkflowRunService.GetWorkflowRunLogs(ctx, namespaceName, runName, stepName, h.config.ClusterGateway.URL, sinceSeconds)
	if err != nil {
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			log.Warn("Workflow run not found")
			writeErrorResponse(w, http.StatusNotFound, "Workflow run not found", services.CodeWorkflowRunNotFound)
			return
		}
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to view workflow run logs")
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowRunReferenceNotFound) {
			log.Warn("Workflow run reference not ready")
			writeErrorResponse(w, http.StatusNotFound, "Workflow run reference not ready", services.CodeWorkflowRunReferenceNotFound)
			return
		}
		log.Error("Failed to get workflow run logs", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get workflow run logs", services.CodeInternalError)
		return
	}

	// Return logs as JSON array
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(logs); err != nil {
		log.Error("Failed to encode logs response", "error", err)
	}
}

func (h *Handler) GetWorkflowRunEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Debug("GetWorkflowRunEvents handler called")

	namespaceName := r.PathValue("namespaceName")
	runName := r.PathValue("runName")
	stepName := r.URL.Query().Get("step")

	if namespaceName == "" {
		log.Error("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	if runName == "" {
		log.Error("Workflow run name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Workflow run name is required", services.CodeInvalidInput)
		return
	}

	log = log.With("namespace", namespaceName, "run", runName, "step", stepName)

	events, err := h.services.WorkflowRunService.GetWorkflowRunEvents(ctx, namespaceName, runName, stepName, h.config.ClusterGateway.URL)
	if err != nil {
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			log.Warn("Workflow run not found")
			writeErrorResponse(w, http.StatusNotFound, "Workflow run not found", services.CodeWorkflowRunNotFound)
			return
		}
		if errors.Is(err, services.ErrForbidden) {
			log.Warn("Unauthorized to view workflow run events")
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowRunReferenceNotFound) {
			log.Warn("Workflow run reference not ready")
			writeErrorResponse(w, http.StatusNotFound, "Workflow run reference not ready", services.CodeWorkflowRunReferenceNotFound)
			return
		}
		log.Error("Failed to get workflow run events", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get workflow run events", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, events)
}
