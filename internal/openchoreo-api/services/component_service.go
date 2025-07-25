// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	"golang.org/x/exp/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ComponentService handles component-related business logic
type ComponentService struct {
	k8sClient           client.Client
	projectService      *ProjectService
	specFetcherRegistry *ComponentSpecFetcherRegistry
	logger              *slog.Logger
}

type PromoteComponentPayload struct {
	models.PromoteComponentRequest
	ComponentName string `json:"componentName"`
	ProjectName   string `json:"projectName"`
	OrgName       string `json:"orgName"`
}

// NewComponentService creates a new component service
func NewComponentService(k8sClient client.Client, projectService *ProjectService, logger *slog.Logger) *ComponentService {
	return &ComponentService{
		k8sClient:           k8sClient,
		projectService:      projectService,
		specFetcherRegistry: NewComponentSpecFetcherRegistry(),
		logger:              logger,
	}
}

// CreateComponent creates a new component in the given project
func (s *ComponentService) CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Creating component", "org", orgName, "project", projectName, "component", req.Name)

	// Sanitize input
	req.Sanitize()

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if err == ErrProjectNotFound {
			s.logger.Warn("Project not found", "org", orgName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Check if component already exists
	exists, err := s.componentExists(ctx, orgName, projectName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if exists {
		s.logger.Warn("Component already exists", "org", orgName, "project", projectName, "component", req.Name)
		return nil, ErrComponentAlreadyExists
	}

	// Create the component and related resources
	if err := s.createComponentResources(ctx, orgName, projectName, req); err != nil {
		s.logger.Error("Failed to create component resources", "error", err)
		return nil, fmt.Errorf("failed to create component: %w", err)
	}

	s.logger.Debug("Component created successfully", "org", orgName, "project", projectName, "component", req.Name)

	// Return the created component
	return &models.ComponentResponse{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Type:        req.Type,
		ProjectName: projectName,
		OrgName:     orgName,
		CreatedAt:   metav1.Now().Time,
		Status:      "Creating",
	}, nil
}

// ListComponents lists all components in the given project
func (s *ComponentService) ListComponents(ctx context.Context, orgName, projectName string) ([]*models.ComponentResponse, error) {
	s.logger.Debug("Listing components", "org", orgName, "project", projectName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if err == ErrProjectNotFound {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	var componentList openchoreov1alpha1.ComponentV2List
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &componentList, listOpts...); err != nil {
		s.logger.Error("Failed to list components", "error", err)
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	var components []*models.ComponentResponse
	for _, item := range componentList.Items {
		// Only include components that belong to the specified project
		if item.Spec.Owner.ProjectName == projectName {
			components = append(components, s.toComponentResponse(&item, make(map[string]interface{})))
		}
	}

	s.logger.Debug("Listed components", "org", orgName, "project", projectName, "count", len(components))
	return components, nil
}

// GetComponent retrieves a specific component
func (s *ComponentService) GetComponent(ctx context.Context, orgName, projectName, componentName string, additionalResources []string) (*models.ComponentResponse, error) {
	s.logger.Debug("Getting component", "org", orgName, "project", projectName, "component", componentName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if err == ErrProjectNotFound {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	component := &openchoreov1alpha1.ComponentV2{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Get Workload and Type optionally
	typeSpecs := make(map[string]interface{})
	validResourceTypes := map[string]bool{"type": true, "workload": true}

	for _, resourceType := range additionalResources {
		if !validResourceTypes[resourceType] {
			s.logger.Warn("Invalid resource type requested", "resourceType", resourceType, "component", componentName)
			continue
		}

		var fetcherKey string
		switch resourceType {
		case "type":
			fetcherKey = string(component.Spec.Type)
		case "workload":
			fetcherKey = "Workload"
		default:
			s.logger.Warn("Unknown resource type requested", "resourceType", resourceType, "component", componentName)
			continue
		}

		fetcher, exists := s.specFetcherRegistry.GetFetcher(fetcherKey)
		if !exists {
			s.logger.Warn("No fetcher registered for resource type", "fetcherKey", fetcherKey, "component", componentName)
			continue
		}

		spec, err := fetcher.FetchSpec(ctx, s.k8sClient, orgName, componentName)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn(
					"Resource not found for fetcher",
					"fetcherKey", fetcherKey,
					"org", orgName,
					"project", projectName,
					"component", componentName,
				)
			} else {
				s.logger.Error(
					"Failed to fetch spec for resource type",
					"fetcherKey", fetcherKey,
					"org", orgName,
					"project", projectName,
					"component", componentName,
					"error", err,
				)
			}
			continue
		}
		typeSpecs[resourceType] = spec
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "org", orgName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	return s.toComponentResponse(component, typeSpecs), nil
}

// componentExists checks if a component already exists by name and namespace and belongs to the specified project
func (s *ComponentService) componentExists(ctx context.Context, orgName, projectName, componentName string) (bool, error) {
	component := &openchoreov1alpha1.ComponentV2{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}

	err := s.k8sClient.Get(ctx, key, component)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil // Not found, so doesn't exist
		}
		return false, fmt.Errorf("failed to check component existence: %w", err) // Some other error
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		return false, nil // Component exists but belongs to a different project
	}

	return true, nil // Found and belongs to the correct project
}

// createComponentResources creates the component and related Kubernetes resources
func (s *ComponentService) createComponentResources(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) error {
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}

	annotations := map[string]string{
		controller.AnnotationKeyDisplayName: displayName,
		controller.AnnotationKeyDescription: req.Description,
	}

	componentCR := &openchoreov1alpha1.ComponentV2{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   orgName,
			Annotations: annotations,
		},
		Spec: openchoreov1alpha1.ComponentV2Spec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: projectName,
			},
			Type: openchoreov1alpha1.ComponentType(req.Type),
		},
	}

	// Only add build configuration if it's provided in the request
	if req.BuildConfig.RepoUrl != "" {
		// Convert template parameters from request format to Kubernetes format
		var parameters []openchoreov1alpha1.Parameter
		for _, param := range req.BuildConfig.TemplateParams {
			parameters = append(parameters, openchoreov1alpha1.Parameter{
				Name:  param.Name,
				Value: param.Value,
			})
		}

		componentCR.Spec.Build = openchoreov1alpha1.BuildSpecInComponent{
			Repository: openchoreov1alpha1.BuildRepository{
				URL: req.BuildConfig.RepoUrl,
				Revision: openchoreov1alpha1.BuildRevision{
					Branch: req.BuildConfig.Branch,
				},
				AppPath: req.BuildConfig.ComponentPath,
			},
			TemplateRef: openchoreov1alpha1.TemplateRef{
				Name:       req.BuildConfig.BuildTemplateRef,
				Parameters: parameters,
			},
		}
	}

	if err := s.k8sClient.Create(ctx, componentCR); err != nil {
		return fmt.Errorf("failed to create component CR: %w", err)
	}

	return nil
}

