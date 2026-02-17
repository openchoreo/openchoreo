// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacy_services

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	defaultPipeline = "default"
)

// DeploymentPipelineService handles deployment pipeline-related business logic
type DeploymentPipelineService struct {
	k8sClient      client.Client
	projectService *ProjectService
	logger         *slog.Logger
	authzPDP       authz.PDP
}

// NewDeploymentPipelineService creates a new deployment pipeline service
func NewDeploymentPipelineService(k8sClient client.Client, projectService *ProjectService, logger *slog.Logger, authzPDP authz.PDP) *DeploymentPipelineService {
	return &DeploymentPipelineService{
		k8sClient:      k8sClient,
		projectService: projectService,
		logger:         logger,
		authzPDP:       authzPDP,
	}
}

// GetProjectDeploymentPipeline retrieves the deployment pipeline for a given project
func (s *DeploymentPipelineService) GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error) {
	s.logger.Debug("Getting project deployment pipeline", "namespace", namespaceName, "project", projectName)

	// First verify the project exists and get its deployment pipeline reference
	project, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		return nil, err
	}

	var pipelineName string
	if project.DeploymentPipeline != "" {
		// Project has an explicit deployment pipeline reference
		pipelineName = project.DeploymentPipeline
		s.logger.Debug("Using explicit deployment pipeline reference", "pipeline", pipelineName)
	} else {
		// No explicit reference, look for default pipeline in the project's namespace
		pipelineName = defaultPipeline
		s.logger.Debug("No explicit deployment pipeline reference, using default", "pipeline", pipelineName)
	}

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewDeploymentPipeline, ResourceTypeDeploymentPipeline, pipelineName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName}); err != nil {
		return nil, err
	}

	// Get the deployment pipeline
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	key := client.ObjectKey{
		Name:      pipelineName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Deployment pipeline not found", "namespace", namespaceName, "project", projectName, "pipeline", pipelineName)
			return nil, ErrDeploymentPipelineNotFound
		}
		s.logger.Error("Failed to get deployment pipeline", "error", err)
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	return s.toDeploymentPipelineResponse(pipeline), nil
}

// toDeploymentPipelineResponse converts a DeploymentPipeline CR to a DeploymentPipelineResponse
func (s *DeploymentPipelineService) toDeploymentPipelineResponse(pipeline *openchoreov1alpha1.DeploymentPipeline) *models.DeploymentPipelineResponse {
	// Convert promotion paths
	promotionPaths := make([]models.PromotionPath, 0, len(pipeline.Spec.PromotionPaths))
	for _, path := range pipeline.Spec.PromotionPaths {
		targetRefs := make([]models.TargetEnvironmentRef, 0, len(path.TargetEnvironmentRefs))
		for _, target := range path.TargetEnvironmentRefs {
			targetRefs = append(targetRefs, models.TargetEnvironmentRef{
				Name:                     target.Name,
				RequiresApproval:         target.RequiresApproval,
				IsManualApprovalRequired: target.IsManualApprovalRequired,
			})
		}
		promotionPaths = append(promotionPaths, models.PromotionPath{
			SourceEnvironmentRef:  path.SourceEnvironmentRef,
			TargetEnvironmentRefs: targetRefs,
		})
	}

	// Determine status from conditions
	status := statusUnknown
	for _, condition := range pipeline.Status.Conditions {
		if condition.Type == statusReady {
			if condition.Status == "True" {
				status = statusReady
			} else {
				status = statusNotReady
			}
			break
		}
	}

	return &models.DeploymentPipelineResponse{
		Name:           pipeline.Name,
		DisplayName:    pipeline.Annotations[controller.AnnotationKeyDisplayName],
		Description:    pipeline.Annotations[controller.AnnotationKeyDescription],
		NamespaceName:  pipeline.Namespace,
		CreatedAt:      pipeline.CreationTimestamp.Time,
		Status:         status,
		PromotionPaths: promotionPaths,
	}
}
