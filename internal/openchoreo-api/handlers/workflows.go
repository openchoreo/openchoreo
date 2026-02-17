// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"

	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("ListWorkflows handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		logger.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	wfs, err := h.services.WorkflowService.ListWorkflows(ctx, namespaceName)
	if err != nil {
		logger.Error("Failed to list Workflows", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Listed Workflows successfully", "namespace", namespaceName, "count", len(wfs))
	writeListResponse(w, wfs, len(wfs), 1, len(wfs))
}

func (h *Handler) GetWorkflowSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	logger.Debug("GetWorkflowSchema handler called")

	namespaceName := r.PathValue("namespaceName")
	workflowName := r.PathValue("workflowName")
	if namespaceName == "" || workflowName == "" {
		logger.Warn("Namespace name and Workflow name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and Workflow name are required", services.CodeInvalidInput)
		return
	}

	schema, err := h.services.WorkflowService.GetWorkflowSchema(ctx, namespaceName, workflowName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			logger.Warn("Unauthorized to view workflow schema", "namespace", namespaceName, "workflow", workflowName)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, services.ErrWorkflowNotFound) {
			logger.Warn("Workflow not found", "namespace", namespaceName, "name", workflowName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow not found", services.CodeWorkflowNotFound)
			return
		}
		logger.Error("Failed to get Workflow schema", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	logger.Debug("Retrieved Workflow schema successfully", "namespace", namespaceName, "name", workflowName)
	writeSuccessResponse(w, http.StatusOK, schema)
}
