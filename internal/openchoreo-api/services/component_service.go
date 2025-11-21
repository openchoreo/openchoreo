// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/releasebinding"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	openchoreoschema "github.com/openchoreo/openchoreo/internal/schema"
)

const (
	// TODO: Move these constants to a shared package to avoid duplication
	statusReady    = "Ready"
	statusNotReady = "NotReady"
	statusUnknown  = "Unknown"
	statusFailed   = "Failed"
)

// ComponentService handles component-related business logic
type ComponentService struct {
	k8sClient           client.Client
	projectService      *ProjectService
	specFetcherRegistry *ComponentSpecFetcherRegistry
	logger              *slog.Logger
}

// parseComponentTypeName extracts the ComponentType name from the ComponentType string
// ComponentType format: {workloadType}/{componentTypeName}, e.g., "deployment/web-app"
// Returns the componentTypeName (second part after the slash)
func (s *ComponentService) parseComponentTypeName(componentType string) (string, error) {
	parts := strings.Split(componentType, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid component type format: %s", componentType)
	}
	return parts[1], nil
}

// buildTraitEnvOverridesSchema extracts and converts a TraitSpec's envOverrides to JSON schema
// Returns nil if the trait has no envOverrides
func (s *ComponentService) buildTraitEnvOverridesSchema(traitSpec openchoreov1alpha1.TraitSpec, traitName string) (*extv1.JSONSchemaProps, error) {
	var traitEnvOverrides map[string]any
	if traitSpec.Schema.EnvOverrides != nil && traitSpec.Schema.EnvOverrides.Raw != nil {
		if err := json.Unmarshal(traitSpec.Schema.EnvOverrides.Raw, &traitEnvOverrides); err != nil {
			return nil, fmt.Errorf("failed to extract envOverrides for trait %s: %w", traitName, err)
		}
	}

	if traitEnvOverrides == nil {
		return nil, nil
	}

	var traitTypes map[string]any
	if traitSpec.Schema.Types != nil && traitSpec.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(traitSpec.Schema.Types.Raw, &traitTypes); err != nil {
			return nil, fmt.Errorf("failed to extract types for trait %s: %w", traitName, err)
		}
	}

	traitDef := openchoreoschema.Definition{
		Types:   traitTypes,
		Schemas: []map[string]any{traitEnvOverrides},
	}

	traitJSONSchema, err := openchoreoschema.ToJSONSchema(traitDef)
	if err != nil {
		return nil, fmt.Errorf("failed to convert trait %s to JSON schema: %w", traitName, err)
	}

	return traitJSONSchema, nil
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

func (s *ComponentService) CreateComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (*models.ComponentReleaseResponse, error) {
	s.logger.Debug("Creating component release", "org", orgName, "project", projectName, "component", componentName, "release", releaseName)

	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "org", orgName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}
	component := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, componentKey, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to the project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "org", orgName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return nil, ErrComponentNotFound
	}

	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}
	workloadList := &openchoreov1alpha1.WorkloadList{}
	if err := s.k8sClient.List(ctx, workloadList, listOpts...); err != nil {
		s.logger.Error("Failed to list workloads", "error", err)
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	var workload *openchoreov1alpha1.Workload
	for _, item := range workloadList.Items {
		if item.Spec.Owner.ComponentName == componentName && item.Spec.Owner.ProjectName == projectName {
			workload = &item
			break
		}
	}

	if workload == nil {
		s.logger.Warn("Workload not found", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrWorkloadNotFound
	}

	// Generate release name if not provided
	if releaseName == "" {
		generatedName, err := s.generateReleaseName(ctx, orgName, projectName, componentName)
		if err != nil {
			return nil, err
		}
		releaseName = generatedName
	}

	// Get ComponentType if using new model
	var componentTypeSpec *openchoreov1alpha1.ComponentTypeSpec
	if component.Spec.ComponentType != "" {
		// Parse ComponentType name from format: {workloadType}/{componentTypeName}
		componentTypeName, err := s.parseComponentTypeName(component.Spec.ComponentType)
		if err != nil {
			s.logger.Error("Invalid ComponentType format", "componentType", component.Spec.ComponentType, "error", err)
			return nil, err
		}

		componentTypeKey := client.ObjectKey{
			Name:      componentTypeName,
			Namespace: orgName,
		}
		componentType := &openchoreov1alpha1.ComponentType{}
		if err := s.k8sClient.Get(ctx, componentTypeKey, componentType); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("ComponentType not found", "componentType", component.Spec.ComponentType)
			} else {
				s.logger.Error("Failed to get ComponentType", "error", err)
			}
		} else {
			componentTypeSpec = &componentType.Spec
		}
	}

	traits := make(map[string]openchoreov1alpha1.TraitSpec)
	for _, componentTrait := range component.Spec.Traits {
		traitKey := client.ObjectKey{
			Name:      componentTrait.Name,
			Namespace: orgName,
		}
		trait := &openchoreov1alpha1.Trait{}
		if err := s.k8sClient.Get(ctx, traitKey, trait); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("Trait not found", "trait", componentTrait.Name)
			} else {
				s.logger.Error("Failed to get Trait", "error", err)
			}
			continue
		}
		traits[componentTrait.Name] = trait.Spec
	}

	// Build ComponentProfile from Component parameters
	componentProfile := openchoreov1alpha1.ComponentProfile{}
	if component.Spec.Parameters != nil {
		componentProfile.Parameters = component.Spec.Parameters
	}

	if component.Spec.Traits != nil {
		componentProfile.Traits = component.Spec.Traits
	}

	// Build workload template spec from workload spec
	workloadTemplateSpec := openchoreov1alpha1.WorkloadTemplateSpec{
		Containers: workload.Spec.Containers,
		Endpoints:  workload.Spec.Endpoints,
	}

	componentRelease := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      releaseName,
			Namespace: orgName,
			Labels: map[string]string{
				labels.LabelKeyProjectName:   projectName,
				labels.LabelKeyComponentName: componentName,
			},
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			ComponentProfile: componentProfile,
			Workload:         workloadTemplateSpec,
		},
	}

	if componentTypeSpec != nil {
		componentRelease.Spec.ComponentType = *componentTypeSpec
	}

	if len(traits) > 0 {
		componentRelease.Spec.Traits = traits
	}

	if err := s.k8sClient.Create(ctx, componentRelease); err != nil {
		s.logger.Error("Failed to create ComponentRelease CR", "error", err)
		return nil, fmt.Errorf("failed to create component release: %w", err)
	}

	s.logger.Debug("ComponentRelease created successfully", "org", orgName, "project", projectName, "component", componentName, "release", releaseName)
	return &models.ComponentReleaseResponse{
		Name:          releaseName,
		ComponentName: componentName,
		ProjectName:   projectName,
		OrgName:       orgName,
		CreatedAt:     componentRelease.CreationTimestamp.Time,
		Status:        statusReady,
	}, nil
}

// generateReleaseName generates a unique release name for a component
// Format: <component_name>-<date>-<number>
// Example: my-component-20240118-1
func (s *ComponentService) generateReleaseName(ctx context.Context, orgName, projectName, componentName string) (string, error) {
	// List existing releases for this component
	releaseList := &openchoreov1alpha1.ComponentReleaseList{}
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
		client.MatchingLabels{
			labels.LabelKeyProjectName:   projectName,
			labels.LabelKeyComponentName: componentName,
		},
	}
	if err := s.k8sClient.List(ctx, releaseList, listOpts...); err != nil {
		s.logger.Error("Failed to list existing releases", "error", err)
		return "", fmt.Errorf("failed to list releases: %w", err)
	}

	// Generate date string in YYYYMMDD format
	now := metav1.Now()
	dateStr := now.Format("20060102")

	// Count releases created today with the same prefix
	todayPrefix := fmt.Sprintf("%s-%s-", componentName, dateStr)
	todayCount := 0
	for _, release := range releaseList.Items {
		if len(release.Name) >= len(todayPrefix) && release.Name[:len(todayPrefix)] == todayPrefix {
			todayCount++
		}
	}

	// Generate the release name with incremented count
	releaseName := fmt.Sprintf("%s-%s-%d", componentName, dateStr, todayCount+1)
	return releaseName, nil
}

// ListComponentReleases lists all component releases for a specific component
func (s *ComponentService) ListComponentReleases(ctx context.Context, orgName, projectName, componentName string) ([]*models.ComponentReleaseResponse, error) {
	s.logger.Debug("Listing component releases", "org", orgName, "project", projectName, "component", componentName)

	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	var releaseList openchoreov1alpha1.ComponentReleaseList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &releaseList, listOpts...); err != nil {
		s.logger.Error("Failed to list component releases", "error", err)
		return nil, fmt.Errorf("failed to list component releases: %w", err)
	}

	releases := make([]*models.ComponentReleaseResponse, 0, len(releaseList.Items))
	for _, item := range releaseList.Items {
		if item.Spec.Owner.ComponentName != componentName || item.Spec.Owner.ProjectName != projectName {
			continue
		}
		releases = append(releases, &models.ComponentReleaseResponse{
			Name:          item.Name,
			ComponentName: componentName,
			ProjectName:   projectName,
			OrgName:       orgName,
			CreatedAt:     item.CreationTimestamp.Time,
			Status:        statusReady,
		})
	}

	s.logger.Debug("Listed component releases", "org", orgName, "project", projectName, "component", componentName, "count", len(releases))
	return releases, nil
}

