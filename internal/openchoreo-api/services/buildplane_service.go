// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// BuildPlaneService handles build plane-related business logic
type BuildPlaneService struct {
	k8sClient   client.Client
	bpClientMgr *kubernetesClient.KubeMultiClientManager
	logger      *slog.Logger
	authzPDP    authz.PDP
}

// NewBuildPlaneService creates a new build plane service
func NewBuildPlaneService(k8sClient client.Client, bpClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger, authzPDP authz.PDP) *BuildPlaneService {
	return &BuildPlaneService{
		k8sClient:   k8sClient,
		bpClientMgr: bpClientMgr,
		logger:      logger,
		authzPDP:    authzPDP,
	}
}

// getBuildPlane retrieves the build plane for an namespace without authorization checks (internal use only)
func (s *BuildPlaneService) getBuildPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.BuildPlane, error) {
	// List all build planes in the namespace namespace
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	err := s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(namespaceName))
	if err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "org", namespaceName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	// Check if any build planes exist
	if len(buildPlanes.Items) == 0 {
		s.logger.Warn("No build planes found", "org", namespaceName)
		return nil, fmt.Errorf("no build planes found for namespace: %s", namespaceName)
	}

	// Return the first build plane (0th index)
	buildPlane := &buildPlanes.Items[0]

	s.logger.Debug("Found build plane", "name", buildPlane.Name, "org", namespaceName)

	return buildPlane, nil
}

// GetBuildPlane retrieves the build plane for an namespace
func (s *BuildPlaneService) GetBuildPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.BuildPlane, error) {
	s.logger.Debug("Getting build plane", "org", namespaceName)
	buildPlane, err := s.getBuildPlane(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewBuildPlane, ResourceTypeBuildPlane, buildPlane.Name,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	return buildPlane, nil
}

// GetBuildPlaneClient creates and returns a Kubernetes client for the build plane cluster
// This method is deprecated and will be removed in a future version.
// Build plane operations should use the cluster gateway proxy instead.
func (s *BuildPlaneService) GetBuildPlaneClient(ctx context.Context, namespaceName string, gatewayURL string) (client.Client, error) {
	s.logger.Debug("Getting build plane client", "org", namespaceName)

	// Get the build plane first
	buildPlane, err := s.getBuildPlane(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	// Use cluster agent proxy mode
	buildPlaneClient, err := kubernetesClient.GetK8sClientFromBuildPlane(
		s.bpClientMgr,
		buildPlane,
		gatewayURL,
	)
	if err != nil {
		s.logger.Error("Failed to create build plane client", "error", err, "org", namespaceName)
		return nil, fmt.Errorf("failed to create build plane client: %w", err)
	}

	s.logger.Debug("Created build plane client", "org", namespaceName, "cluster", buildPlane.Name)
	return buildPlaneClient, nil
}

// ListBuildPlanes retrieves all build planes for an namespace
func (s *BuildPlaneService) ListBuildPlanes(ctx context.Context, namespaceName string) ([]models.BuildPlaneResponse, error) {
	s.logger.Debug("Listing build planes", "org", namespaceName)

	// List all build planes in the namespace namespace
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	err := s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(namespaceName))
	if err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "org", namespaceName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	s.logger.Debug("Found build planes", "count", len(buildPlanes.Items), "org", namespaceName)

	// Convert to response format
	buildPlaneResponses := make([]models.BuildPlaneResponse, 0, len(buildPlanes.Items))
	for i := range buildPlanes.Items {
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewBuildPlane, ResourceTypeBuildPlane, buildPlanes.Items[i].Name,
			authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				s.logger.Debug("Skipping unauthorized build plane", "org", namespaceName, "buildPlane", buildPlanes.Items[i].Name)
				continue
			}
			return nil, err
		}

		displayName := buildPlanes.Items[i].Annotations[controller.AnnotationKeyDisplayName]
		description := buildPlanes.Items[i].Annotations[controller.AnnotationKeyDescription]

		// Determine status from conditions
		status := ""

		// Extract observability plane reference if available
		observabilityPlaneRef := ""
		if buildPlanes.Items[i].Spec.ObservabilityPlaneRef != "" {
			observabilityPlaneRef = buildPlanes.Items[i].Spec.ObservabilityPlaneRef
		}

		buildPlaneResponse := models.BuildPlaneResponse{
			Name:                  buildPlanes.Items[i].Name,
			Namespace:             buildPlanes.Items[i].Namespace,
			DisplayName:           displayName,
			Description:           description,
			ObservabilityPlaneRef: observabilityPlaneRef,
			CreatedAt:             buildPlanes.Items[i].CreationTimestamp.Time,
			Status:                status,
		}

		buildPlaneResponses = append(buildPlaneResponses, buildPlaneResponse)
	}

	return buildPlaneResponses, nil
}
