// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

// BuildPlaneService handles build plane-related business logic
type BuildPlaneService struct {
	k8sClient   client.Client
	BPClientMgr *kubernetesClient.KubeMultiClientManager
	logger      *slog.Logger
}

// NewBuildPlaneService creates a new build plane service
func NewBuildPlaneService(k8sClient client.Client, BPClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger) *BuildPlaneService {
	return &BuildPlaneService{
		k8sClient:   k8sClient,
		BPClientMgr: BPClientMgr,
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

// GetBuildPlaneClient creates and returns a Kubernetes client for the build plane cluster
func (s *BuildPlaneService) GetBuildPlaneClient(ctx context.Context, orgName string) (client.Client, error) {
	s.logger.Debug("Getting build plane client", "org", orgName)

	// Get the build plane first
	buildPlane, err := s.GetBuildPlane(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	buildPlaneClient, err := kubernetesClient.GetK8sClient(
		s.BPClientMgr,
		orgName,
		buildPlane.Spec.KubernetesCluster.Name,
		buildPlane.Spec.KubernetesCluster,
	)
	if err != nil {
		s.logger.Error("Failed to create build plane client", "error", err, "org", orgName)
		return nil, fmt.Errorf("failed to create build plane client: %w", err)
	}

	s.logger.Debug("Created build plane client", "org", orgName, "cluster", buildPlane.Spec.KubernetesCluster.Name)
	return buildPlaneClient, nil
}
