// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// GetProjectDeploymentPipeline returns the deployment pipeline for a project
func (h *Handler) GetProjectDeploymentPipeline(
	ctx context.Context,
	request gen.GetProjectDeploymentPipelineRequestObject,
) (gen.GetProjectDeploymentPipelineResponseObject, error) {
	h.logger.Info("GetProjectDeploymentPipeline called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName)

	pipeline, err := h.services.DeploymentPipelineService.GetProjectDeploymentPipeline(
		ctx,
		request.NamespaceName,
		request.ProjectName,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetProjectDeploymentPipeline403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.GetProjectDeploymentPipeline404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrDeploymentPipelineNotFound) {
			return gen.GetProjectDeploymentPipeline404JSONResponse{NotFoundJSONResponse: notFound("DeploymentPipeline")}, nil
		}
		h.logger.Error("Failed to get project deployment pipeline", "error", err)
		return gen.GetProjectDeploymentPipeline500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetProjectDeploymentPipeline200JSONResponse(toGenDeploymentPipeline(pipeline)), nil
}

// toGenDeploymentPipeline converts models.DeploymentPipelineResponse to gen.DeploymentPipeline
func toGenDeploymentPipeline(p *models.DeploymentPipelineResponse) gen.DeploymentPipeline {
	if p == nil {
		return gen.DeploymentPipeline{}
	}

	metadata := gen.ObjectMeta{
		Name:              p.Name,
		Namespace:         &p.NamespaceName,
		CreationTimestamp: &p.CreatedAt,
	}

	var promotionPaths *[]gen.PromotionPath
	if len(p.PromotionPaths) > 0 {
		paths := make([]gen.PromotionPath, 0, len(p.PromotionPaths))
		for _, path := range p.PromotionPaths {
			genPath := gen.PromotionPath{
				SourceEnvironmentRef: path.SourceEnvironmentRef,
			}

			if len(path.TargetEnvironmentRefs) > 0 {
				targetRefs := make([]gen.TargetEnvironmentRef, 0, len(path.TargetEnvironmentRefs))
				for _, target := range path.TargetEnvironmentRefs {
					targetRefs = append(targetRefs, gen.TargetEnvironmentRef{
						Name:                     target.Name,
						RequiresApproval:         &target.RequiresApproval,
						IsManualApprovalRequired: &target.IsManualApprovalRequired,
					})
				}
				genPath.TargetEnvironmentRefs = targetRefs
			}

			paths = append(paths, genPath)
		}
		promotionPaths = &paths
	}

	spec := &gen.DeploymentPipelineSpec{
		PromotionPaths: promotionPaths,
	}

	return gen.DeploymentPipeline{
		Metadata: metadata,
		Spec:     spec,
	}
}
