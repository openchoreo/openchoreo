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

// ListComponentWorkflows is a stub - ComponentWorkflow has been merged into Workflow.
func (h *Handler) ListComponentWorkflows(
	_ context.Context,
	_ gen.ListComponentWorkflowsRequestObject,
) (gen.ListComponentWorkflowsResponseObject, error) {
	return gen.ListComponentWorkflows200JSONResponse{
		Items:      []gen.ComponentWorkflowTemplate{},
		Pagination: gen.Pagination{},
	}, nil
}

// GetComponentWorkflowSchema returns the parameter schema for a component workflow
func (h *Handler) GetComponentWorkflowSchema(
	_ context.Context,
	_ gen.GetComponentWorkflowSchemaRequestObject,
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
	component, err := h.legacyServices.ComponentService.UpdateComponentWorkflowParameters(
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

// ListComponentWorkflowRuns is a stub - ComponentWorkflowRun has been merged into WorkflowRun.
// Use ListWorkflowRuns with projectName/componentName query params instead.
func (h *Handler) ListComponentWorkflowRuns(
	_ context.Context,
	_ gen.ListComponentWorkflowRunsRequestObject,
) (gen.ListComponentWorkflowRunsResponseObject, error) {
	return gen.ListComponentWorkflowRuns200JSONResponse{
		Items:      []gen.ComponentWorkflowRun{},
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
	workflowRun, err := h.legacyServices.WorkflowRunService.TriggerWorkflow(
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

// GetComponentWorkflowRun is a stub - ComponentWorkflowRun has been merged into WorkflowRun.
// Use GetWorkflowRun instead.
func (h *Handler) GetComponentWorkflowRun(
	_ context.Context,
	_ gen.GetComponentWorkflowRunRequestObject,
) (gen.GetComponentWorkflowRunResponseObject, error) {
	return gen.GetComponentWorkflowRun404JSONResponse{
		NotFoundJSONResponse: notFound("ComponentWorkflowRun"),
	}, nil
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

	return result, nil
}