// toComponentResponse converts a ComponentV2 CR to a ComponentResponse
func (s *ComponentService) toComponentResponse(component *openchoreov1alpha1.ComponentV2, typeSpecs map[string]interface{}) *models.ComponentResponse {
	// Extract project name from the component owner
	projectName := component.Spec.Owner.ProjectName

	// Get status - ComponentV2 doesn't have conditions yet, so default to Creating
	// This can be enhanced later when ComponentV2 adds status conditions
	status := "Creating"

	// Convert template parameters from Kubernetes format to response format
	var templateParams []models.TemplateParameter
	for _, param := range component.Spec.Build.TemplateRef.Parameters {
		templateParams = append(templateParams, models.TemplateParameter{
			Name:  param.Name,
			Value: param.Value,
		})
	}

	response := &models.ComponentResponse{
		Name:        component.Name,
		DisplayName: component.Annotations[controller.AnnotationKeyDisplayName],
		Description: component.Annotations[controller.AnnotationKeyDescription],
		Type:        string(component.Spec.Type),
		ProjectName: projectName,
		OrgName:     component.Namespace,
		CreatedAt:   component.CreationTimestamp.Time,
		Status:      status,
		BuildConfig: &models.BuildConfig{
			RepoUrl:          component.Spec.Build.Repository.URL,
			Branch:           component.Spec.Build.Repository.Revision.Branch,
			ComponentPath:    component.Spec.Build.Repository.AppPath,
			BuildTemplateRef: component.Spec.Build.TemplateRef.Name,
			TemplateParams:   templateParams,
		},
	}

	for _, v := range typeSpecs {
		switch spec := v.(type) {
		case *openchoreov1alpha1.WorkloadSpec:
			response.Workload = spec
		case *openchoreov1alpha1.ServiceSpec:
			response.Service = spec
		case *openchoreov1alpha1.WebApplicationSpec:
			response.WebApplication = spec
		default:
			s.logger.Error("Unknown type in typeSpecs", "component", component.Name, "actualType", fmt.Sprintf("%T", v))
		}
	}

	return response
}