// GetComponentRelease retrieves a specific component release by its name
func (s *ComponentService) GetComponentRelease(ctx context.Context, orgName, projectName, componentName, releaseName string) (*models.ComponentReleaseResponse, error) {
	s.logger.Debug("Getting component release", "org", orgName, "project", projectName, "component", componentName, "release", releaseName)

	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	releaseKey := client.ObjectKey{
		Namespace: orgName,
		Name:      releaseName,
	}
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, releaseKey, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component release not found", "org", orgName, "project", projectName, "component", componentName, "release", releaseName)
			return nil, ErrComponentReleaseNotFound
		}
		s.logger.Error("Failed to get component release", "error", err)
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	if release.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Component release does not belong to component", "org", orgName, "component", componentName, "release", releaseName)
		return nil, ErrComponentReleaseNotFound
	}

	s.logger.Debug("Retrieved component release", "org", orgName, "project", projectName, "component", componentName, "release", releaseName)
	return &models.ComponentReleaseResponse{
		Name:          release.Name,
		ComponentName: componentName,
		ProjectName:   projectName,
		OrgName:       orgName,
		CreatedAt:     release.CreationTimestamp.Time,
		Status:        statusReady, // ComponentRelease is immutable, so it's always ready once created
	}, nil
}

// GetComponentReleaseSchema retrieves the JSON schema for a ComponentRelease
func (s *ComponentService) GetComponentReleaseSchema(ctx context.Context, orgName, projectName, componentName, releaseName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting component release schema", "org", orgName, "project", projectName, "component", componentName, "release", releaseName)

	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	releaseKey := client.ObjectKey{
		Namespace: orgName,
		Name:      releaseName,
	}
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, releaseKey, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component release not found", "org", orgName, "project", projectName, "component", componentName, "release", releaseName)
			return nil, ErrComponentReleaseNotFound
		}
		s.logger.Error("Failed to get component release", "error", err)
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	if release.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Component release does not belong to component", "org", orgName, "component", componentName, "release", releaseName)
		return nil, ErrComponentReleaseNotFound
	}

	var types map[string]any
	if release.Spec.ComponentType.Schema.Types != nil && release.Spec.ComponentType.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(release.Spec.ComponentType.Schema.Types.Raw, &types); err != nil {
			return nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	def := openchoreoschema.Definition{
		Types: types,
	}

	var componentTypeEnvOverrides map[string]any
	if release.Spec.ComponentType.Schema.EnvOverrides != nil && release.Spec.ComponentType.Schema.EnvOverrides.Raw != nil {
		if err := json.Unmarshal(release.Spec.ComponentType.Schema.EnvOverrides.Raw, &componentTypeEnvOverrides); err != nil {
			return nil, fmt.Errorf("failed to extract parameters: %w", err)
		}
	}

	// Build the wrapped schema properties
	wrappedSchema := &extv1.JSONSchemaProps{
		Type:       "object",
		Properties: make(map[string]extv1.JSONSchemaProps),
	}

	// Only add componentTypeEnvOverrides if there are actual envOverrides
	if componentTypeEnvOverrides != nil {
		def.Schemas = []map[string]any{componentTypeEnvOverrides}
		jsonSchema, err := openchoreoschema.ToJSONSchema(def)
		if err != nil {
			return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
		}
		wrappedSchema.Properties["componentTypeEnvOverrides"] = *jsonSchema
	}

	// Process trait overrides from ComponentRelease (trait instances with instance names)
	traitSchemas := make(map[string]extv1.JSONSchemaProps)
	for _, componentTrait := range release.Spec.ComponentProfile.Traits {
		traitSpec, found := release.Spec.Traits[componentTrait.Name]
		if !found {
			s.logger.Warn("Trait definition not found in release", "trait", componentTrait.Name, "instanceName", componentTrait.InstanceName)
			continue
		}

		traitJSONSchema, err := s.buildTraitEnvOverridesSchema(traitSpec, componentTrait.Name)
		if err != nil {
			return nil, err
		}

		// Use instance name as the key (not trait name)
		if traitJSONSchema != nil {
			traitSchemas[componentTrait.InstanceName] = *traitJSONSchema
		}
	}

	if len(traitSchemas) > 0 {
		wrappedSchema.Properties["traitOverrides"] = extv1.JSONSchemaProps{
			Type:       "object",
			Properties: traitSchemas,
		}
	}

	s.logger.Debug("Retrieved component release schema successfully", "org", orgName, "project", projectName, "component", componentName, "release", releaseName, "hasComponentTypeEnvOverrides", componentTypeEnvOverrides != nil, "traitCount", len(traitSchemas))
	return wrappedSchema, nil
}

// GetComponentSchema retrieves the JSON schema for a Component using the latest ComponentType
func (s *ComponentService) GetComponentSchema(ctx context.Context, orgName, projectName, componentName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting component schema", "org", orgName, "project", projectName, "component", componentName)

	// Get the component
	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Parse ComponentType name from format: {workloadType}/{componentTypeName}
	ctName, err := s.parseComponentTypeName(component.Spec.ComponentType)
	if err != nil {
		s.logger.Error("Invalid component type format", "componentType", component.Spec.ComponentType, "error", err)
		return nil, err
	}

	// Get the latest ComponentType
	ctKey := client.ObjectKey{
		Namespace: orgName,
		Name:      ctName,
	}
	var ct openchoreov1alpha1.ComponentType
	if err := s.k8sClient.Get(ctx, ctKey, &ct); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentType not found", "org", orgName, "name", ctName)
			return nil, ErrComponentTypeNotFound
		}
		s.logger.Error("Failed to get ComponentType", "error", err)
		return nil, fmt.Errorf("failed to get ComponentType: %w", err)
	}

	var types map[string]any
	if ct.Spec.Schema.Types != nil && ct.Spec.Schema.Types.Raw != nil {
		if err := yaml.Unmarshal(ct.Spec.Schema.Types.Raw, &types); err != nil {
			return nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	def := openchoreoschema.Definition{
		Types: types,
	}

	var envOverrides map[string]any
	if ct.Spec.Schema.EnvOverrides != nil && ct.Spec.Schema.EnvOverrides.Raw != nil {
		if err := json.Unmarshal(ct.Spec.Schema.EnvOverrides.Raw, &envOverrides); err != nil {
			return nil, fmt.Errorf("failed to extract envOverrides: %w", err)
		}
	}

	// Build the wrapped schema properties
	wrappedSchema := &extv1.JSONSchemaProps{
		Type:       "object",
		Properties: make(map[string]extv1.JSONSchemaProps),
	}

	// Only add componentTypeEnvOverrides if there are actual envOverrides
	if envOverrides != nil {
		def.Schemas = []map[string]any{envOverrides}
		jsonSchema, err := openchoreoschema.ToJSONSchema(def)
		if err != nil {
			return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
		}
		wrappedSchema.Properties["componentTypeEnvOverrides"] = *jsonSchema
	}

	// Process trait overrides from the component's traits
	traitSchemas := make(map[string]extv1.JSONSchemaProps)
	for _, componentTrait := range component.Spec.Traits {
		traitKey := client.ObjectKey{
			Namespace: orgName,
			Name:      componentTrait.Name,
		}
		var trait openchoreov1alpha1.Trait
		if err := s.k8sClient.Get(ctx, traitKey, &trait); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("Trait not found", "org", orgName, "trait", componentTrait.Name)
				continue // Skip missing traits instead of failing
			}
			s.logger.Error("Failed to get trait", "trait", componentTrait.Name, "error", err)
			return nil, fmt.Errorf("failed to get trait %s: %w", componentTrait.Name, err)
		}

		traitJSONSchema, err := s.buildTraitEnvOverridesSchema(trait.Spec, componentTrait.Name)
		if err != nil {
			return nil, err
		}

		// Use instance name as the key (not trait name)
		if traitJSONSchema != nil {
			traitSchemas[componentTrait.InstanceName] = *traitJSONSchema
		}
	}

	if len(traitSchemas) > 0 {
		wrappedSchema.Properties["traitOverrides"] = extv1.JSONSchemaProps{
			Type:       "object",
			Properties: traitSchemas,
		}
	}

	s.logger.Debug("Retrieved component schema successfully", "org", orgName, "project", projectName, "component", componentName, "hasComponentTypeEnvOverrides", envOverrides != nil, "traitCount", len(traitSchemas))
	return wrappedSchema, nil
}

