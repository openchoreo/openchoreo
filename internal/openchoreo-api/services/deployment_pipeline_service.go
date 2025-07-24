// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// DeploymentPipelineService handles deployment pipeline-related business logic
type DeploymentPipelineService struct {
	k8sClient      client.Client
	projectService *ProjectService
	logger         *slog.Logger
}

// NewDeploymentPipelineService creates a new deployment pipeline service
func NewDeploymentPipelineService(k8sClient client.Client, projectService *ProjectService, logger *slog.Logger) *DeploymentPipelineService {
	return &DeploymentPipelineService{
		k8sClient:      k8sClient,
		projectService: projectService,
		logger:         logger,
	}
}

// GetProjectDeploymentPipeline retrieves the deployment pipeline for a given project
func (s *DeploymentPipelineService) GetProjectDeploymentPipeline(ctx context.Context, orgName, projectName string) (*models.DeploymentPipelineResponse, error) {
	s.logger.Debug("Getting project deployment pipeline", "org", orgName, "project", projectName)

	// First verify the project exists and get its deployment pipeline reference
	project, err := s.projectService.GetProject(ctx, orgName, projectName)
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
		pipelineName = "default"
		s.logger.Debug("No explicit deployment pipeline reference, using default", "pipeline", pipelineName)
	}

	// Get the deployment pipeline
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	key := client.ObjectKey{
		Name:      pipelineName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Deployment pipeline not found", "org", orgName, "project", projectName, "pipeline", pipelineName)
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
	var promotionPaths []models.PromotionPath
	for _, path := range pipeline.Spec.PromotionPaths {
		var targetRefs []models.TargetEnvironmentRef
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
	status := "Unknown"
	for _, condition := range pipeline.Status.Conditions {
		if condition.Type == "Ready" {
			if condition.Status == "True" {
				status = "Ready"
			} else {
				status = "NotReady"
			}
			break
		}
	}

	return &models.DeploymentPipelineResponse{
		Name:           pipeline.Name,
		DisplayName:    pipeline.Annotations[controller.AnnotationKeyDisplayName],
		Description:    pipeline.Annotations[controller.AnnotationKeyDescription],
		OrgName:        pipeline.Namespace,
		CreatedAt:      pipeline.CreationTimestamp.Time,
		Status:         status,
		PromotionPaths: promotionPaths,
	}
}
