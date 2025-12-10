// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// BuildPlaneService handles build plane-related business logic
type BuildPlaneService struct {
	k8sClient   client.Client
	bpClientMgr *kubernetesClient.KubeMultiClientManager
	logger      *slog.Logger
}

// NewBuildPlaneService creates a new build plane service
func NewBuildPlaneService(k8sClient client.Client, bpClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger) *BuildPlaneService {
	return &BuildPlaneService{
		k8sClient:   k8sClient,
		bpClientMgr: bpClientMgr,
		logger:      logger,
	}
}

// GetBuildPlane retrieves the build plane for an organization
func (s *BuildPlaneService) GetBuildPlane(ctx context.Context, orgName string) (*openchoreov1alpha1.BuildPlane, error) {
	s.logger.Debug("Getting build plane", "org", orgName)

	// List all build planes in the organization namespace
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	err := s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(orgName))
	if err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "org", orgName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	// Check if any build planes exist
	if len(buildPlanes.Items) == 0 {
		s.logger.Warn("No build planes found", "org", orgName)
		return nil, fmt.Errorf("no build planes found for organization: %s", orgName)
	}

	// Return the first build plane (0th index)
	buildPlane := &buildPlanes.Items[0]
	s.logger.Debug("Found build plane", "name", buildPlane.Name, "org", orgName)

	return buildPlane, nil
}

// ListBuildPlanes retrieves all build planes for an organization
func (s *BuildPlaneService) ListBuildPlanes(ctx context.Context, orgName string) ([]models.BuildPlaneResponse, error) {
	s.logger.Debug("Listing build planes", "org", orgName)

	// List all build planes in the organization namespace
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	err := s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(orgName))
	if err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "org", orgName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	s.logger.Debug("Found build planes", "count", len(buildPlanes.Items), "org", orgName)

	// Convert to response format
	buildPlaneResponses := make([]models.BuildPlaneResponse, 0, len(buildPlanes.Items))
	for _, buildPlane := range buildPlanes.Items {
		displayName := buildPlane.Annotations[controller.AnnotationKeyDisplayName]
		description := buildPlane.Annotations[controller.AnnotationKeyDescription]

		// Determine status from conditions
		status := ""

		// Extract observability plane reference if available
		observabilityPlaneRef := ""
		if buildPlane.Spec.ObservabilityPlaneRef != "" {
			observabilityPlaneRef = buildPlane.Spec.ObservabilityPlaneRef
		}

		buildPlaneResponse := models.BuildPlaneResponse{
			Name:                  buildPlane.Name,
			Namespace:             buildPlane.Namespace,
			DisplayName:           displayName,
			Description:           description,
			KubernetesClusterName: buildPlane.Name,
			ObservabilityPlaneRef: observabilityPlaneRef,
			CreatedAt:             buildPlane.CreationTimestamp.Time,
			Status:                status,
		}

		buildPlaneResponses = append(buildPlaneResponses, buildPlaneResponse)
	}

	return buildPlaneResponses, nil
}