// GetEnvironmentRelease retrieves the Release spec and status for a given component and environment
// Returns the full Release spec and status including resources, owner, environment information, and conditions
func (s *ComponentService) GetEnvironmentRelease(ctx context.Context, orgName, projectName, componentName, environmentName string) (*models.ReleaseResponse, error) {
	s.logger.Debug("Getting release", "org", orgName, "project", projectName, "component", componentName, "environment", environmentName)

	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	var releaseList openchoreov1alpha1.ReleaseList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName: orgName,
			labels.LabelKeyProjectName:      projectName,
			labels.LabelKeyComponentName:    componentName,
			labels.LabelKeyEnvironmentName:  environmentName,
		},
	}

	if err := s.k8sClient.List(ctx, &releaseList, listOpts...); err != nil {
		s.logger.Error("Failed to list releases", "error", err)
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	if len(releaseList.Items) == 0 {
		s.logger.Warn("No release found", "org", orgName, "project", projectName, "component", componentName, "environment", environmentName)
		return nil, ErrReleaseNotFound
	}

	// Get the first matching Release (there should only be one per component/environment)
	release := &releaseList.Items[0]

	s.logger.Debug("Retrieved release successfully", "org", orgName, "project", projectName, "component", componentName, "environment", environmentName, "resourceCount", len(release.Spec.Resources))
	return &models.ReleaseResponse{
		Spec:   release.Spec,
		Status: release.Status,
	}, nil
}

// PatchReleaseBinding patches a ReleaseBinding with environment-specific overrides
func (s *ComponentService) PatchReleaseBinding(ctx context.Context, orgName, projectName, componentName, bindingName string, req *models.PatchReleaseBindingRequest) (*models.ReleaseBindingResponse, error) {
	s.logger.Debug("Patching release binding", "org", orgName, "project", projectName, "component", componentName, "binding", bindingName)

	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	bindingKey := client.ObjectKey{
		Namespace: orgName,
		Name:      bindingName,
	}
	var binding openchoreov1alpha1.ReleaseBinding
	bindingExists := true
	if err := s.k8sClient.Get(ctx, bindingKey, &binding); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Binding doesn't exist, we'll create it
			bindingExists = false
			s.logger.Debug("Release binding not found, will create new one", "org", orgName, "binding", bindingName)

			if req.Environment == "" {
				s.logger.Warn("Environment is required when creating a new release binding")
				return nil, fmt.Errorf("environment is required when creating a new release binding")
			}

			binding = openchoreov1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: orgName,
					Labels: map[string]string{
						labels.LabelKeyProjectName:   projectName,
						labels.LabelKeyComponentName: componentName,
					},
				},
				Spec: openchoreov1alpha1.ReleaseBindingSpec{
					Owner: openchoreov1alpha1.ReleaseBindingOwner{
						ProjectName:   projectName,
						ComponentName: componentName,
					},
					Environment: req.Environment,
				},
			}

			if req.ReleaseName != "" {
				binding.Spec.ReleaseName = req.ReleaseName
			}
		} else {
			s.logger.Error("Failed to get release binding", "error", err)
			return nil, fmt.Errorf("failed to get release binding: %w", err)
		}
	}

	// Verify the binding belongs to the correct component (only if it already exists)
	if bindingExists && binding.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Release binding does not belong to component", "org", orgName, "component", componentName, "binding", bindingName)
		return nil, ErrReleaseBindingNotFound
	}

	if req.ComponentTypeEnvOverrides != nil {
		overridesJSON, err := json.Marshal(req.ComponentTypeEnvOverrides)
		if err != nil {
			s.logger.Error("Failed to marshal component type env overrides", "error", err)
			return nil, fmt.Errorf("failed to marshal component type env overrides: %w", err)
		}
		binding.Spec.ComponentTypeEnvOverrides = &runtime.RawExtension{Raw: overridesJSON}
	}

	if req.TraitOverrides != nil {
		binding.Spec.TraitOverrides = make(map[string]runtime.RawExtension)
		for instanceName, overrides := range req.TraitOverrides {
			overridesJSON, err := json.Marshal(overrides)
			if err != nil {
				s.logger.Error("Failed to marshal trait overrides", "error", err, "instanceName", instanceName)
				return nil, fmt.Errorf("failed to marshal trait overrides for %s: %w", instanceName, err)
			}
			binding.Spec.TraitOverrides[instanceName] = runtime.RawExtension{Raw: overridesJSON}
		}
	}

	if req.WorkloadOverrides != nil {
		containers := make(map[string]openchoreov1alpha1.ContainerOverride)

		for containerName, containerOverride := range req.WorkloadOverrides.Containers {
			envVars := make([]openchoreov1alpha1.EnvVar, len(containerOverride.Env))
			for i, env := range containerOverride.Env {
				envVar := openchoreov1alpha1.EnvVar{
					Key:   env.Key,
					Value: env.Value,
				}

				// Handle ValueFrom for secret references
				if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
					envVar.ValueFrom = &openchoreov1alpha1.EnvVarValueFrom{
						SecretRef: &openchoreov1alpha1.SecretKeyRef{
							Name: env.ValueFrom.SecretRef.Name,
							Key:  env.ValueFrom.SecretRef.Key,
						},
					}
				}

				envVars[i] = envVar
			}

			fileVars := make([]openchoreov1alpha1.FileVar, len(containerOverride.Files))
			for i, file := range containerOverride.Files {
				decodedValue := file.Value
				if file.Value != "" {
					decoded, err := base64.StdEncoding.DecodeString(file.Value)
					if err == nil {
						decodedValue = string(decoded)
					} else {
						s.logger.Warn("Failed to decode base64 file value, using original value",
							"key", file.Key,
							"containerName", containerName,
							"error", err)
					}
				}

				fileVar := openchoreov1alpha1.FileVar{
					Key:       file.Key,
					MountPath: file.MountPath,
					Value:     decodedValue,
				}

				// Handle ValueFrom for secret references
				if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
					fileVar.ValueFrom = &openchoreov1alpha1.EnvVarValueFrom{
						SecretRef: &openchoreov1alpha1.SecretKeyRef{
							Name: file.ValueFrom.SecretRef.Name,
							Key:  file.ValueFrom.SecretRef.Key,
						},
					}
				}

				fileVars[i] = fileVar
			}

			containers[containerName] = openchoreov1alpha1.ContainerOverride{
				Env:   envVars,
				Files: fileVars,
			}
		}

		binding.Spec.WorkloadOverrides = &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
			Containers: containers,
		}
	}

	// Create or update the binding
	if bindingExists {
		if err := s.k8sClient.Update(ctx, &binding); err != nil {
			s.logger.Error("Failed to update release binding", "error", err)
			return nil, fmt.Errorf("failed to update release binding: %w", err)
		}
		s.logger.Debug("Release binding updated successfully", "org", orgName, "project", projectName, "component", componentName, "binding", bindingName)
	} else {
		if err := s.k8sClient.Create(ctx, &binding); err != nil {
			s.logger.Error("Failed to create release binding", "error", err)
			return nil, fmt.Errorf("failed to create release binding: %w", err)
		}
		s.logger.Debug("Release binding created successfully", "org", orgName, "project", projectName, "component", componentName, "binding", bindingName)
	}

	return s.toReleaseBindingResponse(&binding, orgName, projectName, componentName), nil
}

