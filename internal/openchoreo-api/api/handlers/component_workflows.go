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

// ListComponentWorkflows returns a list of component workflows
func (h *Handler) ListComponentWorkflows(
	ctx context.Context,
	request gen.ListComponentWorkflowsRequestObject,
) (gen.ListComponentWorkflowsResponseObject, error) {
	h.logger.Debug("ListComponentWorkflows called", "namespaceName", request.NamespaceName)

	workflows, err := h.services.ComponentWorkflowService.ListComponentWorkflows(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list component workflows", "error", err)
		return gen.ListComponentWorkflows500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.ComponentWorkflowTemplate, 0, len(workflows))
	for _, wf := range workflows {
		items = append(items, toGenComponentWorkflowTemplate(wf))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListComponentWorkflows200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// GetComponentWorkflowSchema returns the parameter schema for a component workflow
func (h *Handler) GetComponentWorkflowSchema(
	ctx context.Context,
	request gen.GetComponentWorkflowSchemaRequestObject,
) (gen.GetComponentWorkflowSchemaResponseObject, error) {
	return nil, errNotImplemented
}

// UpdateComponentWorkflowParameters updates the workflow parameters for a component
func (h *Handler) UpdateComponentWorkflowParameters(
	ctx context.Context,
	request gen.UpdateComponentWorkflowParametersRequestObject,
) (gen.UpdateComponentWorkflowParametersResponseObject, error) {
	return nil, errNotImplemented
}

// ListComponentWorkflowRuns returns a list of workflow runs for a component
func (h *Handler) ListComponentWorkflowRuns(
	ctx context.Context,
	request gen.ListComponentWorkflowRunsRequestObject,
) (gen.ListComponentWorkflowRunsResponseObject, error) {
	h.logger.Debug("ListComponentWorkflowRuns called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName,
		"componentName", request.ComponentName)

	runs, err := h.services.ComponentWorkflowService.ListComponentWorkflowRuns(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
	)
	if err != nil {
		h.logger.Error("Failed to list component workflow runs", "error", err)
		return gen.ListComponentWorkflowRuns500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.ComponentWorkflowRun, 0, len(runs))
	for _, run := range runs {
		items = append(items, toGenComponentWorkflowRun(&run))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListComponentWorkflowRuns200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// CreateComponentWorkflowRun triggers a new workflow run for a component
func (h *Handler) CreateComponentWorkflowRun(
	ctx context.Context,
	request gen.CreateComponentWorkflowRunRequestObject,
) (gen.CreateComponentWorkflowRunResponseObject, error) {
	h.logger.Info("CreateComponentWorkflowRun called",
		"namespace", request.NamespaceName,
		"project", request.ProjectName,
		"component", request.ComponentName)

	// Extract commit from query params (defaults to empty string if not provided)
	commit := ""
	if request.Params.Commit != nil {
		commit = *request.Params.Commit
	}

	// Call service to trigger workflow
	workflowRun, err := h.services.ComponentWorkflowService.TriggerWorkflow(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
		commit,
	)
	if err != nil {
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.CreateComponentWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if errors.Is(err, services.ErrInvalidCommitSHA) {
			return gen.CreateComponentWorkflowRun400JSONResponse{BadRequestJSONResponse: badRequest("Invalid commit SHA format")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateComponentWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to create component workflow run", "error", err)
		return gen.CreateComponentWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.CreateComponentWorkflowRun201JSONResponse(toGenComponentWorkflowRun(workflowRun)), nil
}

// GetComponentWorkflowRun returns details of a specific workflow run
func (h *Handler) GetComponentWorkflowRun(
	ctx context.Context,
	request gen.GetComponentWorkflowRunRequestObject,
) (gen.GetComponentWorkflowRunResponseObject, error) {
	h.logger.Info("GetComponentWorkflowRun called",
		"namespace", request.NamespaceName,
		"project", request.ProjectName,
		"component", request.ComponentName,
		"runName", request.RunName)

	workflowRun, err := h.services.ComponentWorkflowService.GetComponentWorkflowRun(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
		request.RunName,
	)
	if err != nil {
		if errors.Is(err, services.ErrComponentWorkflowRunNotFound) {
			return gen.GetComponentWorkflowRun404JSONResponse{NotFoundJSONResponse: notFound("ComponentWorkflowRun")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetComponentWorkflowRun403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get component workflow run", "error", err)
		return gen.GetComponentWorkflowRun500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetComponentWorkflowRun200JSONResponse(toGenComponentWorkflowRun(workflowRun)), nil
}

// toGenComponentWorkflowTemplate converts models.WorkflowResponse to gen.ComponentWorkflowTemplate
func toGenComponentWorkflowTemplate(wf *models.WorkflowResponse) gen.ComponentWorkflowTemplate {
	return gen.ComponentWorkflowTemplate{
		Name:        wf.Name,
		DisplayName: ptr.To(wf.DisplayName),
		Description: ptr.To(wf.Description),
		CreatedAt:   wf.CreatedAt,
	}
}

// toGenComponentWorkflowRun converts models.ComponentWorkflowResponse to gen.ComponentWorkflowRun
func toGenComponentWorkflowRun(run *models.ComponentWorkflowResponse) gen.ComponentWorkflowRun {
	result := gen.ComponentWorkflowRun{
		Name:          run.Name,
		Uuid:          ptr.To(run.UUID),
		NamespaceName: run.NamespaceName,
		ProjectName:   run.ProjectName,
		ComponentName: run.ComponentName,
		CreatedAt:     run.CreatedAt,
	}
	if run.Commit != "" {
		result.Commit = ptr.To(run.Commit)
	}
	if run.Status != "" {
		result.Status = ptr.To(run.Status)
	}
	if run.Image != "" {
		result.Image = ptr.To(run.Image)
	}
	if run.Workflow != nil {
		workflow := gen.ComponentWorkflowConfig{
			Name: ptr.To(run.Workflow.Name),
		}
		if run.Workflow.SystemParameters != nil && run.Workflow.SystemParameters.Repository != nil {
			repo := run.Workflow.SystemParameters.Repository
			workflow.SystemParameters = &struct {
				Repository *struct {
					AppPath  *string `json:"appPath,omitempty"`
					Revision *struct {
						Branch *string `json:"branch,omitempty"`
						Commit *string `json:"commit,omitempty"`
					} `json:"revision,omitempty"`
					Url *string `json:"url,omitempty"` //nolint // OpenAPI generated model requires this
				} `json:"repository,omitempty"`
			}{
				Repository: &struct {
					AppPath  *string `json:"appPath,omitempty"`
					Revision *struct {
						Branch *string `json:"branch,omitempty"`
						Commit *string `json:"commit,omitempty"`
					} `json:"revision,omitempty"`
					Url *string `json:"url,omitempty"` //nolint // OpenAPI generated model requires this
				}{
					Url:     ptr.To(repo.URL),
					AppPath: ptr.To(repo.AppPath),
				},
			}
			if repo.Revision != nil {
				workflow.SystemParameters.Repository.Revision = &struct {
					Branch *string `json:"branch,omitempty"`
					Commit *string `json:"commit,omitempty"`
				}{
					Branch: ptr.To(repo.Revision.Branch),
				}
				if repo.Revision.Commit != "" {
					workflow.SystemParameters.Repository.Revision.Commit = ptr.To(repo.Revision.Commit)
				}
			}
		}
		if run.Workflow.Parameters != nil {
			workflow.Parameters = &run.Workflow.Parameters
		}
		result.Workflow = &workflow
	}
	return result
}
