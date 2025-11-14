// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListWorkflows handler called")

	orgName := r.PathValue("orgName")
	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	wfs, err := h.services.WorkflowService.ListWorkflows(ctx, orgName)
	if err != nil {
		logger.Error("Failed to list Workflows", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Listed Workflows successfully", "org", orgName, "count", len(wfs))
	writeListResponse(w, wfs, len(wfs), 1, len(wfs))
}

func (h *Handler) GetWorkflowSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetWorkflowSchema handler called")

	orgName := r.PathValue("orgName")
	workflowName := r.PathValue("workflowName")
	if orgName == "" || workflowName == "" {
		logger.Warn("Organization name and Workflow name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name and Workflow name are required", services.CodeInvalidInput)
		return
	}

	schema, err := h.services.WorkflowService.GetWorkflowSchema(ctx, orgName, workflowName)
	if err != nil {
		if errors.Is(err, services.ErrWorkflowNotFound) {
			logger.Warn("Workflow not found", "org", orgName, "name", workflowName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow not found", services.CodeWorkflowNotFound)
			return
		}
		logger.Error("Failed to get Workflow schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved Workflow schema successfully", "org", orgName, "name", workflowName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