// toReleaseBindingResponse converts a ReleaseBinding CR to a ReleaseBindingResponse
func (s *ComponentService) toReleaseBindingResponse(binding *openchoreov1alpha1.ReleaseBinding, orgName, projectName, componentName string) *models.ReleaseBindingResponse {
	response := &models.ReleaseBindingResponse{
		Name:          binding.Name,
		ComponentName: componentName,
		ProjectName:   projectName,
		OrgName:       orgName,
		Environment:   binding.Spec.Environment,
		ReleaseName:   binding.Spec.ReleaseName,
		CreatedAt:     binding.CreationTimestamp.Time,
		Status:        statusNotReady,
	}

	// Determine status from conditions
	response.Status = s.determineReleaseBindingStatus(binding)

	if binding.Spec.ComponentTypeEnvOverrides != nil {
		var overrides map[string]interface{}
		if err := json.Unmarshal(binding.Spec.ComponentTypeEnvOverrides.Raw, &overrides); err == nil {
			response.ComponentTypeEnvOverrides = overrides
		}
	}

	if len(binding.Spec.TraitOverrides) > 0 {
		response.TraitOverrides = make(map[string]interface{})
		for instanceName, rawExt := range binding.Spec.TraitOverrides {
			var overrides map[string]interface{}
			if err := json.Unmarshal(rawExt.Raw, &overrides); err == nil {
				response.TraitOverrides[instanceName] = overrides
			}
		}
	}

	if binding.Spec.WorkloadOverrides != nil {
		containers := make(map[string]models.ContainerOverride)

		for containerName, containerOverride := range binding.Spec.WorkloadOverrides.Containers {
			envVars := make([]models.EnvVar, len(containerOverride.Env))
			for i, env := range containerOverride.Env {
				envVar := models.EnvVar{
					Key:   env.Key,
					Value: env.Value,
				}

				// Handle ValueFrom for secret references
				if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
					envVar.ValueFrom = &models.EnvVarValueFrom{
						SecretRef: &models.SecretKeyRef{
							Name: env.ValueFrom.SecretRef.Name,
							Key:  env.ValueFrom.SecretRef.Key,
						},
					}
				}

				envVars[i] = envVar
			}

			fileVars := make([]models.FileVar, len(containerOverride.Files))
			for i, file := range containerOverride.Files {
				fileVar := models.FileVar{
					Key:       file.Key,
					MountPath: file.MountPath,
					Value:     file.Value,
				}

				// Handle ValueFrom for secret references
				if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
					fileVar.ValueFrom = &models.EnvVarValueFrom{
						SecretRef: &models.SecretKeyRef{
							Name: file.ValueFrom.SecretRef.Name,
							Key:  file.ValueFrom.SecretRef.Key,
						},
					}
				}

				fileVars[i] = fileVar
			}

			containers[containerName] = models.ContainerOverride{
				Env:   envVars,
				Files: fileVars,
			}
		}

		response.WorkloadOverrides = &models.WorkloadOverrides{
			Containers: containers,
		}
	}

	return response
}

func (s *ComponentService) determineReleaseBindingStatus(binding *openchoreov1alpha1.ReleaseBinding) string {
	if len(binding.Status.Conditions) == 0 {
		return statusNotReady
	}

	generation := binding.ObjectMeta.Generation

	// Collect all conditions for the current generation
	var conditionsForGeneration []metav1.Condition
	for i := range binding.Status.Conditions {
		if binding.Status.Conditions[i].ObservedGeneration == generation {
			conditionsForGeneration = append(conditionsForGeneration, binding.Status.Conditions[i])
		}
	}

	// Expected conditions: ReleaseSynced, ResourcesReady, Ready
	// If there are less than 3 conditions for the current generation, it's still in progress
	if len(conditionsForGeneration) < 3 {
		return statusNotReady
	}

	// Check if any condition has Status == False with ResourcesDegraded reason
	for i := range conditionsForGeneration {
		if conditionsForGeneration[i].Status == metav1.ConditionFalse && conditionsForGeneration[i].Reason == string(releasebinding.ReasonResourcesDegraded) {
			return statusFailed
		}
	}

	// Check if any condition has Status == False with ResourcesProgressing reason
	for i := range conditionsForGeneration {
		if conditionsForGeneration[i].Status == metav1.ConditionFalse && conditionsForGeneration[i].Reason == string(releasebinding.ReasonResourcesProgressing) {
			return statusNotReady
		}
	}

	// If all three conditions are present and none are degraded, it's ready
	return statusReady
}

// ListReleaseBindings lists all release bindings for a specific component
// If environments is provided, only returns bindings for those environments
func (s *ComponentService) ListReleaseBindings(ctx context.Context, orgName, projectName, componentName string, environments []string) ([]*models.ReleaseBindingResponse, error) {
	s.logger.Debug("Listing release bindings", "org", orgName, "project", projectName, "component", componentName, "environments", environments)

	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	var bindingList openchoreov1alpha1.ReleaseBindingList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &bindingList, listOpts...); err != nil {
		s.logger.Error("Failed to list release bindings", "error", err)
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	bindings := make([]*models.ReleaseBindingResponse, 0, len(bindingList.Items))
	for i := range bindingList.Items {
		if bindingList.Items[i].Spec.Owner.ComponentName != componentName || bindingList.Items[i].Spec.Owner.ProjectName != projectName {
			continue
		}
		binding := &bindingList.Items[i]

		if len(environments) > 0 {
			matchesEnv := false
			for _, env := range environments {
				if binding.Spec.Environment == env {
					matchesEnv = true
					break
				}
			}
			if !matchesEnv {
				continue
			}
		}

		bindings = append(bindings, s.toReleaseBindingResponse(binding, orgName, projectName, componentName))
	}

	s.logger.Debug("Listed release bindings", "org", orgName, "project", projectName, "component", componentName, "count", len(bindings))
	return bindings, nil
}

// DeployRelease deploys a component release to the lowest environment in the deployment pipeline
func (s *ComponentService) DeployRelease(ctx context.Context, orgName, projectName, componentName string, req *models.DeployReleaseRequest) (*models.ReleaseBindingResponse, error) {
	s.logger.Debug("Deploying release", "org", orgName, "project", projectName, "component", componentName, "release", req.ReleaseName)

	project, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	pipelineName := project.DeploymentPipeline
	if pipelineName == "" {
		s.logger.Warn("Project has no deployment pipeline", "org", orgName, "project", projectName)
		return nil, fmt.Errorf("project has no deployment pipeline configured")
	}

	pipelineKey := client.ObjectKey{
		Namespace: orgName,
		Name:      pipelineName,
	}
	var pipeline openchoreov1alpha1.DeploymentPipeline
	if err := s.k8sClient.Get(ctx, pipelineKey, &pipeline); err != nil {
		s.logger.Error("Failed to get deployment pipeline", "error", err, "pipeline", pipelineName)
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Find the lowest environment (source environment with no incoming paths)
	lowestEnv := s.findLowestEnvironment(pipeline.Spec.PromotionPaths)
	if lowestEnv == "" {
		s.logger.Warn("No lowest environment found in deployment pipeline", "pipeline", pipelineName)
		return nil, fmt.Errorf("no lowest environment found in deployment pipeline")
	}

	s.logger.Debug("Found lowest environment", "environment", lowestEnv)

	// Verify component exists
	componentKey := client.ObjectKey{
		Namespace: orgName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	releaseKey := client.ObjectKey{
		Namespace: orgName,
		Name:      req.ReleaseName,
	}
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, releaseKey, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component release not found", "org", orgName, "release", req.ReleaseName)
			return nil, ErrComponentReleaseNotFound
		}
		s.logger.Error("Failed to get component release", "error", err)
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	if release.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Release does not belong to component", "component", componentName, "release", req.ReleaseName)
		return nil, ErrComponentReleaseNotFound
	}

	bindingName := fmt.Sprintf("%s-%s", componentName, lowestEnv)
	bindingKey := client.ObjectKey{
		Namespace: orgName,
		Name:      bindingName,
	}

	var binding openchoreov1alpha1.ReleaseBinding
	bindingExists := true
	if err := s.k8sClient.Get(ctx, bindingKey, &binding); err != nil {
		if client.IgnoreNotFound(err) == nil {
			bindingExists = false
		} else {
			s.logger.Error("Failed to get release binding", "error", err)
			return nil, fmt.Errorf("failed to get release binding: %w", err)
		}
	}

	if bindingExists {
		s.logger.Debug("Updating existing release binding", "binding", bindingName)
		binding.Spec.ReleaseName = req.ReleaseName
		if err := s.k8sClient.Update(ctx, &binding); err != nil {
			s.logger.Error("Failed to update release binding", "error", err)
			return nil, fmt.Errorf("failed to update release binding: %w", err)
		}
	} else {
		s.logger.Debug("Creating new release binding", "binding", bindingName)
		binding = openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: orgName,
				Labels: map[string]string{
					labels.LabelKeyProjectName:   projectName,
					labels.LabelKeyComponentName: componentName,
				},
			},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{
					ProjectName:   projectName,
					ComponentName: componentName,
				},
				Environment: lowestEnv,
				ReleaseName: req.ReleaseName,
			},
		}
		if err := s.k8sClient.Create(ctx, &binding); err != nil {
			s.logger.Error("Failed to create release binding", "error", err)
			return nil, fmt.Errorf("failed to create release binding: %w", err)
		}
	}

	s.logger.Debug("Release deployed successfully", "org", orgName, "project", projectName, "component", componentName, "release", req.ReleaseName, "environment", lowestEnv)
	return s.toReleaseBindingResponse(&binding, orgName, projectName, componentName), nil
}

// findLowestEnvironment finds the lowest environment in the deployment pipeline
// The lowest environment is one that is not a target in any promotion path
func (s *ComponentService) findLowestEnvironment(promotionPaths []openchoreov1alpha1.PromotionPath) string {
	if len(promotionPaths) == 0 {
		return ""
	}

	// Collect all target environments
	targets := make(map[string]bool)
	for _, path := range promotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find a source environment that is not a target
	for _, path := range promotionPaths {
		if !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}

	// If all sources are targets (circular), return the first source
	if len(promotionPaths) > 0 {
		return promotionPaths[0].SourceEnvironmentRef
	}

	return ""
}

