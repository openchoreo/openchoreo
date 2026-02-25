// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	svcerrors "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	workflowsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflow"
	workflowrunsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun"
)

// ListWorkflows returns a paginated list of workflows within a namespace.
func (h *Handler) ListWorkflows(
	ctx context.Context,
	request gen.ListWorkflowsRequestObject,
) (gen.ListWorkflowsResponseObject, error) {
	h.logger.Debug("ListWorkflows called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.WorkflowService.ListWorkflows(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list workflows", "error", err)
		return gen.ListWorkflows500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Workflow, gen.Workflow](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert workflows", "error", err)
		return gen.ListWorkflows500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListWorkflows200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateWorkflow creates a new workflow within a namespace.
func (h *Handler) CreateWorkflow(
	ctx context.Context,
	request gen.CreateWorkflowRequestObject,
) (gen.CreateWorkflowResponseObject, error) {
	h.logger.Info("CreateWorkflow called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	wfCR, err := convert[gen.Workflow, openchoreov1alpha1.Workflow](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	wfCR.Status = openchoreov1alpha1.WorkflowStatus{}

	created, err := h.services.WorkflowService.CreateWorkflow(ctx, request.NamespaceName, &wfCR)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.CreateWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowsvc.ErrWorkflowAlreadyExists) {
			return gen.CreateWorkflow409JSONResponse{ConflictJSONResponse: conflict("Workflow already exists")}, nil
		}
		h.logger.Error("Failed to create workflow", "error", err)
		return gen.CreateWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWf, err := convert[openchoreov1alpha1.Workflow, gen.Workflow](*created)
	if err != nil {
		h.logger.Error("Failed to convert created workflow", "error", err)
		return gen.CreateWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Workflow created successfully", "namespaceName", request.NamespaceName, "workflow", created.Name)
	return gen.CreateWorkflow201JSONResponse(genWf), nil
}

// GetWorkflow returns details of a specific workflow.
func (h *Handler) GetWorkflow(
	ctx context.Context,
	request gen.GetWorkflowRequestObject,
) (gen.GetWorkflowResponseObject, error) {
	h.logger.Debug("GetWorkflow called", "namespaceName", request.NamespaceName, "workflowName", request.WorkflowName)

	wf, err := h.services.WorkflowService.GetWorkflow(ctx, request.NamespaceName, request.WorkflowName)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.GetWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowsvc.ErrWorkflowNotFound) {
			return gen.GetWorkflow404JSONResponse{NotFoundJSONResponse: notFound("Workflow")}, nil
		}
		h.logger.Error("Failed to get workflow", "error", err, "namespace", request.NamespaceName, "workflow", request.WorkflowName)
		return gen.GetWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWf, err := convert[openchoreov1alpha1.Workflow, gen.Workflow](*wf)
	if err != nil {
		h.logger.Error("Failed to convert workflow", "error", err)
		return gen.GetWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetWorkflow200JSONResponse(genWf), nil
}

// UpdateWorkflow replaces an existing workflow (full update).
func (h *Handler) UpdateWorkflow(
	ctx context.Context,
	request gen.UpdateWorkflowRequestObject,
) (gen.UpdateWorkflowResponseObject, error) {
	h.logger.Info("UpdateWorkflow called", "namespaceName", request.NamespaceName, "workflowName", request.WorkflowName)

	if request.Body == nil {
		return gen.UpdateWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	wfCR, err := convert[gen.Workflow, openchoreov1alpha1.Workflow](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	wfCR.Status = openchoreov1alpha1.WorkflowStatus{}

	// Ensure the name from the URL path is used
	wfCR.Name = request.WorkflowName

	updated, err := h.services.WorkflowService.UpdateWorkflow(ctx, request.NamespaceName, &wfCR)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.UpdateWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowsvc.ErrWorkflowNotFound) {
			return gen.UpdateWorkflow404JSONResponse{NotFoundJSONResponse: notFound("Workflow")}, nil
		}
		h.logger.Error("Failed to update workflow", "error", err)
		return gen.UpdateWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWf, err := convert[openchoreov1alpha1.Workflow, gen.Workflow](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated workflow", "error", err)
		return gen.UpdateWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Workflow updated successfully", "namespaceName", request.NamespaceName, "workflow", updated.Name)
	return gen.UpdateWorkflow200JSONResponse(genWf), nil
}

// DeleteWorkflow deletes a workflow by name.
func (h *Handler) DeleteWorkflow(
	ctx context.Context,
	request gen.DeleteWorkflowRequestObject,
) (gen.DeleteWorkflowResponseObject, error) {
	h.logger.Info("DeleteWorkflow called", "namespaceName", request.NamespaceName, "workflowName", request.WorkflowName)

	err := h.services.WorkflowService.DeleteWorkflow(ctx, request.NamespaceName, request.WorkflowName)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.DeleteWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowsvc.ErrWorkflowNotFound) {
			return gen.DeleteWorkflow404JSONResponse{NotFoundJSONResponse: notFound("Workflow")}, nil
		}
		h.logger.Error("Failed to delete workflow", "error", err)
		return gen.DeleteWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Workflow deleted successfully", "namespaceName", request.NamespaceName, "workflow", request.WorkflowName)
	return gen.DeleteWorkflow204Response{}, nil
}

// GetWorkflowSchema returns the parameter schema for a workflow
func (h *Handler) GetWorkflowSchema(
	ctx context.Context,
	request gen.GetWorkflowSchemaRequestObject,
) (gen.GetWorkflowSchemaResponseObject, error) {
	h.logger.Debug("GetWorkflowSchema called", "namespaceName", request.NamespaceName, "workflowName", request.WorkflowName)

	jsonSchema, err := h.services.WorkflowService.GetWorkflowSchema(ctx, request.NamespaceName, request.WorkflowName)
	if err != nil {
		if errors.Is(err, workflowsvc.ErrWorkflowNotFound) {
			return gen.GetWorkflowSchema404JSONResponse{NotFoundJSONResponse: notFound("Workflow")}, nil
		}
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.GetWorkflowSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get workflow schema", "error", err)
		return gen.GetWorkflowSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert JSONSchemaProps to SchemaResponse (map[string]interface{})
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		h.logger.Error("Failed to marshal schema", "error", err)
		return gen.GetWorkflowSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	var schemaResp gen.SchemaResponse
	if err := json.Unmarshal(data, &schemaResp); err != nil {
		h.logger.Error("Failed to unmarshal schema response", "error", err)
		return gen.GetWorkflowSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetWorkflowSchema200JSONResponse(schemaResp), nil
}

// ListWorkflowRuns returns a list of workflow runs
func (h *Handler) ListWorkflowRuns(
	ctx context.Context,
	request gen.ListWorkflowRunsRequestObject,
) (gen.ListWorkflowRunsResponseObject, error) {
	h.logger.Info("ListWorkflowRuns called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, request.NamespaceName, "", "", opts)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.ListWorkflowRuns403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list workflow runs", "error", err)
		return gen.ListWorkflowRuns500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.WorkflowRun, gen.WorkflowRun](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert workflow runs", "error", err)
		return gen.ListWorkflowRuns500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListWorkflowRuns200JSONResponse(gen.WorkflowRunList{
		Items:      items,
		Pagination: ToPagination(result),
	}), nil
}

// CreateWorkflowRun creates a new workflow run
func (h *Handler) CreateWorkflowRun(
	ctx context.Context,
	request gen.CreateWorkflowRunRequestObject,
) (gen.CreateWorkflowRunResponseObject, error) {
	h.logger.Info("CreateWorkflowRun called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateWorkflowRun400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	wfRunCR, err := convert[gen.WorkflowRun, openchoreov1alpha1.WorkflowRun](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateWorkflowRun400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	wfRunCR.Status = openchoreov1alpha1.WorkflowRunStatus{}

	created, err := h.services.WorkflowRunService.CreateWorkflowRun(ctx, request.NamespaceName, &wfRunCR)
	if err != nil {
		if errors.Is(err, workflowrunsvc.ErrWorkflowNotFound) {
			return gen.CreateWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("Workflow")}, nil
		}
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.CreateWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to create workflow run", "error", err)
		return gen.CreateWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWfRun, err := convert[openchoreov1alpha1.WorkflowRun, gen.WorkflowRun](*created)
	if err != nil {
		h.logger.Error("Failed to convert created workflow run", "error", err)
		return gen.CreateWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("WorkflowRun created successfully", "namespaceName", request.NamespaceName, "workflowRun", created.Name)
	return gen.CreateWorkflowRun201JSONResponse(genWfRun), nil
}

// GetWorkflowRun returns a specific workflow run
func (h *Handler) GetWorkflowRun(
	ctx context.Context,
	request gen.GetWorkflowRunRequestObject,
) (gen.GetWorkflowRunResponseObject, error) {
	h.logger.Info("GetWorkflowRun called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	wfRun, err := h.services.WorkflowRunService.GetWorkflowRun(ctx, request.NamespaceName, request.RunName)
	if err != nil {
		if errors.Is(err, workflowrunsvc.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.GetWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get workflow run", "error", err)
		return gen.GetWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWfRun, err := convert[openchoreov1alpha1.WorkflowRun, gen.WorkflowRun](*wfRun)
	if err != nil {
		h.logger.Error("Failed to convert workflow run", "error", err)
		return gen.GetWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetWorkflowRun200JSONResponse(genWfRun), nil
}

// GetWorkflowRunStatus returns the status and per-step details of a specific workflow run
func (h *Handler) GetWorkflowRunStatus(
	ctx context.Context,
	request gen.GetWorkflowRunStatusRequestObject,
) (gen.GetWorkflowRunStatusResponseObject, error) {
	h.logger.Info("GetWorkflowRunStatus called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	status, err := h.legacyServices.WorkflowRunService.GetWorkflowRunStatus(ctx, request.NamespaceName, request.RunName, h.Config.ClusterGateway.URL)
	if err != nil {
		if errors.Is(err, legacyservices.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunStatus404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, legacyservices.ErrForbidden) {
			return gen.GetWorkflowRunStatus403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get workflow run status", "error", err)
		return gen.GetWorkflowRunStatus500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	steps := make([]gen.WorkflowStepStatus, 0, len(status.Steps))
	for _, s := range status.Steps {
		step := gen.WorkflowStepStatus{
			Name:       s.Name,
			Phase:      normalizeStepPhase(s.Phase),
			StartedAt:  s.StartedAt,
			FinishedAt: s.FinishedAt,
		}
		steps = append(steps, step)
	}

	return gen.GetWorkflowRunStatus200JSONResponse{
		Status:               gen.WorkflowRunStatusResponseStatus(status.Status),
		Steps:                steps,
		HasLiveObservability: status.HasLiveObservability,
	}, nil
}

// GetWorkflowRunLogs returns logs for a specific workflow run
func (h *Handler) GetWorkflowRunLogs(
	ctx context.Context,
	request gen.GetWorkflowRunLogsRequestObject,
) (gen.GetWorkflowRunLogsResponseObject, error) {
	h.logger.Info("GetWorkflowRunLogs called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	step := ""
	if request.Params.Step != nil {
		step = *request.Params.Step
	}

	logs, err := h.legacyServices.WorkflowRunService.GetWorkflowRunLogs(ctx, request.NamespaceName, request.RunName,
		step, h.Config.ClusterGateway.URL, request.Params.SinceSeconds)
	if err != nil {
		if errors.Is(err, legacyservices.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunLogs404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, legacyservices.ErrForbidden) {
			return gen.GetWorkflowRunLogs403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, legacyservices.ErrWorkflowRunReferenceNotFound) {
			return gen.GetWorkflowRunLogs404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		h.logger.Error("Failed to get workflow run logs", "error", err)
		return gen.GetWorkflowRunLogs500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result := make(gen.GetWorkflowRunLogs200JSONResponse, 0, len(logs))
	for _, entry := range logs {
		logEntry := gen.WorkflowRunLogEntry{Log: entry.Log}
		if entry.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
				logEntry.Timestamp = &ts
			} else if ts, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
				logEntry.Timestamp = &ts
			}
		}
		result = append(result, logEntry)
	}
	return result, nil
}

// GetWorkflowRunEvents returns Kubernetes events for a specific workflow run
func (h *Handler) GetWorkflowRunEvents(
	ctx context.Context,
	request gen.GetWorkflowRunEventsRequestObject,
) (gen.GetWorkflowRunEventsResponseObject, error) {
	h.logger.Info("GetWorkflowRunEvents called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	step := ""
	if request.Params.Step != nil {
		step = *request.Params.Step
	}

	events, err := h.legacyServices.WorkflowRunService.GetWorkflowRunEvents(ctx, request.NamespaceName, request.RunName,
		step, h.Config.ClusterGateway.URL)
	if err != nil {
		if errors.Is(err, legacyservices.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunEvents404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, legacyservices.ErrForbidden) {
			return gen.GetWorkflowRunEvents403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, legacyservices.ErrWorkflowRunReferenceNotFound) {
			return gen.GetWorkflowRunEvents404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRunReference")}, nil
		}
		h.logger.Error("Failed to get workflow run events", "error", err)
		return gen.GetWorkflowRunEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result := make(gen.GetWorkflowRunEvents200JSONResponse, 0, len(events))
	for _, entry := range events {
		ts, err := time.Parse(time.RFC3339, entry.Timestamp)
		if err != nil {
			ts, err = time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err != nil {
				h.logger.Warn("Failed to parse event timestamp", "timestamp", entry.Timestamp, "error", err)
			}
		}
		result = append(result, gen.WorkflowRunEventEntry{
			Timestamp: ts,
			Type:      entry.Type,
			Reason:    entry.Reason,
			Message:   entry.Message,
		})
	}
	return result, nil
}

// normalizeStepPhase maps a raw phase string from the WorkflowTask CRD to a
// valid OpenAPI enum value. Argo's "Omitted" phase is mapped to "Skipped";
// any other unrecognized value falls back to "Error".
func normalizeStepPhase(phase string) gen.WorkflowStepStatusPhase {
	switch phase {
	case "Pending", "Running", "Succeeded", "Failed", "Skipped", "Error":
		return gen.WorkflowStepStatusPhase(phase)
	case "Omitted":
		return gen.WorkflowStepStatusPhaseSkipped
	default:
		return gen.WorkflowStepStatusPhaseError
	}
}
