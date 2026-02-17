// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
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
	h.logger.Info("UpdateComponentWorkflowParameters called",
		"namespace", request.NamespaceName,
		"project", request.ProjectName,
		"component", request.ComponentName)

	// Convert gen.UpdateComponentWorkflowRequest to models.UpdateComponentWorkflowRequest
	req, err := toModelsUpdateComponentWorkflowRequest(request.Body)
	if err != nil {
		h.logger.Error("Failed to convert request", "error", err)
		return gen.UpdateComponentWorkflowParameters400JSONResponse{
			BadRequestJSONResponse: badRequest("Invalid request body"),
		}, nil
	}

	// Call service to update workflow parameters
	component, err := h.services.ComponentService.UpdateComponentWorkflowParameters(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
		req,
	)
	if err != nil {
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.UpdateComponentWorkflowParameters404JSONResponse{
				NotFoundJSONResponse: notFound("Component"),
			}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.UpdateComponentWorkflowParameters404JSONResponse{
				NotFoundJSONResponse: notFound("Project"),
			}, nil
		}
		if errors.Is(err, services.ErrWorkflowSchemaInvalid) {
			return gen.UpdateComponentWorkflowParameters400JSONResponse{
				BadRequestJSONResponse: badRequest("Invalid workflow parameters"),
			}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateComponentWorkflowParameters403JSONResponse{
				ForbiddenJSONResponse: forbidden(),
			}, nil
		}
		h.logger.Error("Failed to update component workflow parameters", "error", err)
		return gen.UpdateComponentWorkflowParameters500JSONResponse{
			InternalErrorJSONResponse: internalError(),
		}, nil
	}

	return gen.UpdateComponentWorkflowParameters200JSONResponse(toGenComponent(component)), nil
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

// toModelsUpdateComponentWorkflowRequest converts gen.UpdateComponentWorkflowRequest to models.UpdateComponentWorkflowRequest
func toModelsUpdateComponentWorkflowRequest(req *gen.UpdateComponentWorkflowRequest) (*models.UpdateComponentWorkflowRequest, error) {
	if req == nil {
		return &models.UpdateComponentWorkflowRequest{}, nil
	}

	result := &models.UpdateComponentWorkflowRequest{}

	// Convert parameters if provided
	if req.Parameters != nil {
		// Marshal to JSON and unmarshal to runtime.RawExtension
		parametersJSON, err := json.Marshal(req.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal parameters: %w", err)
		}
		result.Parameters = &runtime.RawExtension{Raw: parametersJSON}
	}

	// Convert system parameters if provided
	if req.SystemParameters != nil && req.SystemParameters.Repository != nil {
		repo := req.SystemParameters.Repository
		result.SystemParameters = &models.ComponentWorkflowSystemParams{
			Repository: models.ComponentWorkflowRepository{},
		}

		if repo.Url != nil {
			result.SystemParameters.Repository.URL = *repo.Url
		}
		if repo.AppPath != nil {
			result.SystemParameters.Repository.AppPath = *repo.AppPath
		}
		if repo.Revision != nil {
			if repo.Revision.Branch != nil {
				result.SystemParameters.Repository.Revision.Branch = *repo.Revision.Branch
			}
			if repo.Revision.Commit != nil {
				result.SystemParameters.Repository.Revision.Commit = *repo.Revision.Commit
			}
		}
	}

	return result, nil
}