// CreateComponent creates a new component in the given project
func (s *ComponentService) CreateComponent(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Creating component", "org", orgName, "project", projectName, "component", req.Name)

	// Sanitize input
	req.Sanitize()

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
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
	component, err := s.createComponentResources(ctx, orgName, projectName, req)
	if err != nil {
		s.logger.Error("Failed to create component resources", "error", err)
		return nil, fmt.Errorf("failed to create component: %w", err)
	}

	s.logger.Debug("Component created successfully", "org", orgName, "project", projectName, "component", req.Name)

	// Return the created component
	return &models.ComponentResponse{
		UID:         string(component.UID),
		Name:        component.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Type:        req.Type,
		ProjectName: projectName,
		OrgName:     orgName,
		CreatedAt:   component.CreationTimestamp.Time,
		Status:      "Created",
	}, nil
}

// ListComponents lists all components in the given project
func (s *ComponentService) ListComponents(ctx context.Context, orgName, projectName string) ([]*models.ComponentResponse, error) {
	s.logger.Debug("Listing components", "org", orgName, "project", projectName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	var componentList openchoreov1alpha1.ComponentList
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, &componentList, listOpts...); err != nil {
		s.logger.Error("Failed to list components", "error", err)
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	components := make([]*models.ComponentResponse, 0, len(componentList.Items))
	for _, item := range componentList.Items {
		// Only include components that belong to the specified project
		if item.Spec.Owner.ProjectName == projectName {
			components = append(components, s.toComponentResponse(&item, make(map[string]interface{}), false))
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
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	component := &openchoreov1alpha1.Component{}
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

	return s.toComponentResponse(component, typeSpecs, true), nil
}

// componentExists checks if a component already exists by name and namespace and belongs to the specified project
func (s *ComponentService) componentExists(ctx context.Context, orgName, projectName, componentName string) (bool, error) {
	component := &openchoreov1alpha1.Component{}
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
func (s *ComponentService) createComponentResources(ctx context.Context, orgName, projectName string, req *models.CreateComponentRequest) (*openchoreov1alpha1.Component, error) {
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}

	annotations := map[string]string{
		controller.AnnotationKeyDisplayName: displayName,
		controller.AnnotationKeyDescription: req.Description,
	}

	componentCR := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   orgName,
			Annotations: annotations,
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: projectName,
			},
			ComponentType: req.Type,
		},
	}

	// Set workflow configuration if provided
	if req.Workflow != nil {
		componentCR.Spec.Workflow = &openchoreov1alpha1.WorkflowConfig{
			Name:   req.Workflow.Name,
			Schema: req.Workflow.Schema,
		}
	}

	if err := s.k8sClient.Create(ctx, componentCR); err != nil {
		return nil, fmt.Errorf("failed to create component CR: %w", err)
	}

	return componentCR, nil
}

// toComponentResponse converts a Component CR to a ComponentResponse
// includeWorkflow parameter controls whether to include workflow in the response
func (s *ComponentService) toComponentResponse(component *openchoreov1alpha1.Component, typeSpecs map[string]interface{}, includeWorkflow bool) *models.ComponentResponse {
	// Extract project name from the component owner
	projectName := component.Spec.Owner.ProjectName

	// Get status - Component doesn't have conditions yet, so default to Creating
	// This can be enhanced later when Component adds status conditions
	status := "Created"

	// Convert workflow configuration to API Workflow format only if requested
	var workflow *models.Workflow
	if includeWorkflow && component.Spec.Workflow != nil {
		workflow = &models.Workflow{
			Name:   component.Spec.Workflow.Name,
			Schema: component.Spec.Workflow.Schema,
		}
	}

	response := &models.ComponentResponse{
		UID:         string(component.UID),
		Name:        component.Name,
		DisplayName: component.Annotations[controller.AnnotationKeyDisplayName],
		Description: component.Annotations[controller.AnnotationKeyDescription],
		Type:        component.Spec.ComponentType,
		ProjectName: projectName,
		OrgName:     component.Namespace,
		CreatedAt:   component.CreationTimestamp.Time,
		Status:      status,
		Workflow:    workflow,
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

	bindings := make([]*models.BindingResponse, 0, len(environments))
	for _, environment := range environments {
		binding, err := s.getComponentBinding(ctx, orgName, projectName, componentName, environment, component.Type)
		if err != nil {
			// If binding not found for an environment, skip it rather than failing the entire request
			if errors.Is(err, ErrBindingNotFound) {
				s.logger.Debug("Binding not found for environment", "environment", environment)
				continue
			}
			return nil, err
		}
		bindings = append(bindings, binding)
	}

	s.logger.Info("Bindings", "bindings", bindings)

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
	switch openchoreov1alpha1.DefinedComponentType(componentType) {
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
			Status:  models.BindingStatusTypeUndeployed, // Default to "NotYetDeployed"
			Reason:  "",
			Message: "",
		},
	}

	// Extract status from conditions and map to UI-friendly status
	for _, condition := range binding.Status.Conditions {
		if condition.Type == statusReady {
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
			Status:  models.BindingStatusTypeUndeployed, // Default to "NotYetDeployed"
			Reason:  "",
			Message: "",
		},
	}

	// Extract status from conditions and map to UI-friendly status
	for _, condition := range binding.Status.Conditions {
		if condition.Type == statusReady {
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
			Status:  models.BindingStatusTypeUndeployed, // Default to "NotYetDeployed"
			Reason:  "",
			Message: "",
		},
	}

	// TODO: ScheduledTaskBinding doesn't have conditions in its status yet
	// When conditions are added, implement the same status mapping logic as Service and WebApplication bindings
	// For now, default to NotYetDeployed status
	response.BindingStatus.Status = models.BindingStatusTypeUndeployed

	// ScheduledTaskBinding doesn't have endpoints, but we still extract the image
	response.ScheduledTaskBinding = &models.ScheduledTaskBinding{
		Image: s.extractImageFromWorkloadSpec(binding.Spec.WorkloadSpec),
	}

	return response, nil
}

// convertEndpointStatus converts from Kubernetes endpoint status to API response model
func (s *ComponentService) convertEndpointStatus(endpoints []openchoreov1alpha1.EndpointStatus) []models.EndpointStatus {
	result := make([]models.EndpointStatus, 0, len(endpoints))

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
		pipelineName = defaultPipeline
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
	environments := make([]string, 0, len(environmentSet))
	for env := range environmentSet {
		environments = append(environments, env)
	}

	s.logger.Debug("Extracted environments from deployment pipeline", "pipeline", pipelineName, "environments", environments)
	return environments, nil
}

// PromoteComponent promotes a component from source environment to target environment
func (s *ComponentService) PromoteComponent(ctx context.Context, req *PromoteComponentPayload) (*models.ReleaseBindingResponse, error) {
	s.logger.Debug("Promoting component", "org", req.OrgName, "project", req.ProjectName, "component", req.ComponentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment)

	if err := s.validatePromotionPath(ctx, req.OrgName, req.ProjectName, req.SourceEnvironment, req.TargetEnvironment); err != nil {
		return nil, err
	}

	sourceReleaseBinding, err := s.getReleaseBinding(ctx, req.OrgName, req.ProjectName, req.ComponentName, req.SourceEnvironment)
	if err != nil {
		return nil, fmt.Errorf("failed to get source release binding: %w", err)
	}

	if err := s.createOrUpdateReleaseBinding(ctx, req, sourceReleaseBinding); err != nil {
		return nil, fmt.Errorf("failed to create/update target release binding: %w", err)
	}

	targetReleaseBinding, err := s.getReleaseBinding(ctx, req.OrgName, req.ProjectName, req.ComponentName, req.TargetEnvironment)
	if err != nil {
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	return s.toReleaseBindingResponse(targetReleaseBinding, req.OrgName, req.ProjectName, req.ComponentName), nil
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
		return models.BindingStatusTypeReady // "Active"
	}

	// Condition status is False
	switch condition.Reason {
	case "ResourcesSuspended", "ResourcesUndeployed":
		return models.BindingStatusTypeSuspended // "Suspended"
	case "ResourceHealthProgressing":
		// Use BindingStatusTypeInProgress, which maps to "InProgress" in UI
		return models.BindingStatusTypeInProgress // "InProgress"
	case "ResourceHealthDegraded", "ServiceClassNotFound", "APIClassNotFound", "WebApplicationClassNotFound", "ScheduledTaskClassNotFound",
		"InvalidConfiguration", "ReleaseCreationFailed", "ReleaseUpdateFailed", "ReleaseDeletionFailed":
		return models.BindingStatusTypeFailed // "Failed"
	default:
		// For unknown/initial states, use NotYetDeployed
		return models.BindingStatusTypeUndeployed // "NotYetDeployed"
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
		pipelineName = defaultPipeline
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

	s.logger.Info("Promotion paths", "promotionPaths", pipeline.Spec.PromotionPaths)

	// Check if the promotion path is valid
	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef == sourceEnv {
			s.logger.Info("Source environment", "source", sourceEnv)
			for _, target := range path.TargetEnvironmentRefs {
				s.logger.Info("Target environment", "target", target.Name)
				if target.Name == targetEnv {
					s.logger.Info("Valid promotion path found", "source", sourceEnv, "target", targetEnv)
					s.logger.Debug("Valid promotion path found", "source", sourceEnv, "target", targetEnv)
					return nil
				}
			}
		}
	}

	s.logger.Warn("Invalid promotion path", "source", sourceEnv, "target", targetEnv, "pipeline", pipelineName)
	return ErrInvalidPromotionPath
}

