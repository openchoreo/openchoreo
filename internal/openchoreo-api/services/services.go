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
	WorkflowRunService        *WorkflowRunService
	ComponentWorkflowService  *ComponentWorkflowService
	TraitService              *TraitService
	NamespaceService          *NamespaceService
	EnvironmentService        *EnvironmentService
	DataPlaneService          *DataPlaneService
	BuildPlaneService         *BuildPlaneService
	DeploymentPipelineService *DeploymentPipelineService
	SchemaService             *SchemaService
	SecretReferenceService    *SecretReferenceService
	GitSecretService          *GitSecretService
	WebhookService            *WebhookService
	AuthzService              *AuthzService
	ObservabilityPlaneService *ObservabilityPlaneService
	k8sClient                 client.Client // Direct access to K8s client for apply operations
}

// NewServices creates and initializes all services
func NewServices(k8sClient client.Client, k8sClientMgr *kubernetesClient.KubeMultiClientManager, authzPAP authz.PAP, authzPDP authz.PDP, logger *slog.Logger, gatewayURL string) *Services {
	// Create project service
	projectService := NewProjectService(k8sClient, logger.With("service", "project"), authzPDP)

	// Create component service (depends on project service)
	componentService := NewComponentService(k8sClient, projectService, logger.With("service", "component"), authzPDP)

	// Create namespace service
	namespaceService := NewNamespaceService(k8sClient, logger.With("service", "namespace"), authzPDP)

	// Create environment service
	environmentService := NewEnvironmentService(k8sClient, logger.With("service", "environment"), authzPDP)

	// Create dataplane service
	dataplaneService := NewDataPlaneService(k8sClient, logger.With("service", "dataplane"), authzPDP)

	// Create build plane service with client manager for multi-cluster support
	buildPlaneService := NewBuildPlaneService(k8sClient, k8sClientMgr, logger.With("service", "buildplane"), authzPDP)

	// Create deployment pipeline service (depends on project service)
	deploymentPipelineService := NewDeploymentPipelineService(k8sClient, projectService, logger.With("service", "deployment-pipeline"), authzPDP)

	// Create ComponentType service
	componentTypeService := NewComponentTypeService(k8sClient, logger.With("service", "componenttype"), authzPDP)

	// Create Trait service
	traitService := NewTraitService(k8sClient, logger.With("service", "trait"), authzPDP)

	// Create Workflow service
	workflowService := NewWorkflowService(k8sClient, logger.With("service", "workflow"), authzPDP)

	// Create WorkflowRun service
	workflowRunService := NewWorkflowRunService(k8sClient, logger.With("service", "workflowrun"), authzPDP)

	// Create ComponentWorkflow service
	componentWorkflowService := NewComponentWorkflowService(k8sClient, logger.With("service", "componentworkflow"), authzPDP, buildPlaneService)

	// Create webhook service (handles all git providers)
	webhookService := NewWebhookService(k8sClient, componentWorkflowService)

	// Create Schema service
	schemaService := NewSchemaService(k8sClient, logger.With("service", "schema"))

	// Create SecretReference service
	secretReferenceService := NewSecretReferenceService(k8sClient, logger.With("service", "secretreference"), authzPDP)

	// Create GitSecret service
	gitSecretService := NewGitSecretService(k8sClient, k8sClientMgr, buildPlaneService, logger.With("service", "gitsecret"), authzPDP, gatewayURL)

	// Create Authorization service
	authzService := NewAuthzService(authzPAP, authzPDP, logger.With("service", "authz"))

	// Create ObservabilityPlane service
	observabilityPlaneService := NewObservabilityPlaneService(k8sClient, logger.With("service", "observabilityplane"))

	return &Services{
		ProjectService:            projectService,
		ComponentService:          componentService,
		ComponentTypeService:      componentTypeService,
		WorkflowService:           workflowService,
		WorkflowRunService:        workflowRunService,
		ComponentWorkflowService:  componentWorkflowService,
		TraitService:              traitService,
		NamespaceService:          namespaceService,
		EnvironmentService:        environmentService,
		DataPlaneService:          dataplaneService,
		BuildPlaneService:         buildPlaneService,
		DeploymentPipelineService: deploymentPipelineService,
		SchemaService:             schemaService,
		SecretReferenceService:    secretReferenceService,
		GitSecretService:          gitSecretService,
		WebhookService:            webhookService,
		AuthzService:              authzService,
		ObservabilityPlaneService: observabilityPlaneService,
		k8sClient:                 k8sClient,
	}
}

// GetKubernetesClient returns the Kubernetes client for direct API operations
func (s *Services) GetKubernetesClient() client.Client {
	return s.k8sClient
}