// GetComponentBindings retrieves bindings for a component in multiple environments
// If environments is empty, it will get all environments from the project's deployment pipeline
func (s *ComponentService) GetComponentBindings(ctx context.Context, orgName, projectName, componentName string, environments []string) ([]*models.BindingResponse, error) {
	s.logger.Debug("Getting component bindings", "org", orgName, "project", projectName, "component", componentName, "environments", environments)

	// First get the component to determine its type
	component, err := s.GetComponent(ctx, orgName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	// If no environments specified, get all environments from the deployment pipeline
	if len(environments) == 0 {
		pipelineEnvironments, err := s.getEnvironmentsFromDeploymentPipeline(ctx, orgName, projectName)
		if err != nil {
			return nil, err
		}
		environments = pipelineEnvironments
		s.logger.Debug("Using environments from deployment pipeline", "environments", environments)
	}

	var bindings []*models.BindingResponse
	for _, environment := range environments {
		binding, err := s.getComponentBinding(ctx, orgName, projectName, componentName, environment, component.Type)
		if err != nil {
			// If binding not found for an environment, skip it rather than failing the entire request
			if err == ErrBindingNotFound {
				s.logger.Debug("Binding not found for environment", "environment", environment)
				continue
			}
			return nil, err
		}
		bindings = append(bindings, binding)
	}

	return bindings, nil
}

// GetComponentBinding retrieves the binding for a component in a specific environment
func (s *ComponentService) GetComponentBinding(ctx context.Context, orgName, projectName, componentName, environment string) (*models.BindingResponse, error) {
	s.logger.Debug("Getting component binding", "org", orgName, "project", projectName, "component", componentName, "environment", environment)

	// First get the component to determine its type
	component, err := s.GetComponent(ctx, orgName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	return s.getComponentBinding(ctx, orgName, projectName, componentName, environment, component.Type)
}

// getComponentBinding retrieves the binding for a component in a specific environment
func (s *ComponentService) getComponentBinding(ctx context.Context, orgName, projectName, componentName, environment, componentType string) (*models.BindingResponse, error) {
	// Determine binding type based on component type
	var bindingResponse *models.BindingResponse
	var err error
	switch openchoreov1alpha1.ComponentType(componentType) {
	case openchoreov1alpha1.ComponentTypeService:
		bindingResponse, err = s.getServiceBinding(ctx, orgName, componentName, environment)
	case openchoreov1alpha1.ComponentTypeWebApplication:
		bindingResponse, err = s.getWebApplicationBinding(ctx, orgName, componentName, environment)
	case openchoreov1alpha1.ComponentTypeScheduledTask:
		bindingResponse, err = s.getScheduledTaskBinding(ctx, orgName, componentName, environment)
	default:
		return nil, fmt.Errorf("unsupported component type: %s", componentType)
	}

	if err != nil {
		return nil, err
	}

	// Populate common fields
	bindingResponse.ComponentName = componentName
	bindingResponse.ProjectName = projectName
	bindingResponse.OrgName = orgName
	bindingResponse.Environment = environment

	return bindingResponse, nil
}

// getServiceBinding retrieves a ServiceBinding from the cluster
func (s *ComponentService) getServiceBinding(ctx context.Context, orgName, componentName, environment string) (*models.BindingResponse, error) {
	// Use the reusable CR method to get the ServiceBinding
	binding, err := s.getServiceBindingCR(ctx, orgName, componentName, environment)
	if err != nil {
		return nil, err
	}

	// Convert to response model
	response := &models.BindingResponse{
		Name: binding.Name,
		Type: "Service",
		BindingStatus: models.BindingStatus{
			Status:  models.BindingStatusTypeInProgress, // Default to "NotYetDeployed"
			Reason:  "",
			Message: "",
		},
	}

	// Extract status from conditions and map to UI-friendly status
	for _, condition := range binding.Status.Conditions {
		if condition.Type == "Ready" {
			response.BindingStatus.Reason = condition.Reason
			response.BindingStatus.Message = condition.Message
			response.BindingStatus.LastTransitioned = condition.LastTransitionTime.Time

			// Map condition status and reason to UI-friendly status
			response.BindingStatus.Status = s.mapConditionToBindingStatus(condition)
			break
		}
	}

	// Convert endpoint status and extract image
	serviceBinding := &models.ServiceBinding{
		Endpoints: s.convertEndpointStatus(binding.Status.Endpoints),
		Image:     s.extractImageFromWorkloadSpec(binding.Spec.WorkloadSpec),
	}
	response.ServiceBinding = serviceBinding

	return response, nil
}

// getWebApplicationBinding retrieves a WebApplicationBinding from the cluster
func (s *ComponentService) getWebApplicationBinding(ctx context.Context, orgName, componentName, environment string) (*models.BindingResponse, error) {
	// Use the reusable CR method to get the WebApplicationBinding
	binding, err := s.getWebApplicationBindingCR(ctx, orgName, componentName, environment)
	if err != nil {
		return nil, err
	}

	// Convert to response model
	response := &models.BindingResponse{
		Name: binding.Name,
		Type: "WebApplication",
		BindingStatus: models.BindingStatus{
			Status:  models.BindingStatusTypeInProgress, // Default to "NotYetDeployed"
			Reason:  "",
			Message: "",
		},
	}

	// Extract status from conditions and map to UI-friendly status
	for _, condition := range binding.Status.Conditions {
		if condition.Type == "Ready" {
			response.BindingStatus.Reason = condition.Reason
			response.BindingStatus.Message = condition.Message
			response.BindingStatus.LastTransitioned = condition.LastTransitionTime.Time

			// Map condition status and reason to UI-friendly status
			response.BindingStatus.Status = s.mapConditionToBindingStatus(condition)
			break
		}
	}

	// Convert endpoint status and extract image
	webAppBinding := &models.WebApplicationBinding{
		Endpoints: s.convertEndpointStatus(binding.Status.Endpoints),
		Image:     s.extractImageFromWorkloadSpec(binding.Spec.WorkloadSpec),
	}
	response.WebApplicationBinding = webAppBinding

	return response, nil
}

// getScheduledTaskBinding retrieves a ScheduledTaskBinding from the cluster
func (s *ComponentService) getScheduledTaskBinding(ctx context.Context, orgName, componentName, environment string) (*models.BindingResponse, error) {
	// Use the reusable CR method to get the ScheduledTaskBinding
	binding, err := s.getScheduledTaskBindingCR(ctx, orgName, componentName, environment)
	if err != nil {
		return nil, err
	}

	// Convert to response model
	response := &models.BindingResponse{
		Name: binding.Name,
		Type: "ScheduledTask",
		BindingStatus: models.BindingStatus{
			Status:  models.BindingStatusTypeInProgress, // Default to "NotYetDeployed"
			Reason:  "",
			Message: "",
		},
	}

	// TODO: ScheduledTaskBinding doesn't have conditions in its status yet
	// When conditions are added, implement the same status mapping logic as Service and WebApplication bindings
	// For now, default to NotYetDeployed status
	response.BindingStatus.Status = models.BindingStatusTypeInProgress

	// ScheduledTaskBinding doesn't have endpoints, but we still extract the image
	response.ScheduledTaskBinding = &models.ScheduledTaskBinding{
		Image: s.extractImageFromWorkloadSpec(binding.Spec.WorkloadSpec),
	}

	return response, nil
}

// convertEndpointStatus converts from Kubernetes endpoint status to API response model
func (s *ComponentService) convertEndpointStatus(endpoints []openchoreov1alpha1.EndpointStatus) []models.EndpointStatus {
	var result []models.EndpointStatus

	for _, ep := range endpoints {
		endpointStatus := models.EndpointStatus{
			Name: ep.Name,
			Type: string(ep.Type),
		}

		// Convert each visibility level
		if ep.Project != nil {
			endpointStatus.Project = &models.ExposedEndpoint{
				Host:     ep.Project.Host,
				Port:     int(ep.Project.Port),
				Scheme:   ep.Project.Scheme,
				BasePath: ep.Project.BasePath,
				URI:      ep.Project.URI,
			}
		}

		if ep.Organization != nil {
			endpointStatus.Organization = &models.ExposedEndpoint{
				Host:     ep.Organization.Host,
				Port:     int(ep.Organization.Port),
				Scheme:   ep.Organization.Scheme,
				BasePath: ep.Organization.BasePath,
				URI:      ep.Organization.URI,
			}
		}

		if ep.Public != nil {
			endpointStatus.Public = &models.ExposedEndpoint{
				Host:     ep.Public.Host,
				Port:     int(ep.Public.Port),
				Scheme:   ep.Public.Scheme,
				BasePath: ep.Public.BasePath,
				URI:      ep.Public.URI,
			}
		}

		result = append(result, endpointStatus)
	}

	return result
}

// getEnvironmentsFromDeploymentPipeline extracts all environments from the project's deployment pipeline
func (s *ComponentService) getEnvironmentsFromDeploymentPipeline(ctx context.Context, orgName, projectName string) ([]string, error) {
	// Get the project to determine the deployment pipeline reference
	project, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		return nil, err
	}

	var pipelineName string
	if project.DeploymentPipeline != "" {
		pipelineName = project.DeploymentPipeline
	} else {
		pipelineName = "default"
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
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Extract unique environments from promotion paths
	environmentSet := make(map[string]bool)
	for _, path := range pipeline.Spec.PromotionPaths {
		// Add source environment
		environmentSet[path.SourceEnvironmentRef] = true

		// Add target environments
		for _, target := range path.TargetEnvironmentRefs {
			environmentSet[target.Name] = true
		}
	}

	// Convert set to slice
	var environments []string
	for env := range environmentSet {
		environments = append(environments, env)
	}

	s.logger.Debug("Extracted environments from deployment pipeline", "pipeline", pipelineName, "environments", environments)
	return environments, nil
}

// PromoteComponent promotes a component from source environment to target environment
func (s *ComponentService) PromoteComponent(ctx context.Context, req *PromoteComponentPayload) ([]*models.BindingResponse, error) {
	s.logger.Debug("Promoting component", "org", req.OrgName, "project", req.ProjectName, "component", req.ComponentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment)

	// Validate that the promotion path is allowed by the deployment pipeline
	if err := s.validatePromotionPath(ctx, req.OrgName, req.ProjectName, req.SourceEnvironment, req.TargetEnvironment); err != nil {
		return nil, err
	}

	// Get the component to determine its type
	component, err := s.GetComponent(ctx, req.OrgName, req.ProjectName, req.ComponentName, []string{})
	if err != nil {
		return nil, err
	}

	// Create or update the target binding
	if err := s.createOrUpdateTargetBinding(ctx, req, component.Type); err != nil {
		return nil, fmt.Errorf("failed to create target binding: %w", err)
	}

	// Return all bindings for the component after promotion
	allEnvironments, err := s.getEnvironmentsFromDeploymentPipeline(ctx, req.OrgName, req.ProjectName)
	if err != nil {
		s.logger.Warn("Failed to get environments from deployment pipeline, returning empty list", "error", err)
		allEnvironments = []string{req.SourceEnvironment, req.TargetEnvironment}
	}

	bindings, err := s.GetComponentBindings(ctx, req.OrgName, req.ProjectName, req.ComponentName, allEnvironments)
	if err != nil {
		return nil, fmt.Errorf("failed to get component bindings after promotion: %w", err)
	}

	s.logger.Debug("Component promoted successfully", "org", req.OrgName, "project", req.ProjectName, "component", req.ComponentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment, "bindingsCount", len(bindings))

	return bindings, nil
}

// extractImageFromWorkloadSpec extracts the first container image from the workload spec
// Returns empty string if no containers or images are found
func (s *ComponentService) extractImageFromWorkloadSpec(workloadSpec openchoreov1alpha1.WorkloadTemplateSpec) string {
	// If no containers are defined, return empty string
	if len(workloadSpec.Containers) == 0 {
		return ""
	}

	// Return the image from the first container
	// In most cases, there should be only one container, but we take the first if multiple exist
	for _, container := range workloadSpec.Containers {
		if container.Image != "" {
			return container.Image
		}
	}

	return ""
}

// mapConditionToBindingStatus maps Kubernetes condition status and reason to UI-friendly binding status
func (s *ComponentService) mapConditionToBindingStatus(condition metav1.Condition) models.BindingStatusType {
	if condition.Status == metav1.ConditionTrue {
		switch condition.Reason {
		case "AllResourcesReady":
			return models.BindingStatusTypeReady // "Active"
		case "ResourcesReadyWithSuspended":
			return models.BindingStatusTypeSuspended // "Suspended"
		default:
			return models.BindingStatusTypeReady // "Active"
		}
	}

	// Condition status is False
	switch condition.Reason {
	case "ResourceHealthProgressing":
		// Use BindingStatusTypePending which maps to "InProgress" in UI
		return models.BindingStatusTypePending // "InProgress"
	case "ResourceHealthDegraded", "ServiceClassNotFound", "APIClassNotFound",
		"InvalidConfiguration", "ReleaseCreationFailed", "ReleaseUpdateFailed":
		return models.BindingStatusTypeFailed // "Failed"
	default:
		// For unknown/initial states, use NotYetDeployed
		return models.BindingStatusTypeInProgress // "NotYetDeployed"
	}
}

// validatePromotionPath validates that the promotion path is allowed by the deployment pipeline
func (s *ComponentService) validatePromotionPath(ctx context.Context, orgName, projectName, sourceEnv, targetEnv string) error {
	// Get the project to determine the deployment pipeline reference
	project, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		return err
	}

	var pipelineName string
	if project.DeploymentPipeline != "" {
		pipelineName = project.DeploymentPipeline
	} else {
		pipelineName = "default"
	}

	// Get the deployment pipeline
	pipeline := &openchoreov1alpha1.DeploymentPipeline{}
	key := client.ObjectKey{
		Name:      pipelineName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, key, pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrDeploymentPipelineNotFound
		}
		return fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Check if the promotion path is valid
	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef == sourceEnv {
			for _, target := range path.TargetEnvironmentRefs {
				if target.Name == targetEnv {
					s.logger.Debug("Valid promotion path found", "source", sourceEnv, "target", targetEnv)
					return nil
				}
			}
		}
	}

	s.logger.Warn("Invalid promotion path", "source", sourceEnv, "target", targetEnv, "pipeline", pipelineName)
	return ErrInvalidPromotionPath
}