// getReleaseBinding retrieves a ReleaseBinding for a component in a specific environment
func (s *ComponentService) getReleaseBinding(ctx context.Context, orgName, projectName, componentName, environment string) (*openchoreov1alpha1.ReleaseBinding, error) {
	// List all ReleaseBindings in the namespace
	bindingList := &openchoreov1alpha1.ReleaseBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(orgName),
	}

	if err := s.k8sClient.List(ctx, bindingList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	s.logger.Info("Release bindings", "releaseBindings", bindingList.Items)

	// Find the binding that matches the environment
	for i := range bindingList.Items {
		if bindingList.Items[i].Spec.Owner.ComponentName != componentName || bindingList.Items[i].Spec.Owner.ProjectName != projectName {
			continue
		}
		binding := &bindingList.Items[i]
		if binding.Spec.Environment == environment {
			return binding, nil
		}
	}

	return nil, ErrReleaseBindingNotFound
}

// createOrUpdateReleaseBinding creates or updates a ReleaseBinding in the target environment
func (s *ComponentService) createOrUpdateReleaseBinding(ctx context.Context, req *PromoteComponentPayload, sourceBinding *openchoreov1alpha1.ReleaseBinding) error {
	// Check if there's already a binding for this component in the target environment
	existingTargetBinding, err := s.getReleaseBinding(ctx, req.OrgName, req.ProjectName, req.ComponentName, req.TargetEnvironment)
	var targetBindingName string

	if err != nil && !errors.Is(err, ErrReleaseBindingNotFound) {
		return fmt.Errorf("failed to check existing target binding: %w", err)
	}

	if errors.Is(err, ErrReleaseBindingNotFound) {
		// No existing binding, generate new name
		targetBindingName = fmt.Sprintf("%s-%s", req.ComponentName, req.TargetEnvironment)
	} else {
		// Existing binding found, use its name
		targetBindingName = existingTargetBinding.Name
	}

	var targetBinding *openchoreov1alpha1.ReleaseBinding

	if existingTargetBinding == nil {
		targetBinding = &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetBindingName,
				Namespace: req.OrgName,
				Labels: map[string]string{
					labels.LabelKeyProjectName:   req.ProjectName,
					labels.LabelKeyComponentName: req.ComponentName,
				},
			},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{
					ProjectName:   req.ProjectName,
					ComponentName: req.ComponentName,
				},
				Environment: req.TargetEnvironment,
				ReleaseName: sourceBinding.Spec.ReleaseName,
			},
		}
	} else {
		targetBinding = existingTargetBinding
		targetBinding.Spec.ReleaseName = sourceBinding.Spec.ReleaseName
	}

	if existingTargetBinding == nil {
		// Create new binding
		if err := s.k8sClient.Create(ctx, targetBinding); err != nil {
			return fmt.Errorf("failed to create target release binding: %w", err)
		}
		s.logger.Debug("Created new ReleaseBinding", "name", targetBindingName, "namespace", req.OrgName, "environment", req.TargetEnvironment)
	} else {
		// Update existing binding
		if err := s.k8sClient.Update(ctx, targetBinding); err != nil {
			return fmt.Errorf("failed to update target release binding: %w", err)
		}
		s.logger.Debug("Updated existing ReleaseBinding", "name", targetBindingName, "namespace", req.OrgName, "environment", req.TargetEnvironment)
	}

	return nil
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

// UpdateComponentBinding updates a component binding
func (s *ComponentService) UpdateComponentBinding(ctx context.Context, orgName, projectName, componentName, bindingName string, req *models.UpdateBindingRequest) (*models.BindingResponse, error) {
	s.logger.Debug("Updating component binding", "org", orgName, "project", projectName, "component", componentName, "binding", bindingName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "org", orgName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Verify component exists
	exists, err := s.componentExists(ctx, orgName, projectName, componentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if !exists {
		s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Get the component type to determine which binding type to update
	component, err := s.GetComponent(ctx, orgName, projectName, componentName, []string{})
	if err != nil {
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Update the appropriate binding based on component type
	var updatedBinding *models.BindingResponse
	switch component.Type {
	case string(openchoreov1alpha1.ComponentTypeService):
		binding := &openchoreov1alpha1.ServiceBinding{}
		err = s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      bindingName,
			Namespace: orgName,
		}, binding)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				s.logger.Error("Failed to get service binding", "error", err)
				return nil, fmt.Errorf("failed to get service binding: %w", err)
			}
			s.logger.Warn("Service binding not found", "binding", bindingName)
			return nil, ErrBindingNotFound
		}

		// Update the releaseState
		binding.Spec.ReleaseState = openchoreov1alpha1.ReleaseState(req.ReleaseState)

		if err := s.k8sClient.Update(ctx, binding); err != nil {
			s.logger.Error("Failed to update service binding", "error", err)
			return nil, fmt.Errorf("failed to update service binding: %w", err)
		}

		updatedBinding = &models.BindingResponse{
			Name:          binding.Name,
			Type:          string(openchoreov1alpha1.ComponentTypeService),
			ComponentName: componentName,
			ProjectName:   projectName,
			OrgName:       orgName,
			Environment:   binding.Spec.Environment,
			ServiceBinding: &models.ServiceBinding{
				ReleaseState: string(binding.Spec.ReleaseState),
			},
		}

	case string(openchoreov1alpha1.ComponentTypeWebApplication):
		binding := &openchoreov1alpha1.WebApplicationBinding{}
		err = s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      bindingName,
			Namespace: orgName,
		}, binding)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				s.logger.Error("Failed to get web application binding", "error", err)
				return nil, fmt.Errorf("failed to get web application binding: %w", err)
			}
			s.logger.Warn("Web application binding not found", "binding", bindingName)
			return nil, ErrBindingNotFound
		}

		// Update the releaseState
		binding.Spec.ReleaseState = openchoreov1alpha1.ReleaseState(req.ReleaseState)

		if err := s.k8sClient.Update(ctx, binding); err != nil {
			s.logger.Error("Failed to update web application binding", "error", err)
			return nil, fmt.Errorf("failed to update web application binding: %w", err)
		}

		updatedBinding = &models.BindingResponse{
			Name:          binding.Name,
			Type:          string(openchoreov1alpha1.ComponentTypeWebApplication),
			ComponentName: componentName,
			ProjectName:   projectName,
			OrgName:       orgName,
			Environment:   binding.Spec.Environment,
			WebApplicationBinding: &models.WebApplicationBinding{
				ReleaseState: string(binding.Spec.ReleaseState),
			},
		}

	case string(openchoreov1alpha1.ComponentTypeScheduledTask):
		binding := &openchoreov1alpha1.ScheduledTaskBinding{}
		err = s.k8sClient.Get(ctx, client.ObjectKey{
			Name:      bindingName,
			Namespace: orgName,
		}, binding)
		if err != nil {
			if client.IgnoreNotFound(err) != nil {
				s.logger.Error("Failed to get scheduled task binding", "error", err)
				return nil, fmt.Errorf("failed to get scheduled task binding: %w", err)
			}
			s.logger.Warn("Scheduled task binding not found", "binding", bindingName)
			return nil, ErrBindingNotFound
		}

		// Update the releaseState
		binding.Spec.ReleaseState = openchoreov1alpha1.ReleaseState(req.ReleaseState)

		if err := s.k8sClient.Update(ctx, binding); err != nil {
			s.logger.Error("Failed to update scheduled task binding", "error", err)
			return nil, fmt.Errorf("failed to update scheduled task binding: %w", err)
		}

		updatedBinding = &models.BindingResponse{
			Name:          binding.Name,
			Type:          string(openchoreov1alpha1.ComponentTypeScheduledTask),
			ComponentName: componentName,
			ProjectName:   projectName,
			OrgName:       orgName,
			Environment:   binding.Spec.Environment,
			ScheduledTaskBinding: &models.ScheduledTaskBinding{
				ReleaseState: string(binding.Spec.ReleaseState),
			},
		}

	default:
		s.logger.Error("Unsupported component type", "type", component.Type)
		return nil, fmt.Errorf("unsupported component type: %s", component.Type)
	}

	s.logger.Debug("Component binding updated successfully", "org", orgName, "project", projectName, "component", componentName, "binding", bindingName)
	return updatedBinding, nil
}

