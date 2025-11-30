// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"log/slog"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
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
	ObservabilityPlaneService *ObservabilityPlaneService
	GitHubWebhookService      *GitHubWebhookService
	k8sClient                 client.Client // Direct access to K8s client for apply operations
}

// NewServices creates and initializes all services
func NewServices(k8sClient client.Client, k8sBPClientMgr *kubernetesClient.KubeMultiClientManager, authzPAP authz.PAP, authzPDP authz.PDP, logger *slog.Logger) *Services {
	// Create project service
	projectService := NewProjectService(k8sClient, logger.With("service", "project"), authzPDP)

	// Initialize Git Provider from environment
	gitProvider := initializeGitProvider(logger)

	// Get webhook base URL from environment
	webhookBaseURL := os.Getenv("WEBHOOK_BASE_URL")
	if webhookBaseURL == "" {
		logger.Warn("WEBHOOK_BASE_URL not set, using default", "default", "http://localhost:8080")
		webhookBaseURL = "http://localhost:8080"
	}

	// Create build plane service with client manager for multi-cluster support
	buildPlaneService := NewBuildPlaneService(k8sClient, k8sBPClientMgr, logger.With("service", "buildplane"))

	// Create component workflow service
	componentWorkflowService := NewComponentWorkflowService(k8sClient, logger.With("service", "component-workflow"))

	// Create GitHub webhook service (depends on component workflow service and git provider)
	githubWebhookService := NewGitHubWebhookService(k8sClient, gitProvider, componentWorkflowService)

	// Create component service (depends on project service and git provider)
	componentService := NewComponentService(k8sClient, projectService, gitProvider, webhookBaseURL, logger.With("service", "component"), authzPDP)

	// Create organization service
	organizationService := NewOrganizationService(k8sClient, logger.With("service", "organization"), authzPDP)

	// Create environment service
	environmentService := NewEnvironmentService(k8sClient, logger.With("service", "environment"), authzPDP)

	// Create dataplane service
	dataplaneService := NewDataPlaneService(k8sClient, logger.With("service", "dataplane"), authzPDP)

	// Create build plane service with client manager for multi-cluster support
	buildPlaneService := NewBuildPlaneService(k8sClient, k8sBPClientMgr, logger.With("service", "buildplane"), authzPDP)

	// Create deployment pipeline service (depends on project service)
	deploymentPipelineService := NewDeploymentPipelineService(k8sClient, projectService, logger.With("service", "deployment-pipeline"), authzPDP)

	// Create ComponentType service
	componentTypeService := NewComponentTypeService(k8sClient, logger.With("service", "componenttype"), authzPDP)

	// Create Trait service
	traitService := NewTraitService(k8sClient, logger.With("service", "trait"), authzPDP)

	// Create Workflow service
	workflowService := NewWorkflowService(k8sClient, logger.With("service", "workflow"), authzPDP)

	// Create ComponentWorkflow service
	componentWorkflowService := NewComponentWorkflowService(k8sClient, logger.With("service", "componentworkflow"), authzPDP)

	// Create Schema service
	schemaService := NewSchemaService(k8sClient, logger.With("service", "schema"))

	// Create SecretReference service
	secretReferenceService := NewSecretReferenceService(k8sClient, logger.With("service", "secretreference"), authzPDP)

	// Create Authorization service
	authzService := NewAuthzService(authzPAP, authzPDP, logger.With("service", "authz"))

	// Create ObservabilityPlane service
	observabilityPlaneService := NewObservabilityPlaneService(k8sClient, logger.With("service", "observabilityplane"))

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
		ObservabilityPlaneService: observabilityPlaneService,
		GitHubWebhookService:      githubWebhookService,
		k8sClient:                 k8sClient,
	}
}

// initializeGitProvider initializes the git provider based on environment variables
func initializeGitProvider(logger *slog.Logger) git.Provider {
	gitProviderType := os.Getenv("GIT_PROVIDER")
	if gitProviderType == "" {
		gitProviderType = "github" // Default to GitHub
	}

	gitToken := os.Getenv("GIT_TOKEN")
	if gitToken == "" {
		logger.Warn("GIT_TOKEN not set, git provider will not be able to create webhooks")
	}

	gitBaseURL := os.Getenv("GIT_BASE_URL") // For GitHub Enterprise

	config := git.ProviderConfig{
		Token:   gitToken,
		BaseURL: gitBaseURL,
	}

	provider, err := git.GetProvider(git.ProviderType(gitProviderType), config)
	if err != nil {
		logger.Error("Failed to initialize git provider", "error", err, "type", gitProviderType)
		return nil
	}

	logger.Info("Git provider initialized", "type", gitProviderType)
	return provider
}

// GetKubernetesClient returns the Kubernetes client for direct API operations
func (s *Services) GetKubernetesClient() client.Client {
	return s.k8sClient
}