// createOrUpdateTargetBinding creates or updates the binding in the target environment
func (s *ComponentService) createOrUpdateTargetBinding(ctx context.Context, req *PromoteComponentPayload, componentType string) error {
	switch openchoreov1alpha1.ComponentType(componentType) {
	case openchoreov1alpha1.ComponentTypeService:
		return s.createOrUpdateServiceBinding(ctx, req)
	case openchoreov1alpha1.ComponentTypeWebApplication:
		return s.createOrUpdateWebApplicationBinding(ctx, req)
	case openchoreov1alpha1.ComponentTypeScheduledTask:
		return s.createOrUpdateScheduledTaskBinding(ctx, req)
	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}
}

// getServiceBindingCR retrieves a ServiceBinding CR from the cluster
func (s *ComponentService) getServiceBindingCR(ctx context.Context, orgName, componentName, environment string) (*openchoreov1alpha1.ServiceBinding, error) {
	// List all ServiceBindings in the namespace and filter by owner and environment
	bindingList := &openchoreov1alpha1.ServiceBindingList{}
	if err := s.k8sClient.List(ctx, bindingList, client.InNamespace(orgName)); err != nil {
		return nil, fmt.Errorf("failed to list service bindings: %w", err)
	}

	// Find the binding that matches the component and environment
	for i := range bindingList.Items {
		b := &bindingList.Items[i]
		if b.Spec.Owner.ComponentName == componentName && b.Spec.Environment == environment {
			return b, nil
		}
	}

	return nil, ErrBindingNotFound
}

