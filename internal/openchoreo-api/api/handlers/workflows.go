// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"time"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListWorkflows returns a list of generic workflows
func (h *Handler) ListWorkflows(
	ctx context.Context,
	request gen.ListWorkflowsRequestObject,
) (gen.ListWorkflowsResponseObject, error) {
	h.logger.Debug("ListWorkflows called", "namespaceName", request.NamespaceName)

	workflows, err := h.legacyServices.WorkflowService.ListWorkflows(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list workflows", "error", err)
		return gen.ListWorkflows500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Workflow, 0, len(workflows))
	for _, wf := range workflows {
		items = append(items, toGenWorkflow(wf))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListWorkflows200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenWorkflow converts models.WorkflowResponse to gen.Workflow
func toGenWorkflow(wf *models.WorkflowResponse) gen.Workflow {
	return gen.Workflow{
		Name:        wf.Name,
		DisplayName: ptr.To(wf.DisplayName),
		Description: ptr.To(wf.Description),
		CreatedAt:   wf.CreatedAt,
	}
}

// GetWorkflowSchema returns the parameter schema for a workflow
func (h *Handler) GetWorkflowSchema(
	ctx context.Context,
	request gen.GetWorkflowSchemaRequestObject,
) (gen.GetWorkflowSchemaResponseObject, error) {
	return nil, errNotImplemented
}

// ListWorkflowRuns returns a list of workflow runs
func (h *Handler) ListWorkflowRuns(
	ctx context.Context,
	request gen.ListWorkflowRunsRequestObject,
) (gen.ListWorkflowRunsResponseObject, error) {
	h.logger.Info("ListWorkflowRuns called", "namespaceName", request.NamespaceName)

	runs, err := h.legacyServices.WorkflowRunService.ListWorkflowRuns(ctx, request.NamespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListWorkflowRuns403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list workflow runs", "error", err)
		return gen.ListWorkflowRuns500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result := gen.WorkflowRunList{
		Items: make([]gen.WorkflowRun, 0, len(runs)),
	}
	for _, run := range runs {
		result.Items = append(result.Items, toGenWorkflowRun(run))
	}

	return gen.ListWorkflowRuns200JSONResponse(result), nil
}

// CreateWorkflowRun creates a new workflow run
func (h *Handler) CreateWorkflowRun(
	ctx context.Context,
	request gen.CreateWorkflowRunRequestObject,
) (gen.CreateWorkflowRunResponseObject, error) {
	h.logger.Info("CreateWorkflowRun called",
		"namespace", request.NamespaceName,
		"workflow", request.Body.WorkflowName)

	// Convert request to models.CreateWorkflowRunRequest
	req := &models.CreateWorkflowRunRequest{
		WorkflowName: request.Body.WorkflowName,
		Parameters:   request.Body.Parameters,
	}

	// Call service to create workflow run
	workflowRun, err := h.legacyServices.WorkflowRunService.CreateWorkflowRun(ctx, request.NamespaceName, req)
	if err != nil {
		if errors.Is(err, services.ErrWorkflowNotFound) {
			return gen.CreateWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("Workflow")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to create workflow run", "error", err)
		return gen.CreateWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.CreateWorkflowRun201JSONResponse(toGenWorkflowRun(workflowRun)), nil
}

// GetWorkflowRun returns a specific workflow run
func (h *Handler) GetWorkflowRun(
	ctx context.Context,
	request gen.GetWorkflowRunRequestObject,
) (gen.GetWorkflowRunResponseObject, error) {
	h.logger.Info("GetWorkflowRun called",
		"namespace", request.NamespaceName,
		"runName", request.RunName)

	workflowRun, err := h.legacyServices.WorkflowRunService.GetWorkflowRun(ctx, request.NamespaceName, request.RunName)
	if err != nil {
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get workflow run", "error", err)
		return gen.GetWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetWorkflowRun200JSONResponse(toGenWorkflowRun(workflowRun)), nil
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
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunStatus404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
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
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunLogs404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetWorkflowRunLogs403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrWorkflowRunReferenceNotFound) {
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
		if errors.Is(err, services.ErrWorkflowRunNotFound) {
			return gen.GetWorkflowRunEvents404JSONResponse{NotFoundJSONResponse: notFound("WorkflowRun")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetWorkflowRunEvents403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrWorkflowRunReferenceNotFound) {
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

// toGenWorkflowRun converts models.WorkflowRunResponse to gen.WorkflowRun
func toGenWorkflowRun(r *models.WorkflowRunResponse) gen.WorkflowRun {
	if r == nil {
		return gen.WorkflowRun{}
	}

	result := gen.WorkflowRun{
		Name:         r.Name,
		OrgName:      r.NamespaceName,
		WorkflowName: r.WorkflowName,
		Status:       gen.WorkflowRunStatus(r.Status),
		CreatedAt:    r.CreatedAt,
	}

	if r.FinishedAt != nil {
		result.FinishedAt = r.FinishedAt
	}

	if r.Phase != "" {
		result.Phase = &r.Phase
	}

	if r.Parameters != nil {
		result.Parameters = &r.Parameters
	}

	if r.UUID != "" {
		result.Uuid = &r.UUID
	}

	return result
}

// normalizeStepPhase maps a raw phase string from the WorkflowTask CRD to a
// valid OpenAPI enum value. Argo's "Omitted" phase is mapped to "Skipped";
// any other unrecognized value falls back to "Error".
func normalizeStepPhase(phase string) gen.WorkflowStepStatusPhase {
	switch phase {
	case "Pending", "Running", "Succeeded", "Failed", "Skipped", "Error":
		return gen.WorkflowStepStatusPhase(phase)
	case "Omitted":
		return gen.Skipped
	default:
		return gen.Error
	}
}
