// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	deploymentpipelinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline"
)

// ListDeploymentPipelines returns a paginated list of deployment pipelines within a namespace.
func (h *Handler) ListDeploymentPipelines(
	ctx context.Context,
	request gen.ListDeploymentPipelinesRequestObject,
) (gen.ListDeploymentPipelinesResponseObject, error) {
	h.logger.Debug("ListDeploymentPipelines called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.DeploymentPipelineService.ListDeploymentPipelines(ctx, request.NamespaceName, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListDeploymentPipelines403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list deployment pipelines", "error", err)
		return gen.ListDeploymentPipelines500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.DeploymentPipeline, gen.DeploymentPipeline](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert deployment pipelines", "error", err)
		return gen.ListDeploymentPipelines500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListDeploymentPipelines200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateDeploymentPipeline creates a new deployment pipeline within a namespace.
func (h *Handler) CreateDeploymentPipeline(
	ctx context.Context,
	request gen.CreateDeploymentPipelineRequestObject,
) (gen.CreateDeploymentPipelineResponseObject, error) {
	h.logger.Info("CreateDeploymentPipeline called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateDeploymentPipeline400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	dpCR, err := convert[gen.DeploymentPipeline, openchoreov1alpha1.DeploymentPipeline](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateDeploymentPipeline400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	dpCR.Status = openchoreov1alpha1.DeploymentPipelineStatus{}

	created, err := h.services.DeploymentPipelineService.CreateDeploymentPipeline(ctx, request.NamespaceName, &dpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateDeploymentPipeline403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, deploymentpipelinesvc.ErrDeploymentPipelineAlreadyExists) {
			return gen.CreateDeploymentPipeline409JSONResponse{ConflictJSONResponse: conflict("Deployment pipeline already exists")}, nil
		}
		h.logger.Error("Failed to create deployment pipeline", "error", err)
		return gen.CreateDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genDP, err := convert[openchoreov1alpha1.DeploymentPipeline, gen.DeploymentPipeline](*created)
	if err != nil {
		h.logger.Error("Failed to convert created deployment pipeline", "error", err)
		return gen.CreateDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Deployment pipeline created successfully", "namespaceName", request.NamespaceName, "deploymentPipeline", created.Name)
	return gen.CreateDeploymentPipeline201JSONResponse(genDP), nil
}

// GetDeploymentPipeline returns details of a specific deployment pipeline.
func (h *Handler) GetDeploymentPipeline(
	ctx context.Context,
	request gen.GetDeploymentPipelineRequestObject,
) (gen.GetDeploymentPipelineResponseObject, error) {
	h.logger.Debug("GetDeploymentPipeline called", "namespaceName", request.NamespaceName, "deploymentPipelineName", request.DeploymentPipelineName)

	dp, err := h.services.DeploymentPipelineService.GetDeploymentPipeline(ctx, request.NamespaceName, request.DeploymentPipelineName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetDeploymentPipeline403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, deploymentpipelinesvc.ErrDeploymentPipelineNotFound) {
			return gen.GetDeploymentPipeline404JSONResponse{NotFoundJSONResponse: notFound("DeploymentPipeline")}, nil
		}
		h.logger.Error("Failed to get deployment pipeline", "error", err)
		return gen.GetDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genDP, err := convert[openchoreov1alpha1.DeploymentPipeline, gen.DeploymentPipeline](*dp)
	if err != nil {
		h.logger.Error("Failed to convert deployment pipeline", "error", err)
		return gen.GetDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetDeploymentPipeline200JSONResponse(genDP), nil
}

// UpdateDeploymentPipeline replaces an existing deployment pipeline (full update).
func (h *Handler) UpdateDeploymentPipeline(
	ctx context.Context,
	request gen.UpdateDeploymentPipelineRequestObject,
) (gen.UpdateDeploymentPipelineResponseObject, error) {
	h.logger.Info("UpdateDeploymentPipeline called", "namespaceName", request.NamespaceName, "deploymentPipelineName", request.DeploymentPipelineName)

	if request.Body == nil {
		return gen.UpdateDeploymentPipeline400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	dpCR, err := convert[gen.DeploymentPipeline, openchoreov1alpha1.DeploymentPipeline](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateDeploymentPipeline400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	dpCR.Status = openchoreov1alpha1.DeploymentPipelineStatus{}

	// Ensure the name from the URL path is used
	dpCR.Name = request.DeploymentPipelineName

	updated, err := h.services.DeploymentPipelineService.UpdateDeploymentPipeline(ctx, request.NamespaceName, &dpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateDeploymentPipeline403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, deploymentpipelinesvc.ErrDeploymentPipelineNotFound) {
			return gen.UpdateDeploymentPipeline404JSONResponse{NotFoundJSONResponse: notFound("DeploymentPipeline")}, nil
		}
		h.logger.Error("Failed to update deployment pipeline", "error", err)
		return gen.UpdateDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genDP, err := convert[openchoreov1alpha1.DeploymentPipeline, gen.DeploymentPipeline](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated deployment pipeline", "error", err)
		return gen.UpdateDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Deployment pipeline updated successfully", "namespaceName", request.NamespaceName, "deploymentPipeline", updated.Name)
	return gen.UpdateDeploymentPipeline200JSONResponse(genDP), nil
}

// DeleteDeploymentPipeline deletes a deployment pipeline by name.
func (h *Handler) DeleteDeploymentPipeline(
	ctx context.Context,
	request gen.DeleteDeploymentPipelineRequestObject,
) (gen.DeleteDeploymentPipelineResponseObject, error) {
	h.logger.Info("DeleteDeploymentPipeline called", "namespaceName", request.NamespaceName, "deploymentPipelineName", request.DeploymentPipelineName)

	err := h.services.DeploymentPipelineService.DeleteDeploymentPipeline(ctx, request.NamespaceName, request.DeploymentPipelineName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteDeploymentPipeline403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, deploymentpipelinesvc.ErrDeploymentPipelineNotFound) {
			return gen.DeleteDeploymentPipeline404JSONResponse{NotFoundJSONResponse: notFound("DeploymentPipeline")}, nil
		}
		h.logger.Error("Failed to delete deployment pipeline", "error", err)
		return gen.DeleteDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Deployment pipeline deleted successfully", "namespaceName", request.NamespaceName, "deploymentPipeline", request.DeploymentPipelineName)
	return gen.DeleteDeploymentPipeline204Response{}, nil
}