// createOrUpdateServiceBinding creates or updates a ServiceBinding in the target environment
func (s *ComponentService) createOrUpdateServiceBinding(ctx context.Context, req *PromoteComponentPayload) error {
	// Get the source ServiceBinding CR using the new reusable method
	sourceK8sBinding, err := s.getServiceBindingCR(ctx, req.OrgName, req.ComponentName, req.SourceEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get source service binding: %w", err)
	}

	// First check if there's already a binding for this component in the target environment
	existingTargetBinding, err := s.getServiceBindingCR(ctx, req.OrgName, req.ComponentName, req.TargetEnvironment)
	var targetBindingName string

	if err != nil && err != ErrBindingNotFound {
		return fmt.Errorf("failed to check existing target binding: %w", err)
	}

	if err == ErrBindingNotFound {
		// No existing binding, generate new name
		targetBindingName = fmt.Sprintf("%s-%s", req.ComponentName, req.TargetEnvironment)
	} else {
		// Existing binding found, use its name
		targetBindingName = existingTargetBinding.Name
	}

	// Create or update the target ServiceBinding
	targetBinding := &openchoreov1alpha1.ServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetBindingName,
			Namespace: req.OrgName,
		},
		Spec: openchoreov1alpha1.ServiceBindingSpec{
			Owner: openchoreov1alpha1.ServiceOwner{
				ProjectName:   req.ProjectName,
				ComponentName: req.ComponentName,
			},
			Environment:  req.TargetEnvironment,
			ClassName:    sourceK8sBinding.Spec.ClassName,
			WorkloadSpec: sourceK8sBinding.Spec.WorkloadSpec,
			APIs:         sourceK8sBinding.Spec.APIs,
		},
	}

	if existingTargetBinding == nil {
		// Create new binding
		if err := s.k8sClient.Create(ctx, targetBinding); err != nil {
			return fmt.Errorf("failed to create target service binding: %w", err)
		}
		s.logger.Debug("Created new ServiceBinding", "name", targetBindingName, "namespace", req.OrgName)
	} else {
		// Update existing binding
		existingTargetBinding.Spec = targetBinding.Spec
		if err := s.k8sClient.Update(ctx, existingTargetBinding); err != nil {
			return fmt.Errorf("failed to update target service binding: %w", err)
		}
		s.logger.Debug("Updated existing ServiceBinding", "name", targetBindingName, "namespace", req.OrgName)
	}

	return nil
}

