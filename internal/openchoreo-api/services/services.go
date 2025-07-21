// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"golang.org/x/exp/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

type Services struct {
	ProjectService      *ProjectService
	ComponentService    *ComponentService
	OrganizationService *OrganizationService
	EnvironmentService  *EnvironmentService
	DataPlaneService    *DataPlaneService
	BuildService        *BuildService
	BuildPlaneService   *BuildPlaneService
	k8sClient           client.Client // Direct access to K8s client for apply operations
}

// NewServices creates and initializes all services
func NewServices(k8sClient client.Client, k8sBPClientMgr *kubernetesClient.KubeMultiClientManager, logger *slog.Logger) *Services {
	// Create project service
	projectService := NewProjectService(k8sClient, logger.With("service", "project"))

	// Create component service (depends on project service)
	componentService := NewComponentService(k8sClient, projectService, logger.With("service", "component"))

	// Create organization service
	organizationService := NewOrganizationService(k8sClient, logger.With("service", "organization"))

	// Create environment service
	environmentService := NewEnvironmentService(k8sClient, logger.With("service", "environment"))

	// Create dataplane service
	dataplaneService := NewDataPlaneService(k8sClient, logger.With("service", "dataplane"))

	// Create build plane service with client manager for multi-cluster support
	buildPlaneService := NewBuildPlaneService(k8sClient, k8sBPClientMgr, logger.With("service", "buildplane"))

	// Create build service (depends on build plane service)
	buildService := NewBuildService(k8sClient, buildPlaneService, k8sBPClientMgr, logger.With("service", "build"))

	return &Services{
		ProjectService:      projectService,
		ComponentService:    componentService,
		OrganizationService: organizationService,
		EnvironmentService:  environmentService,
		DataPlaneService:    dataplaneService,
		BuildService:        buildService,
		BuildPlaneService:   buildPlaneService,
		k8sClient:           k8sClient,
	}
}

// GetKubernetesClient returns the Kubernetes client for direct API operations
func (s *Services) GetKubernetesClient() client.Client {
	return s.k8sClient
}
