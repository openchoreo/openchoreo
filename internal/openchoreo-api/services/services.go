// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
)

type Services struct {
	ProjectService            *ProjectService
	ComponentService          *ComponentService
	ComponentTypeService      *ComponentTypeService
	WorkflowService           *WorkflowService
	ComponentWorkflowService  *ComponentWorkflowService
	TraitService              *TraitService
	OrganizationService       *OrganizationService
	EnvironmentService        *EnvironmentService
	DataPlaneService          *DataPlaneService
	BuildPlaneService         *BuildPlaneService
	DeploymentPipelineService *DeploymentPipelineService
	SchemaService             *SchemaService
	SecretReferenceService    *SecretReferenceService
	AuthzService              *AuthzService
	k8sClient                 client.Client // Direct access to K8s client for apply operations
}

// NewServices creates and initializes all services
func NewServices(k8sClient client.Client, k8sBPClientMgr *kubernetesClient.KubeMultiClientManager, authzPAP authz.PAP, authzPDP authz.PDP, logger *slog.Logger) *Services {
	// Create project service
	projectService := NewProjectService(k8sClient, logger.With("service", "project"), authzPDP)

	// Create component service (depends on project service)
	componentService := NewComponentService(k8sClient, projectService, logger.With("service", "component"), authzPDP)

	// Create organization service
	organizationService := NewOrganizationService(k8sClient, logger.With("service", "organization"))

	// Create environment service
	environmentService := NewEnvironmentService(k8sClient, logger.With("service", "environment"))

	// Create dataplane service
	dataplaneService := NewDataPlaneService(k8sClient, logger.With("service", "dataplane"))

	// Create build plane service with client manager for multi-cluster support
	buildPlaneService := NewBuildPlaneService(k8sClient, k8sBPClientMgr, logger.With("service", "buildplane"))

	// Create deployment pipeline service (depends on project service)
	deploymentPipelineService := NewDeploymentPipelineService(k8sClient, projectService, logger.With("service", "deployment-pipeline"))

	// Create ComponentType service
	componentTypeService := NewComponentTypeService(k8sClient, logger.With("service", "componenttype"))

	// Create Trait service
	traitService := NewTraitService(k8sClient, logger.With("service", "trait"))

	// Create Workflow service
	workflowService := NewWorkflowService(k8sClient, logger.With("service", "workflow"))

	// Create ComponentWorkflow service
	componentWorkflowService := NewComponentWorkflowService(k8sClient, logger.With("service", "componentworkflow"))

	// Create Schema service
	schemaService := NewSchemaService(k8sClient, logger.With("service", "schema"))

	// Create SecretReference service
	secretReferenceService := NewSecretReferenceService(k8sClient, logger.With("service", "secretreference"))

	// Create Authorization service
	authzService := NewAuthzService(authzPAP, authzPDP, logger.With("service", "authz"))

	return &Services{
		ProjectService:            projectService,
		ComponentService:          componentService,
		ComponentTypeService:      componentTypeService,
		WorkflowService:           workflowService,
		ComponentWorkflowService:  componentWorkflowService,
		TraitService:              traitService,
		OrganizationService:       organizationService,
		EnvironmentService:        environmentService,
		DataPlaneService:          dataplaneService,
		BuildPlaneService:         buildPlaneService,
		DeploymentPipelineService: deploymentPipelineService,
		SchemaService:             schemaService,
		SecretReferenceService:    secretReferenceService,
		AuthzService:              authzService,
		k8sClient:                 k8sClient,
	}
}

// GetKubernetesClient returns the Kubernetes client for direct API operations
func (s *Services) GetKubernetesClient() client.Client {
	return s.k8sClient
}
