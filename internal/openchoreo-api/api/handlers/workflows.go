// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListWorkflows returns a list of generic workflows
func (h *Handler) ListWorkflows(
	ctx context.Context,
	request gen.ListWorkflowsRequestObject,
) (gen.ListWorkflowsResponseObject, error) {
	h.logger.Debug("ListWorkflows called", "namespaceName", request.NamespaceName)

	workflows, err := h.services.WorkflowService.ListWorkflows(ctx, request.NamespaceName)
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

	runs, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, request.NamespaceName)
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
	return nil, errNotImplemented
}

// GetWorkflowRun returns a specific workflow run
func (h *Handler) GetWorkflowRun(
	ctx context.Context,
	request gen.GetWorkflowRunRequestObject,
) (gen.GetWorkflowRunResponseObject, error) {
	return nil, errNotImplemented
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
