// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/logger"
)

func (h *Handler) ListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Debug("ListWorkflowRuns handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	// Optional query params to filter by project and component labels
	projectName := r.URL.Query().Get("projectName")
	componentName := r.URL.Query().Get("componentName")

	// Authorization check
	//if err := h.services.WorkflowRunService.AuthorizeView(ctx, namespaceName, projectName, componentName); err != nil {
	//	log.Warn("Unauthorized to list workflow runs", "namespace", namespaceName)
	//	writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
	//	return
	//}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(openChoreoGVK("WorkflowRunList"))

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	// Add label-based filtering if project or component specified
	labelSelector := make(map[string]string)
	if projectName != "" {
		labelSelector[ocLabels.LabelKeyProjectName] = projectName
	}
	if componentName != "" {
		labelSelector[ocLabels.LabelKeyComponentName] = componentName
	}
	if len(labelSelector) > 0 {
		listOpts = append(listOpts, client.MatchingLabels(labelSelector))
	}

	k8sClient := h.services.GetKubernetesClient()
	if err := k8sClient.List(ctx, list, listOpts...); err != nil {
		log.Error("Failed to list WorkflowRuns", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	items := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, item.Object)
	}

	log.Debug("Listed WorkflowRuns successfully", "namespace", namespaceName, "count", len(items))
	writeListResponse(w, items, len(items), 1, len(items))
}

func (h *Handler) GetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Debug("GetWorkflowRun handler called")

	namespaceName := r.PathValue("namespaceName")
	runName := r.PathValue("runName")
	if namespaceName == "" || runName == "" {
		log.Warn("Namespace name and run name are required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name and run name are required", services.CodeInvalidInput)
		return
	}

	// Optional query params for authorization
	//projectName := r.URL.Query().Get("projectName")
	//componentName := r.URL.Query().Get("componentName")

	// Authorization check
	//if err := h.services.WorkflowRunService.AuthorizeView(ctx, namespaceName, projectName, componentName); err != nil {
	//	log.Warn("Unauthorized to view workflow run", "namespace", namespaceName, "run", runName)
	//	writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
	//	return
	//}

	gvk := openChoreoGVK("WorkflowRun")
	obj, err := h.getResourceByGVK(ctx, gvk, namespaceName, runName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Warn("WorkflowRun not found", "namespace", namespaceName, "run", runName)
			writeErrorResponse(w, http.StatusNotFound, "Workflow run not found", services.CodeWorkflowRunNotFound)
			return
		}
		log.Error("Failed to get WorkflowRun", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	log.Debug("Retrieved WorkflowRun successfully", "namespace", namespaceName, "run", runName)
	writeSuccessResponse(w, http.StatusOK, obj.Object)
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
	log := logger.GetLogger(ctx)
	log.Debug("CreateWorkflowRun handler called")

	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		log.Warn("Namespace name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	// Decode the full WorkflowRun YAML/JSON from the request body
	var resourceObj map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&resourceObj); err != nil {
		log.Warn("Failed to decode request body", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Validate the resource (kind, apiVersion, name)
	kind, apiVersion, name, err := validateResourceRequest(resourceObj)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
		return
	}

	if kind != "WorkflowRun" {
		writeErrorResponse(w, http.StatusBadRequest, "Kind must be WorkflowRun", services.CodeInvalidInput)
		return
	}

	// Extract labels from the request body for authorization
	unstructuredObj := &unstructured.Unstructured{Object: resourceObj}
	//labels := unstructuredObj.GetLabels()
	//projectName := labels[ocLabels.LabelKeyProjectName]
	//componentName := labels[ocLabels.LabelKeyComponentName]
	//
	//// Authorize using labels from the request body
	//if err := h.services.WorkflowRunService.AuthorizeCreate(ctx, namespaceName, projectName, componentName); err != nil {
	//	log.Warn("Authorization failed for WorkflowRun creation", "namespace", namespaceName, "name", name, "error", err)
	//	writeErrorResponse(w, http.StatusForbidden, "Not authorized to create WorkflowRun", services.CodeForbidden)
	//	return
	//}

	// Set namespace from URL
	unstructuredObj.SetNamespace(namespaceName)

	// Check if a WorkflowRun with the same name already exists
	gvk := openChoreoGVK("WorkflowRun")
	existing, err := h.getResourceByGVK(ctx, gvk, namespaceName, name)
	if err == nil && existing != nil {
		log.Warn("WorkflowRun already exists", "namespace", namespaceName, "name", name)
		writeErrorResponse(w, http.StatusConflict, "WorkflowRun already exists with name: "+name, services.CodeWorkflowRunAlreadyExists)
		return
	}
	// If err is not a "not found" error, it's an unexpected error
	if err != nil && client.IgnoreNotFound(err) != nil {
		log.Error("Failed to check existing WorkflowRun", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error", services.CodeInternalError)
		return
	}

	// Handle namespace logic
	if err := h.handleResourceNamespace(unstructuredObj, apiVersion, kind); err != nil {
		log.Error("Failed to handle resource namespace", "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Failed to handle resource namespace: "+err.Error(), services.CodeInvalidInput)
		return
	}

	// Apply the resource to Kubernetes
	operation, err := h.applyToKubernetes(ctx, unstructuredObj)
	if err != nil {
		log.Error("Failed to create WorkflowRun", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create WorkflowRun: "+err.Error(), services.CodeInternalError)
		return
	}

	response := ResourceCRUDResponse{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		Namespace:  namespaceName,
		Operation:  operation,
	}

	log.Info("WorkflowRun created successfully", "namespace", namespaceName, "name", name, "operation", operation)
	writeSuccessResponse(w, http.StatusCreated, response)
}

func (h *Handler) GetWorkflowRunLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	log.Debug("GetWorkflowRunLogs handler called")

	namespaceName := r.PathValue("namespaceName")
	runName := r.PathValue("runName")
	stepName := r.URL.Query().Get("step")

	var sinceSeconds *int64
	if sinceSecondsStr := r.URL.Query().Get("sinceSeconds"); sinceSecondsStr != "" {
		parsed, err := strconv.ParseInt(sinceSecondsStr, 10, 64)
		if err != nil || parsed < 0 {
			log.Error("Invalid sinceSeconds parameter", "sinceSeconds", sinceSecondsStr)
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
		log.Error("Failed to get workflow run logs", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get workflow run logs", services.CodeInternalError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(logs); err != nil {
		log.Error("Failed to encode logs response", "error", err)
	}
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