// ComponentObserverResponse represents the response for observer URL requests
type ComponentObserverResponse struct {
	ObserverURL      string                    `json:"observerUrl,omitempty"`
	ConnectionMethod *ObserverConnectionMethod `json:"connectionMethod,omitempty"`
	Message          string                    `json:"message,omitempty"`
}

// ObserverConnectionMethod contains the access method for the observer
type ObserverConnectionMethod struct {
	Type        string `json:"type,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	BearerToken string `json:"bearerToken,omitempty"`
}

// GetComponentObserverURL retrieves the observer URL for component runtime logs
func (s *ComponentService) GetComponentObserverURL(ctx context.Context, orgName, projectName, componentName, environmentName string) (*ComponentObserverResponse, error) {
	s.logger.Debug("Getting component observer URL", "org", orgName, "project", projectName, "component", componentName, "environment", environmentName)

	// 1. Verify component exists in project
	_, err := s.GetComponent(ctx, orgName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	// 2. Get the environment
	env := &openchoreov1alpha1.Environment{}
	envKey := client.ObjectKey{
		Name:      environmentName,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, envKey, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Environment not found", "org", orgName, "environment", environmentName)
			return nil, ErrEnvironmentNotFound
		}
		s.logger.Error("Failed to get environment", "error", err, "org", orgName, "environment", environmentName)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// 3. Check if environment has a dataplane reference
	if env.Spec.DataPlaneRef == "" {
		s.logger.Error("Environment has no dataplane reference", "environment", environmentName)
		return nil, fmt.Errorf("environment %s has no dataplane reference", environmentName)
	}

	// 4. Get the DataPlane configuration for the environment
	dp := &openchoreov1alpha1.DataPlane{}
	dpKey := client.ObjectKey{
		Name:      env.Spec.DataPlaneRef,
		Namespace: orgName,
	}

	if err := s.k8sClient.Get(ctx, dpKey, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Error("DataPlane not found", "org", orgName, "dataplane", env.Spec.DataPlaneRef)
			return nil, ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get dataplane", "error", err, "org", orgName, "dataplane", env.Spec.DataPlaneRef)
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}

	// 5. Check if observer is configured in the dataplane
	if dp.Spec.Observer.URL == "" {
		s.logger.Debug("Observer URL not configured in dataplane", "dataplane", dp.Name)
		return &ComponentObserverResponse{
			Message: "observability-logs have not been configured",
		}, nil
	}

	// 6. Return observer URL and connection method from DataPlane.Spec.Observer
	connectionMethod := &ObserverConnectionMethod{
		Type:     "basic",
		Username: dp.Spec.Observer.Authentication.BasicAuth.Username,
		Password: dp.Spec.Observer.Authentication.BasicAuth.Password,
	}

	return &ComponentObserverResponse{
		ObserverURL:      dp.Spec.Observer.URL,
		ConnectionMethod: connectionMethod,
	}, nil
}

// GetBuildObserverURL retrieves the observer URL for component build logs
func (s *ComponentService) GetBuildObserverURL(ctx context.Context, orgName, projectName, componentName string) (*ComponentObserverResponse, error) {
	s.logger.Debug("Getting build observer URL", "org", orgName, "project", projectName, "component", componentName)

	// 1. Verify component exists in project
	_, err := s.GetComponent(ctx, orgName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	// 2. Get BuildPlane configuration for the organization
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	err = s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(orgName))
	if err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "org", orgName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	// Check if any build planes exist
	if len(buildPlanes.Items) == 0 {
		s.logger.Error("No build planes found", "org", orgName)
		return nil, fmt.Errorf("no build planes found for organization: %s", orgName)
	}

	// Get the first build plane (0th index)
	buildPlane := &buildPlanes.Items[0]
	s.logger.Debug("Found build plane", "name", buildPlane.Name, "org", orgName)

	// 3. Check if observer is configured
	if buildPlane.Spec.Observer.URL == "" {
		s.logger.Debug("Observer URL not configured in build plane", "buildPlane", buildPlane.Name)
		return &ComponentObserverResponse{
			Message: "observability-logs have not been configured",
		}, nil
	}

	// 4. Return observer URL and connection method from BuildPlane.Spec.Observer
	connectionMethod := &ObserverConnectionMethod{
		Type:     "basic",
		Username: buildPlane.Spec.Observer.Authentication.BasicAuth.Username,
		Password: buildPlane.Spec.Observer.Authentication.BasicAuth.Password,
	}

	return &ComponentObserverResponse{
		ObserverURL:      buildPlane.Spec.Observer.URL,
		ConnectionMethod: connectionMethod,
	}, nil
}

// GetComponentWorkloads retrieves workload data for a specific component
func (s *ComponentService) GetComponentWorkloads(ctx context.Context, orgName, projectName, componentName string) (interface{}, error) {
	s.logger.Debug("Getting component workloads", "org", orgName, "project", projectName, "component", componentName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Verify component exists and belongs to the project
	component := &openchoreov1alpha1.Component{}
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

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "org", orgName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Use the WorkloadSpecFetcher to get workload data
	fetcher := &WorkloadSpecFetcher{}
	workloadSpec, err := fetcher.FetchSpec(ctx, s.k8sClient, orgName, componentName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workload not found for component", "org", orgName, "project", projectName, "component", componentName)
			return nil, fmt.Errorf("workload not found for component: %s", componentName)
		}
		s.logger.Error("Failed to fetch workload spec", "error", err)
		return nil, fmt.Errorf("failed to fetch workload spec: %w", err)
	}

	return workloadSpec, nil
}

// CreateComponentWorkload creates or updates workload data for a specific component
func (s *ComponentService) CreateComponentWorkload(ctx context.Context, orgName, projectName, componentName string, workloadSpec *openchoreov1alpha1.WorkloadSpec) (*openchoreov1alpha1.WorkloadSpec, error) {
	s.logger.Debug("Creating/updating component workload", "org", orgName, "project", projectName, "component", componentName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Verify component exists and belongs to the project
	component := &openchoreov1alpha1.Component{}
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

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "org", orgName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Check if workload already exists
	workloadList := &openchoreov1alpha1.WorkloadList{}
	if err := s.k8sClient.List(ctx, workloadList, client.InNamespace(orgName)); err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	var existingWorkload *openchoreov1alpha1.Workload
	for i := range workloadList.Items {
		workload := &workloadList.Items[i]
		if workload.Spec.Owner.ComponentName == componentName {
			existingWorkload = workload
			break
		}
	}

	var workloadName string

	if existingWorkload != nil {
		// Update existing workload
		existingWorkload.Spec = *workloadSpec
		if err := s.k8sClient.Update(ctx, existingWorkload); err != nil {
			s.logger.Error("Failed to update workload", "error", err)
			return nil, fmt.Errorf("failed to update workload: %w", err)
		}
		s.logger.Debug("Updated existing workload", "name", existingWorkload.Name, "namespace", orgName)
		workloadName = existingWorkload.Name
	} else {
		// Create new workload
		workloadName = componentName + "-workload"
		workload := &openchoreov1alpha1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadName,
				Namespace: orgName,
			},
			Spec: *workloadSpec,
		}

		// Ensure the workload has the correct owner
		workload.Spec.Owner = openchoreov1alpha1.WorkloadOwner{
			ProjectName:   projectName,
			ComponentName: componentName,
		}

		if err := s.k8sClient.Create(ctx, workload); err != nil {
			s.logger.Error("Failed to create workload", "error", err)
			return nil, fmt.Errorf("failed to create workload: %w", err)
		}
		s.logger.Debug("Created new workload", "name", workload.Name, "namespace", orgName)
	}

	// Create the appropriate type-specific resource based on component type if it doesn't exist
	if err := s.createTypeSpecificResource(ctx, orgName, projectName, componentName, workloadName, component.Spec.Type); err != nil {
		s.logger.Error("Failed to create type-specific resource", "componentType", component.Spec.Type, "error", err)
		return nil, fmt.Errorf("failed to create type-specific resource: %w", err)
	}

	return workloadSpec, nil
}

// createTypeSpecificResource creates the appropriate resource (Service, WebApplication, or ScheduledTask) based on component type
func (s *ComponentService) createTypeSpecificResource(ctx context.Context, orgName, projectName, componentName, workloadName string, componentType openchoreov1alpha1.DefinedComponentType) error {
	switch componentType {
	case openchoreov1alpha1.ComponentTypeService:
		return s.createServiceResource(ctx, orgName, projectName, componentName, workloadName)
	case openchoreov1alpha1.ComponentTypeWebApplication:
		return s.createWebApplicationResource(ctx, orgName, projectName, componentName, workloadName)
	case openchoreov1alpha1.ComponentTypeScheduledTask:
		return s.createScheduledTaskResource(ctx, orgName, projectName, componentName, workloadName)
	default:
		s.logger.Debug("No type-specific resource needed for component type", "componentType", componentType)
		return nil
	}
}

// createServiceResource creates a Service resource for Service components
func (s *ComponentService) createServiceResource(ctx context.Context, orgName, projectName, componentName, workloadName string) error {
	// Check if service already exists
	serviceList := &openchoreov1alpha1.ServiceList{}
	if err := s.k8sClient.List(ctx, serviceList, client.InNamespace(orgName)); err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	// Check if service already exists for this component
	for _, service := range serviceList.Items {
		if service.Spec.Owner.ComponentName == componentName && service.Spec.Owner.ProjectName == projectName {
			s.logger.Debug("Service already exists for component", "service", service.Name, "component", componentName)
			return nil
		}
	}

	// Create new service
	service := &openchoreov1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName + "-service",
			Namespace: orgName,
		},
		Spec: openchoreov1alpha1.ServiceSpec{
			Owner: openchoreov1alpha1.ServiceOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkloadName: workloadName,
			ClassName:    defaultPipeline,
		},
	}

	if err := s.k8sClient.Create(ctx, service); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	s.logger.Debug("Created service for component", "service", service.Name, "component", componentName, "workload", workloadName)
	return nil
}

// createWebApplicationResource creates a WebApplication resource for WebApplication components
func (s *ComponentService) createWebApplicationResource(ctx context.Context, orgName, projectName, componentName, workloadName string) error {
	// Check if web application already exists
	webAppList := &openchoreov1alpha1.WebApplicationList{}
	if err := s.k8sClient.List(ctx, webAppList, client.InNamespace(orgName)); err != nil {
		return fmt.Errorf("failed to list web applications: %w", err)
	}

	// Check if web application already exists for this component
	for _, webApp := range webAppList.Items {
		if webApp.Spec.Owner.ComponentName == componentName && webApp.Spec.Owner.ProjectName == projectName {
			s.logger.Debug("WebApplication already exists for component", "webApp", webApp.Name, "component", componentName)
			return nil
		}
	}

	// Create new web application
	webApp := &openchoreov1alpha1.WebApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName + "-webapp",
			Namespace: orgName,
		},
		Spec: openchoreov1alpha1.WebApplicationSpec{
			Owner: openchoreov1alpha1.WebApplicationOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkloadName: workloadName,
			ClassName:    defaultPipeline,
		},
	}

	if err := s.k8sClient.Create(ctx, webApp); err != nil {
		return fmt.Errorf("failed to create web application: %w", err)
	}

	s.logger.Debug("Created web application for component", "webApp", webApp.Name, "component", componentName, "workload", workloadName)
	return nil
}

// createScheduledTaskResource creates a ScheduledTask resource for ScheduledTask components
func (s *ComponentService) createScheduledTaskResource(ctx context.Context, orgName, projectName, componentName, workloadName string) error {
	// Check if scheduled task already exists
	scheduledTaskList := &openchoreov1alpha1.ScheduledTaskList{}
	if err := s.k8sClient.List(ctx, scheduledTaskList, client.InNamespace(orgName)); err != nil {
		return fmt.Errorf("failed to list scheduled tasks: %w", err)
	}

	// Check if scheduled task already exists for this component
	for _, scheduledTask := range scheduledTaskList.Items {
		if scheduledTask.Spec.Owner.ComponentName == componentName && scheduledTask.Spec.Owner.ProjectName == projectName {
			s.logger.Debug("ScheduledTask already exists for component", "scheduledTask", scheduledTask.Name, "component", componentName)
			return nil
		}
	}

	// Create new scheduled task
	scheduledTask := &openchoreov1alpha1.ScheduledTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName + "-task",
			Namespace: orgName,
		},
		Spec: openchoreov1alpha1.ScheduledTaskSpec{
			Owner: openchoreov1alpha1.ScheduledTaskOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkloadName: workloadName,
			ClassName:    defaultPipeline,
		},
	}

	if err := s.k8sClient.Create(ctx, scheduledTask); err != nil {
		return fmt.Errorf("failed to create scheduled task: %w", err)
	}

	s.logger.Debug("Created scheduled task for component", "scheduledTask", scheduledTask.Name, "component", componentName, "workload", workloadName)
	return nil
}

// UpdateComponentWorkflowSchema updates the workflow schema for a component
func (s *ComponentService) UpdateComponentWorkflowSchema(ctx context.Context, orgName, projectName, componentName string, req *models.UpdateWorkflowSchemaRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Updating component workflow schema", "org", orgName, "project", projectName, "component", componentName)

	// Verify project exists
	_, err := s.projectService.GetProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "org", orgName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Get the component
	componentKey := client.ObjectKey{
		Name:      componentName,
		Namespace: orgName,
	}
	component := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, componentKey, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "org", orgName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to the project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "org", orgName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return nil, ErrComponentNotFound
	}

	// Check if component has workflow configuration
	if component.Spec.Workflow == nil {
		s.logger.Warn("Component does not have workflow configuration", "org", orgName, "project", projectName, "component", componentName)
		return nil, fmt.Errorf("component does not have workflow configuration")
	}

	// Validate the schema against the Workflow CRD
	if err := s.validateWorkflowSchema(ctx, orgName, component.Spec.Workflow.Name, req.Schema); err != nil {
		s.logger.Warn("Invalid workflow schema", "error", err, "workflow", component.Spec.Workflow.Name)
		return nil, ErrWorkflowSchemaInvalid
	}

	// Update the workflow schema
	component.Spec.Workflow.Schema = req.Schema

	// Update the component in Kubernetes
	if err := s.k8sClient.Update(ctx, component); err != nil {
		s.logger.Error("Failed to update component", "error", err)
		return nil, fmt.Errorf("failed to update component: %w", err)
	}

	s.logger.Debug("Updated component workflow schema successfully", "org", orgName, "project", projectName, "component", componentName)

	// Return the updated component
	return s.GetComponent(ctx, orgName, projectName, componentName, []string{})
}

// validateWorkflowSchema validates the provided schema against the Workflow CRD's schema definition
func (s *ComponentService) validateWorkflowSchema(ctx context.Context, orgName, workflowName string, providedSchema *runtime.RawExtension) error {
	// Fetch the Workflow CR
	workflowKey := client.ObjectKey{
		Name:      workflowName,
		Namespace: orgName,
	}
	workflow := &openchoreov1alpha1.Workflow{}
	if err := s.k8sClient.Get(ctx, workflowKey, workflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workflow not found", "org", orgName, "workflow", workflowName)
			return ErrWorkflowNotFound
		}
		s.logger.Error("Failed to get workflow", "error", err)
		return fmt.Errorf("failed to get workflow: %w", err)
	}

	// If workflow has no schema defined, any schema is valid
	if workflow.Spec.Schema == nil {
		return nil
	}

	// If provided schema is nil or empty, it's valid (defaults will be applied)
	if providedSchema == nil || len(providedSchema.Raw) == 0 {
		return nil
	}

	// Unmarshal the workflow's schema definition
	var workflowSchemaMap map[string]any
	if err := json.Unmarshal(workflow.Spec.Schema.Raw, &workflowSchemaMap); err != nil {
		s.logger.Error("Failed to unmarshal workflow schema", "error", err)
		return fmt.Errorf("failed to parse workflow schema definition: %w", err)
	}

	// Unmarshal the provided schema values
	var providedValues map[string]any
	if err := json.Unmarshal(providedSchema.Raw, &providedValues); err != nil {
		s.logger.Error("Failed to unmarshal provided schema", "error", err)
		return fmt.Errorf("failed to parse provided schema: %w", err)
	}

	// Build structural schema from workflow schema definition
	structural, err := s.buildWorkflowStructuralSchema(workflowSchemaMap)
	if err != nil {
		s.logger.Error("Failed to build structural schema", "error", err)
		return fmt.Errorf("failed to build workflow schema structure: %w", err)
	}

	// Validate the provided values against the structural schema
	if err := openchoreoschema.ValidateAgainstSchema(providedValues, structural); err != nil {
		return err
	}

	return nil
}

// buildWorkflowStructuralSchema converts a workflow schema map to a structural schema
func (s *ComponentService) buildWorkflowStructuralSchema(workflowSchemaMap map[string]any) (*schema.Structural, error) {
	// Import the schema package if not already imported
	// The workflow schema uses the same format as ComponentType schemas
	def := openchoreoschema.Definition{
		Schemas: []map[string]any{workflowSchemaMap},
	}

	structural, err := openchoreoschema.ToStructural(def)
	if err != nil {
		return nil, fmt.Errorf("failed to convert workflow schema: %w", err)
	}

	return structural, nil
}