// getWebApplicationBindingCR retrieves a WebApplicationBinding CR from the cluster
func (s *ComponentService) getWebApplicationBindingCR(ctx context.Context, orgName, componentName, environment string) (*openchoreov1alpha1.WebApplicationBinding, error) {
	// List all WebApplicationBindings in the namespace and filter by owner and environment
	bindingList := &openchoreov1alpha1.WebApplicationBindingList{}
	if err := s.k8sClient.List(ctx, bindingList, client.InNamespace(orgName)); err != nil {
		return nil, fmt.Errorf("failed to list web application bindings: %w", err)
	}

	// Find the binding that matches the component and environment
	for i := range bindingList.Items {
		b := &bindingList.Items[i]
		if b.Spec.Owner.ComponentName == componentName && b.Spec.Environment == environment {
			return b, nil
		}
	}

	return nil, ErrBindingNotFound
}

// createOrUpdateWebApplicationBinding creates or updates a WebApplicationBinding in the target environment
func (s *ComponentService) createOrUpdateWebApplicationBinding(ctx context.Context, req *PromoteComponentPayload) error {
	// Get the source WebApplicationBinding CR using the new reusable method
	sourceK8sBinding, err := s.getWebApplicationBindingCR(ctx, req.OrgName, req.ComponentName, req.SourceEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get source web application binding: %w", err)
	}

	// First check if there's already a binding for this component in the target environment
	existingTargetBinding, err := s.getWebApplicationBindingCR(ctx, req.OrgName, req.ComponentName, req.TargetEnvironment)
	var targetBindingName string

	if err != nil && err != ErrBindingNotFound {
		return fmt.Errorf("failed to check existing target binding: %w", err)
	}

	if err == ErrBindingNotFound {
		// No existing binding, generate new name
		targetBindingName = fmt.Sprintf("%s-%s", req.ComponentName, req.TargetEnvironment)
	} else {
		// Existing binding found, use its name
		targetBindingName = existingTargetBinding.Name
	}

	// Create or update the target WebApplicationBinding
	targetBinding := &openchoreov1alpha1.WebApplicationBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetBindingName,
			Namespace: req.OrgName,
		},
		Spec: openchoreov1alpha1.WebApplicationBindingSpec{
			Owner: openchoreov1alpha1.WebApplicationOwner{
				ProjectName:   req.ProjectName,
				ComponentName: req.ComponentName,
			},
			Environment:  req.TargetEnvironment,
			ClassName:    sourceK8sBinding.Spec.ClassName,
			WorkloadSpec: sourceK8sBinding.Spec.WorkloadSpec,
			Overrides:    sourceK8sBinding.Spec.Overrides,
		},
	}

	if existingTargetBinding == nil {
		// Create new binding
		if err := s.k8sClient.Create(ctx, targetBinding); err != nil {
			return fmt.Errorf("failed to create target web application binding: %w", err)
		}
		s.logger.Debug("Created new WebApplicationBinding", "name", targetBindingName, "namespace", req.OrgName)
	} else {
		// Update existing binding
		existingTargetBinding.Spec = targetBinding.Spec
		if err := s.k8sClient.Update(ctx, existingTargetBinding); err != nil {
			return fmt.Errorf("failed to update target web application binding: %w", err)
		}
		s.logger.Debug("Updated existing WebApplicationBinding", "name", targetBindingName, "namespace", req.OrgName)
	}

	return nil
}

// getScheduledTaskBindingCR retrieves a ScheduledTaskBinding CR from the cluster
func (s *ComponentService) getScheduledTaskBindingCR(ctx context.Context, orgName, componentName, environment string) (*openchoreov1alpha1.ScheduledTaskBinding, error) {
	// List all ScheduledTaskBindings in the namespace and filter by owner and environment
	bindingList := &openchoreov1alpha1.ScheduledTaskBindingList{}
	if err := s.k8sClient.List(ctx, bindingList, client.InNamespace(orgName)); err != nil {
		return nil, fmt.Errorf("failed to list scheduled task bindings: %w", err)
	}

	// Find the binding that matches the component and environment
	for i := range bindingList.Items {
		b := &bindingList.Items[i]
		if b.Spec.Owner.ComponentName == componentName && b.Spec.Environment == environment {
			return b, nil
		}
	}

	return nil, ErrBindingNotFound
}

// createOrUpdateScheduledTaskBinding creates or updates a ScheduledTaskBinding in the target environment
func (s *ComponentService) createOrUpdateScheduledTaskBinding(ctx context.Context, req *PromoteComponentPayload) error {
	// Get the source ScheduledTaskBinding CR using the new reusable method
	sourceK8sBinding, err := s.getScheduledTaskBindingCR(ctx, req.OrgName, req.ComponentName, req.SourceEnvironment)
	if err != nil {
		return fmt.Errorf("failed to get source scheduled task binding: %w", err)
	}

	// First check if there's already a binding for this component in the target environment
	existingTargetBinding, err := s.getScheduledTaskBindingCR(ctx, req.OrgName, req.ComponentName, req.TargetEnvironment)
	var targetBindingName string

	if err != nil && err != ErrBindingNotFound {
		return fmt.Errorf("failed to check existing target binding: %w", err)
	}

	if err == ErrBindingNotFound {
		// No existing binding, generate new name
		targetBindingName = fmt.Sprintf("%s-%s", req.ComponentName, req.TargetEnvironment)
	} else {
		// Existing binding found, use its name
		targetBindingName = existingTargetBinding.Name
	}

	// Create or update the target ScheduledTaskBinding
	targetBinding := &openchoreov1alpha1.ScheduledTaskBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetBindingName,
			Namespace: req.OrgName,
		},
		Spec: openchoreov1alpha1.ScheduledTaskBindingSpec{
			Owner: openchoreov1alpha1.ScheduledTaskOwner{
				ProjectName:   req.ProjectName,
				ComponentName: req.ComponentName,
			},
			Environment:  req.TargetEnvironment,
			ClassName:    sourceK8sBinding.Spec.ClassName,
			WorkloadSpec: sourceK8sBinding.Spec.WorkloadSpec,
			Overrides:    sourceK8sBinding.Spec.Overrides,
		},
	}

	if existingTargetBinding == nil {
		// Create new binding
		if err := s.k8sClient.Create(ctx, targetBinding); err != nil {
			return fmt.Errorf("failed to create target scheduled task binding: %w", err)
		}
		s.logger.Debug("Created new ScheduledTaskBinding", "name", targetBindingName, "namespace", req.OrgName)
	} else {
		// Update existing binding
		existingTargetBinding.Spec = targetBinding.Spec
		if err := s.k8sClient.Update(ctx, existingTargetBinding); err != nil {
			return fmt.Errorf("failed to update target scheduled task binding: %w", err)
		}
		s.logger.Debug("Updated existing ScheduledTaskBinding", "name", targetBindingName, "namespace", req.OrgName)
	}

	return nil
}
