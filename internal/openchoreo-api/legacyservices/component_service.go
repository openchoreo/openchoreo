// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
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
	authzPDP            authz.PDP
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

// fetchComponentTypeSpec fetches the ComponentTypeSpec from the cluster based on the ComponentTypeRef.
// Returns nil (no error) if the referenced resource is not found.
func (s *ComponentService) fetchComponentTypeSpec(ctx context.Context, ctRef *openchoreov1alpha1.ComponentTypeRef, namespaceName string) (*openchoreov1alpha1.ComponentTypeSpec, error) {
	componentTypeName, err := s.parseComponentTypeName(ctRef.Name)
	if err != nil {
		s.logger.Error("Invalid ComponentType format", "componentType", ctRef.Name, "error", err)
		return nil, err
	}

	switch ctRef.Kind {
	case openchoreov1alpha1.ComponentTypeRefKindClusterComponentType:
		cct := &openchoreov1alpha1.ClusterComponentType{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: componentTypeName}, cct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("ClusterComponentType not found", "componentType", ctRef.Name)
				return nil, nil
			}
			s.logger.Error("Failed to get ClusterComponentType", "error", err)
			return nil, err
		}
		// Convert ClusterTraitRef to TraitRef for allowedTraits
		allowedTraits := make([]openchoreov1alpha1.TraitRef, len(cct.Spec.AllowedTraits))
		for i, ref := range cct.Spec.AllowedTraits {
			allowedTraits[i] = openchoreov1alpha1.TraitRef{
				Kind: openchoreov1alpha1.TraitRefKind(ref.Kind),
				Name: ref.Name,
			}
		}
		// Convert ClusterComponentTypeTrait to ComponentTypeTrait
		traits := make([]openchoreov1alpha1.ComponentTypeTrait, len(cct.Spec.Traits))
		for i, t := range cct.Spec.Traits {
			traits[i] = openchoreov1alpha1.ComponentTypeTrait{
				Kind:         openchoreov1alpha1.TraitRefKind(t.Kind),
				Name:         t.Name,
				InstanceName: t.InstanceName,
				Parameters:   t.Parameters,
				EnvOverrides: t.EnvOverrides,
			}
		}
		spec := openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType:     cct.Spec.WorkloadType,
			AllowedWorkflows: cct.Spec.AllowedWorkflows,
			Schema:           cct.Spec.Schema,
			Traits:           traits,
			AllowedTraits:    allowedTraits,
			Validations:      cct.Spec.Validations,
			Resources:        cct.Spec.Resources,
		}
		return &spec, nil
	default:
		ct := &openchoreov1alpha1.ComponentType{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: componentTypeName, Namespace: namespaceName}, ct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("ComponentType not found", "componentType", ctRef.Name)
				return nil, nil
			}
			s.logger.Error("Failed to get ComponentType", "error", err)
			return nil, err
		}
		return &ct.Spec, nil
	}
}

// fetchTraitSpec fetches a TraitSpec from the cluster based on the trait kind and name.
// For ClusterTrait, converts ClusterTraitSpec to TraitSpec for downstream compatibility.
// Returns nil (no error) if the referenced resource is not found.
func (s *ComponentService) fetchTraitSpec(ctx context.Context, kind openchoreov1alpha1.TraitRefKind, name, namespaceName string) (*openchoreov1alpha1.TraitSpec, error) {
	switch kind {
	case openchoreov1alpha1.TraitRefKindClusterTrait:
		ct := &openchoreov1alpha1.ClusterTrait{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, ct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil, nil
			}
			return nil, err
		}
		// Convert ClusterTraitSpec to TraitSpec
		return &openchoreov1alpha1.TraitSpec{
			Schema:  ct.Spec.Schema,
			Creates: ct.Spec.Creates,
			Patches: ct.Spec.Patches,
		}, nil
	default:
		trait := &openchoreov1alpha1.Trait{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespaceName}, trait); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil, nil
			}
			return nil, err
		}
		return &trait.Spec, nil
	}
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
	NamespaceName string `json:"namespaceName"`
}

// NewComponentService creates a new component service
func NewComponentService(k8sClient client.Client, projectService *ProjectService, logger *slog.Logger, authzPDP authz.PDP) *ComponentService {
	return &ComponentService{
		k8sClient:           k8sClient,
		projectService:      projectService,
		specFetcherRegistry: NewComponentSpecFetcherRegistry(),
		logger:              logger,
		authzPDP:            authzPDP,
	}
}

func (s *ComponentService) CreateComponentRelease(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (*models.ComponentReleaseResponse, error) {
	s.logger.Debug("Creating component release", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateComponentRelease, ResourceTypeComponentRelease, releaseName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}
	component := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, componentKey, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to the project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return nil, ErrComponentNotFound
	}

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
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
		s.logger.Warn("Workload not found", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrWorkloadNotFound
	}

	// Generate release name if not provided
	if releaseName == "" {
		generatedName, err := s.generateReleaseName(ctx, namespaceName, projectName, componentName)
		if err != nil {
			return nil, err
		}
		releaseName = generatedName
	}

	// Get ComponentType
	var componentTypeSpec *openchoreov1alpha1.ComponentTypeSpec
	spec, err := s.fetchComponentTypeSpec(ctx, &component.Spec.ComponentType, namespaceName)
	if err != nil {
		return nil, err
	}
	componentTypeSpec = spec

	traits := make(map[string]openchoreov1alpha1.TraitSpec)
	// traitKindByName tracks which Kind claimed each trait Name so we can detect
	// cross-kind collisions (e.g. Trait "x" vs ClusterTrait "x") that would
	// silently overwrite each other in the traits map.
	traitKindByName := make(map[string]openchoreov1alpha1.TraitRefKind)
	for _, componentTrait := range component.Spec.Traits {
		kind := componentTrait.Kind
		if kind == "" {
			kind = openchoreov1alpha1.TraitRefKindTrait
		}
		if prevKind, exists := traitKindByName[componentTrait.Name]; exists && prevKind != kind {
			s.logger.Error("Trait name collision across kinds",
				"trait", componentTrait.Name, "existingKind", prevKind, "newKind", kind)
			return nil, fmt.Errorf("trait name %q is referenced as both %s and %s; trait names must be unique across kinds",
				componentTrait.Name, prevKind, kind)
		}
		traitSpec, err := s.fetchTraitSpec(ctx, kind, componentTrait.Name, namespaceName)
		if err != nil {
			s.logger.Error("Failed to get Trait", "kind", kind, "trait", componentTrait.Name, "error", err)
			continue
		}
		if traitSpec == nil {
			s.logger.Warn("Trait not found", "kind", kind, "trait", componentTrait.Name)
			continue
		}
		traitKindByName[componentTrait.Name] = kind
		traits[componentTrait.Name] = *traitSpec
	}

	// Build ComponentProfile from Component parameters (only if there's content)
	var componentProfile *openchoreov1alpha1.ComponentProfile
	if component.Spec.Parameters != nil || len(component.Spec.Traits) > 0 {
		componentProfile = &openchoreov1alpha1.ComponentProfile{}
		if component.Spec.Parameters != nil {
			componentProfile.Parameters = component.Spec.Parameters
		}
		if component.Spec.Traits != nil {
			componentProfile.Traits = component.Spec.Traits
		}
	}

	// Build workload template spec from workload spec
	workloadTemplateSpec := openchoreov1alpha1.WorkloadTemplateSpec{
		Containers: workload.Spec.Containers,
		Endpoints:  workload.Spec.Endpoints,
	}

	componentRelease := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      releaseName,
			Namespace: namespaceName,
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

	s.logger.Debug("ComponentRelease created successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
	return &models.ComponentReleaseResponse{
		Name:          releaseName,
		ComponentName: componentName,
		ProjectName:   projectName,
		NamespaceName: namespaceName,
		CreatedAt:     componentRelease.CreationTimestamp.Time,
		Status:        statusReady,
	}, nil
}

// generateReleaseName generates a unique release name for a component
// Format: <component_name>-<date>-<number>
// Example: my-component-20240118-1
func (s *ComponentService) generateReleaseName(ctx context.Context, namespaceName, projectName, componentName string) (string, error) {
	// List existing releases for this component
	releaseList := &openchoreov1alpha1.ComponentReleaseList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
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
func (s *ComponentService) ListComponentReleases(ctx context.Context, namespaceName, projectName, componentName string) ([]*models.ComponentReleaseResponse, error) {
	s.logger.Debug("Listing component releases", "namespace", namespaceName, "project", projectName, "component", componentName)

	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	var releaseList openchoreov1alpha1.ComponentReleaseList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
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
		// Authorization check for each release
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentRelease, ResourceTypeComponentRelease, item.Name,
			authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized releases
				s.logger.Debug("Skipping unauthorized component release", "namespace", namespaceName, "project", projectName, "component", componentName, "release", item.Name)
				continue
			}
			// system failures, return the error
			return nil, err
		}
		releases = append(releases, &models.ComponentReleaseResponse{
			Name:          item.Name,
			ComponentName: componentName,
			ProjectName:   projectName,
			NamespaceName: namespaceName,
			CreatedAt:     item.CreationTimestamp.Time,
			Status:        statusReady,
		})
	}

	s.logger.Debug("Listed component releases", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(releases))
	return releases, nil
}

// GetComponentRelease retrieves a specific component release by its name
func (s *ComponentService) GetComponentRelease(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (*models.ComponentReleaseResponse, error) {
	s.logger.Debug("Getting component release", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentRelease, ResourceTypeComponentRelease, releaseName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	releaseKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      releaseName,
	}
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, releaseKey, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component release not found", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
			return nil, ErrComponentReleaseNotFound
		}
		s.logger.Error("Failed to get component release", "error", err)
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	if release.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Component release does not belong to component", "namespace", namespaceName, "component", componentName, "release", releaseName)
		return nil, ErrComponentReleaseNotFound
	}

	s.logger.Debug("Retrieved component release", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
	return &models.ComponentReleaseResponse{
		Name:          release.Name,
		ComponentName: componentName,
		ProjectName:   projectName,
		NamespaceName: namespaceName,
		CreatedAt:     release.CreationTimestamp.Time,
		Status:        statusReady, // ComponentRelease is immutable, so it's always ready once created
	}, nil
}

// GetComponentReleaseSchema retrieves the JSON schema for a ComponentRelease
func (s *ComponentService) GetComponentReleaseSchema(ctx context.Context, namespaceName, projectName, componentName, releaseName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting component release schema", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponentRelease, ResourceTypeComponentRelease, releaseName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	releaseKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      releaseName,
	}
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, releaseKey, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component release not found", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName)
			return nil, ErrComponentReleaseNotFound
		}
		s.logger.Error("Failed to get component release", "error", err)
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	if release.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Component release does not belong to component", "namespace", namespaceName, "component", componentName, "release", releaseName)
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
	if release.Spec.ComponentProfile != nil {
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
	}

	if len(traitSchemas) > 0 {
		wrappedSchema.Properties["traitOverrides"] = extv1.JSONSchemaProps{
			Type:       "object",
			Properties: traitSchemas,
		}
	}

	s.logger.Debug("Retrieved component release schema successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "release", releaseName, "hasComponentTypeEnvOverrides", componentTypeEnvOverrides != nil, "traitCount", len(traitSchemas))
	return wrappedSchema, nil
}

// GetComponentSchema retrieves the JSON schema for a Component using the latest ComponentType
func (s *ComponentService) GetComponentSchema(ctx context.Context, namespaceName, projectName, componentName string) (*extv1.JSONSchemaProps, error) {
	s.logger.Debug("Getting component schema", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Get the component
	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Parse ComponentType name from format: {workloadType}/{componentTypeName}
	ctName, err := s.parseComponentTypeName(component.Spec.ComponentType.Name)
	if err != nil {
		s.logger.Error("Invalid component type format", "componentType", component.Spec.ComponentType.Name, "error", err)
		return nil, err
	}

	// Get the latest ComponentType or ClusterComponentType
	var ct openchoreov1alpha1.ComponentType
	switch component.Spec.ComponentType.Kind {
	case openchoreov1alpha1.ComponentTypeRefKindClusterComponentType:
		var cct openchoreov1alpha1.ClusterComponentType
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: ctName}, &cct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("ClusterComponentType not found", "name", ctName)
				return nil, ErrComponentTypeNotFound
			}
			s.logger.Error("Failed to get ClusterComponentType", "error", err)
			return nil, fmt.Errorf("failed to get ClusterComponentType: %w", err)
		}
		ct = openchoreov1alpha1.ComponentType{
			ObjectMeta: cct.ObjectMeta,
			Spec: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType:     cct.Spec.WorkloadType,
				AllowedWorkflows: cct.Spec.AllowedWorkflows,
				Schema:           cct.Spec.Schema,
				Resources:        cct.Spec.Resources,
			},
		}
	default:
		ctKey := client.ObjectKey{
			Namespace: namespaceName,
			Name:      ctName,
		}
		if err := s.k8sClient.Get(ctx, ctKey, &ct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("ComponentType not found", "namespace", namespaceName, "name", ctName)
				return nil, ErrComponentTypeNotFound
			}
			s.logger.Error("Failed to get ComponentType", "error", err)
			return nil, fmt.Errorf("failed to get ComponentType: %w", err)
		}
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
		traitSpec, err := s.fetchTraitSpec(ctx, componentTrait.Kind, componentTrait.Name, namespaceName)
		if err != nil {
			s.logger.Error("Failed to get trait", "kind", componentTrait.Kind, "trait", componentTrait.Name, "error", err)
			return nil, fmt.Errorf("failed to get trait %s: %w", componentTrait.Name, err)
		}
		if traitSpec == nil {
			s.logger.Warn("Trait not found", "kind", componentTrait.Kind, "namespace", namespaceName, "trait", componentTrait.Name)
			continue // Skip missing traits instead of failing
		}

		traitJSONSchema, err := s.buildTraitEnvOverridesSchema(*traitSpec, componentTrait.Name)
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

	s.logger.Debug("Retrieved component schema successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "hasComponentTypeEnvOverrides", envOverrides != nil, "traitCount", len(traitSchemas))
	return wrappedSchema, nil
}

// GetEnvironmentRelease retrieves the Release spec and status for a given component and environment
// Returns the full Release spec and status including resources, owner, environment information, and conditions
func (s *ComponentService) GetEnvironmentRelease(ctx context.Context, namespaceName, projectName, componentName, environmentName string) (*models.ReleaseResponse, error) {
	s.logger.Debug("Getting release", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environmentName)

	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	var releaseList openchoreov1alpha1.ReleaseList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName:   namespaceName,
			labels.LabelKeyProjectName:     projectName,
			labels.LabelKeyComponentName:   componentName,
			labels.LabelKeyEnvironmentName: environmentName,
		},
	}

	if err := s.k8sClient.List(ctx, &releaseList, listOpts...); err != nil {
		s.logger.Error("Failed to list releases", "error", err)
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	if len(releaseList.Items) == 0 {
		s.logger.Warn("No release found", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environmentName)
		return nil, ErrReleaseNotFound
	}

	// Get the first matching Release (there should only be one per component/environment)
	release := &releaseList.Items[0]

	s.logger.Debug("Retrieved release successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environmentName, "resourceCount", len(release.Spec.Resources))
	return &models.ReleaseResponse{
		Spec:   release.Spec,
		Status: release.Status,
	}, nil
}

// convertEnvVars converts environment variables from the request model to the CR model
func (s *ComponentService) convertEnvVars(envVars []models.EnvVar) []openchoreov1alpha1.EnvVar {
	result := make([]openchoreov1alpha1.EnvVar, len(envVars))
	for i, env := range envVars {
		envVar := openchoreov1alpha1.EnvVar{
			Key:   env.Key,
			Value: env.Value,
		}

		if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
			envVar.ValueFrom = &openchoreov1alpha1.EnvVarValueFrom{
				SecretRef: &openchoreov1alpha1.SecretKeyRef{
					Name: env.ValueFrom.SecretRef.Name,
					Key:  env.ValueFrom.SecretRef.Key,
				},
			}
		}

		result[i] = envVar
	}
	return result
}

// convertFileVars converts file variables from the request model to the CR model
func (s *ComponentService) convertFileVars(fileVars []models.FileVar, containerName string) []openchoreov1alpha1.FileVar {
	result := make([]openchoreov1alpha1.FileVar, len(fileVars))
	for i, file := range fileVars {
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

		if file.ValueFrom != nil && file.ValueFrom.SecretRef != nil {
			fileVar.ValueFrom = &openchoreov1alpha1.EnvVarValueFrom{
				SecretRef: &openchoreov1alpha1.SecretKeyRef{
					Name: file.ValueFrom.SecretRef.Name,
					Key:  file.ValueFrom.SecretRef.Key,
				},
			}
		}

		result[i] = fileVar
	}
	return result
}

// applyWorkloadOverrides applies workload overrides to the release binding
func (s *ComponentService) applyWorkloadOverrides(binding *openchoreov1alpha1.ReleaseBinding, req *models.PatchReleaseBindingRequest) {
	if req.WorkloadOverrides == nil {
		return
	}

	containers := make(map[string]openchoreov1alpha1.ContainerOverride)
	for containerName, containerOverride := range req.WorkloadOverrides.Containers {
		containers[containerName] = openchoreov1alpha1.ContainerOverride{
			Env:   s.convertEnvVars(containerOverride.Env),
			Files: s.convertFileVars(containerOverride.Files, containerName),
		}
	}

	binding.Spec.WorkloadOverrides = &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
		Containers: containers,
	}
}

// PatchReleaseBinding patches a ReleaseBinding with environment-specific overrides
func (s *ComponentService) PatchReleaseBinding(ctx context.Context, namespaceName, projectName, componentName, bindingName string, req *models.PatchReleaseBindingRequest) (*models.ReleaseBindingResponse, error) {
	s.logger.Debug("Patching release binding", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionUpdateReleaseBinding, ResourceTypeReleaseBinding, bindingName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	bindingKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      bindingName,
	}
	var binding openchoreov1alpha1.ReleaseBinding
	bindingExists := true

	err = s.k8sClient.Get(ctx, bindingKey, &binding)
	// Return early for non-NotFound errors
	if err != nil && client.IgnoreNotFound(err) != nil {
		s.logger.Error("Failed to get release binding", "error", err)
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	// Handle binding not found - create new one
	if err != nil {
		bindingExists = false
		s.logger.Debug("Release binding not found, will create new one", "namespace", namespaceName, "binding", bindingName)

		if req.Environment == "" {
			s.logger.Warn("Environment is required when creating a new release binding")
			return nil, fmt.Errorf("environment is required when creating a new release binding")
		}

		binding = openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: namespaceName,
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
	}

	// Verify the binding belongs to the correct component (only if it already exists)
	if bindingExists && binding.Spec.Owner.ComponentName != componentName {
		s.logger.Warn("Release binding does not belong to component", "namespace", namespaceName, "component", componentName, "binding", bindingName)
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

	s.applyWorkloadOverrides(&binding, req)

	// Create or update the binding
	if bindingExists {
		if err := s.k8sClient.Update(ctx, &binding); err != nil {
			s.logger.Error("Failed to update release binding", "error", err)
			return nil, fmt.Errorf("failed to update release binding: %w", err)
		}
		s.logger.Debug("Release binding updated successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)
	} else {
		if err := s.k8sClient.Create(ctx, &binding); err != nil {
			s.logger.Error("Failed to create release binding", "error", err)
			return nil, fmt.Errorf("failed to create release binding: %w", err)
		}
		s.logger.Debug("Release binding created successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)
	}

	return s.toReleaseBindingResponse(&binding, namespaceName, projectName, componentName), nil
}

// toReleaseBindingResponse converts a ReleaseBinding CR to a ReleaseBindingResponse
func (s *ComponentService) toReleaseBindingResponse(binding *openchoreov1alpha1.ReleaseBinding, namespaceName, projectName, componentName string) *models.ReleaseBindingResponse {
	response := &models.ReleaseBindingResponse{
		Name:          binding.Name,
		ComponentName: componentName,
		ProjectName:   projectName,
		NamespaceName: namespaceName,
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
func (s *ComponentService) ListReleaseBindings(ctx context.Context, namespaceName, projectName, componentName string, environments []string) ([]*models.ReleaseBindingResponse, error) {
	s.logger.Debug("Listing release bindings", "namespace", namespaceName, "project", projectName, "component", componentName, "environments", environments)

	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	var bindingList openchoreov1alpha1.ReleaseBindingList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
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

		// Authorization check for each release binding
		if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewReleaseBinding, ResourceTypeReleaseBinding, binding.Name,
			authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
			if errors.Is(err, ErrForbidden) {
				// Skip unauthorized release bindings
				s.logger.Debug("Skipping unauthorized release binding", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", binding.Name)
				continue
			}
			// Return non-forbidden errors
			return nil, err
		}

		bindings = append(bindings, s.toReleaseBindingResponse(binding, namespaceName, projectName, componentName))
	}

	s.logger.Debug("Listed release bindings", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(bindings))
	return bindings, nil
}

// DeployRelease deploys a component release to the lowest environment in the deployment pipeline
func (s *ComponentService) DeployRelease(ctx context.Context, namespaceName, projectName, componentName string, req *models.DeployReleaseRequest) (*models.ReleaseBindingResponse, error) {
	s.logger.Debug("Deploying release", "namespace", namespaceName, "project", projectName, "component", componentName, "release", req.ReleaseName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionDeployComponent, ResourceTypeComponent, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	project, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	pipelineName := project.DeploymentPipeline
	if pipelineName == "" {
		s.logger.Warn("Project has no deployment pipeline", "namespace", namespaceName, "project", projectName)
		return nil, fmt.Errorf("project has no deployment pipeline configured")
	}

	pipelineKey := client.ObjectKey{
		Namespace: namespaceName,
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
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component does not belong to project", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	releaseKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      req.ReleaseName,
	}
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, releaseKey, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component release not found", "namespace", namespaceName, "release", req.ReleaseName)
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
		Namespace: namespaceName,
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
				Namespace: namespaceName,
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

	s.logger.Debug("Release deployed successfully", "namespace", namespaceName, "project", projectName, "component", componentName, "release", req.ReleaseName, "environment", lowestEnv)
	return s.toReleaseBindingResponse(&binding, namespaceName, projectName, componentName), nil
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
func (s *ComponentService) CreateComponent(ctx context.Context, namespaceName, projectName string, req *models.CreateComponentRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Creating component", "namespace", namespaceName, "project", projectName, "component", req.Name)

	// Sanitize input
	req.Sanitize()

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateComponent, ResourceTypeComponent, req.Name,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: req.Name}); err != nil {
		return nil, err
	}

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Check if component already exists
	exists, err := s.componentExists(ctx, namespaceName, projectName, req.Name)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if exists {
		s.logger.Warn("Component already exists", "namespace", namespaceName, "project", projectName, "component", req.Name)
		return nil, ErrComponentAlreadyExists
	}

	// Create the component and related resources
	component, err := s.createComponentResources(ctx, namespaceName, projectName, req)
	if err != nil {
		s.logger.Error("Failed to create component resources", "error", err)
		return nil, fmt.Errorf("failed to create component: %w", err)
	}

	s.logger.Debug("Component created successfully", "namespace", namespaceName, "project", projectName, "component", req.Name)

	// Return the created component
	return &models.ComponentResponse{
		UID:           string(component.UID),
		Name:          component.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		Type:          req.ComponentType.Name,
		ProjectName:   projectName,
		NamespaceName: namespaceName,
		CreatedAt:     component.CreationTimestamp.Time,
		Status:        "Created",
	}, nil
}

// DeleteComponent deletes a component from the given project
func (s *ComponentService) DeleteComponent(ctx context.Context, namespaceName, projectName, componentName string) error {
	s.logger.Debug("Deleting component", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionDeleteComponent, ResourceTypeComponent, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return err
	}

	// Get the component first to ensure it exists and belongs to the project
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return ErrComponentNotFound
	}

	// Delete the component CR
	if err := s.k8sClient.Delete(ctx, component); err != nil {
		s.logger.Error("Failed to delete component CR", "error", err)
		return fmt.Errorf("failed to delete component: %w", err)
	}

	s.logger.Debug("Component deleted successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	return nil
}

// ListComponents lists all components in the given project
func (s *ComponentService) ListComponents(ctx context.Context, namespaceName, projectName string) ([]*models.ComponentResponse, error) {
	s.logger.Debug("Listing components", "namespace", namespaceName, "project", projectName)

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	var componentList openchoreov1alpha1.ComponentList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}

	if err := s.k8sClient.List(ctx, &componentList, listOpts...); err != nil {
		s.logger.Error("Failed to list components", "error", err)
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	components := make([]*models.ComponentResponse, 0, len(componentList.Items))
	for _, item := range componentList.Items {
		// Only include components that belong to the specified project
		if item.Spec.Owner.ProjectName == projectName {
			// Authorization check for each component
			if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponent, ResourceTypeComponent, item.Name,
				authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: item.Name}); err != nil {
				if errors.Is(err, ErrForbidden) {
					// Skip unauthorized components
					s.logger.Debug("Skipping unauthorized component", "namespace", namespaceName, "project", projectName, "component", item.Name)
					continue
				}
				// system failures, return the error
				return nil, err
			}
			components = append(components, s.toComponentResponse(&item, make(map[string]interface{}), false))
		}
	}

	s.logger.Debug("Listed components", "namespace", namespaceName, "project", projectName, "count", len(components))
	return components, nil
}

// GetComponent retrieves a specific component
func (s *ComponentService) GetComponent(ctx context.Context, namespaceName, projectName, componentName string, additionalResources []string) (*models.ComponentResponse, error) {
	s.logger.Debug("Getting component", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponent, ResourceTypeComponent, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
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
			fetcherKey = component.Spec.ComponentType.Name
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

		spec, err := fetcher.FetchSpec(ctx, s.k8sClient, namespaceName, componentName)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn(
					"Resource not found for fetcher",
					"fetcherKey", fetcherKey,
					"namespace", namespaceName,
					"project", projectName,
					"component", componentName,
				)
			} else {
				s.logger.Error(
					"Failed to fetch spec for resource type",
					"fetcherKey", fetcherKey,
					"namespace", namespaceName,
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
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	return s.toComponentResponse(component, typeSpecs, true), nil
}

// PatchComponent patches a Component with the provided updates
func (s *ComponentService) PatchComponent(ctx context.Context, namespaceName, projectName, componentName string,
	req *models.PatchComponentRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Patching component", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionUpdateComponent, ResourceTypeComponent, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	patchBase := component.DeepCopy()

	s.applyComponentPatch(&component.Spec, req)

	// Only patch if there are actual changes
	if !reflect.DeepEqual(patchBase.Spec, component.Spec) {
		patch := client.MergeFrom(patchBase)
		if err := s.k8sClient.Patch(ctx, &component, patch); err != nil {
			s.logger.Error("Failed to patch component", "error", err)
			return nil, fmt.Errorf("failed to patch component: %w", err)
		}
		s.logger.Debug("Component patched successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	} else {
		s.logger.Debug("No changes detected, returning existing component", "namespace", namespaceName, "project", projectName, "component", componentName)
	}

	return s.toComponentResponse(&component, nil, true), nil
}

// applyComponentPatch applies non-nil fields from PatchComponentRequest to ComponentSpec using reflection
// This automatically handles all pointer fields in the request struct without explicit if checks
// Field names must match exactly between PatchComponentRequest and ComponentSpec
// Example: req.AutoDeploy (pointer) maps to spec.AutoDeploy (value) by matching field name "AutoDeploy"
func (s *ComponentService) applyComponentPatch(spec *openchoreov1alpha1.ComponentSpec, req *models.PatchComponentRequest) {
	reqValue := reflect.ValueOf(req).Elem()
	specValue := reflect.ValueOf(spec).Elem()
	reqType := reqValue.Type()

	for i := 0; i < reqValue.NumField(); i++ {
		reqField := reqValue.Field(i)
		fieldName := reqType.Field(i).Name

		// Skip if the field is not a pointer or is nil
		if reqField.Kind() != reflect.Ptr || reqField.IsNil() {
			continue
		}

		// Find the corresponding field in spec
		specField := specValue.FieldByName(fieldName)
		if !specField.IsValid() || !specField.CanSet() {
			s.logger.Warn("Field not found or cannot be set in ComponentSpec", "field", fieldName)
			continue
		}

		// Set the spec field to the dereferenced request value
		specField.Set(reqField.Elem())
		s.logger.Debug("Patched field", "field", fieldName, "value", reqField.Elem().Interface())
	}
}

// componentExists checks if a component already exists by name and namespace and belongs to the specified project
func (s *ComponentService) componentExists(ctx context.Context, namespaceName, projectName, componentName string) (bool, error) {
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
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
func (s *ComponentService) createComponentResources(ctx context.Context, namespaceName, projectName string, req *models.CreateComponentRequest) (*openchoreov1alpha1.Component, error) {
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Name
	}

	annotations := map[string]string{
		controller.AnnotationKeyDisplayName: displayName,
		controller.AnnotationKeyDescription: req.Description,
	}

	componentSpec := openchoreov1alpha1.ComponentSpec{
		Owner: openchoreov1alpha1.ComponentOwner{
			ProjectName: projectName,
		},
	}

	if req.ComponentType != nil {
		kind := openchoreov1alpha1.ComponentTypeRefKind(req.ComponentType.Kind)
		if kind == "" {
			kind = openchoreov1alpha1.ComponentTypeRefKindComponentType
		}
		componentSpec.ComponentType = openchoreov1alpha1.ComponentTypeRef{
			Kind: kind,
			Name: req.ComponentType.Name,
		}
	}

	if req.AutoDeploy != nil {
		componentSpec.AutoDeploy = *req.AutoDeploy
	}

	if req.Parameters != nil {
		componentSpec.Parameters = req.Parameters
	}

	if len(req.Traits) > 0 {
		componentSpec.Traits = make([]openchoreov1alpha1.ComponentTrait, len(req.Traits))
		for i, trait := range req.Traits {
			componentSpec.Traits[i] = openchoreov1alpha1.ComponentTrait{
				Kind:         openchoreov1alpha1.TraitRefKind(trait.Kind),
				Name:         trait.Name,
				InstanceName: trait.InstanceName,
				Parameters:   trait.Parameters,
			}
		}
	}

	componentCR := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
		Spec: componentSpec,
	}

	// Set component workflow configuration if provided (new preferred way)
	if req.ComponentWorkflow != nil {
		workflowConfig := &openchoreov1alpha1.ComponentWorkflowRunConfig{
			Name: req.ComponentWorkflow.Name,
		}

		// Set system parameters if provided
		if req.ComponentWorkflow.SystemParameters != nil {
			workflowConfig.SystemParameters = openchoreov1alpha1.SystemParametersValues{
				Repository: openchoreov1alpha1.RepositoryValues{
					URL: req.ComponentWorkflow.SystemParameters.Repository.URL,
					Revision: openchoreov1alpha1.RepositoryRevisionValues{
						Branch: req.ComponentWorkflow.SystemParameters.Repository.Revision.Branch,
						Commit: req.ComponentWorkflow.SystemParameters.Repository.Revision.Commit,
					},
					AppPath: req.ComponentWorkflow.SystemParameters.Repository.AppPath,
				},
			}
		}

		// Set developer parameters if provided
		if req.ComponentWorkflow.Parameters != nil {
			workflowConfig.Parameters = req.ComponentWorkflow.Parameters
		}

		componentCR.Spec.Workflow = workflowConfig
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

	// Convert workflow configuration to API ComponentWorkflow format only if requested
	var componentWorkflow *models.ComponentWorkflow
	if includeWorkflow && component.Spec.Workflow != nil {
		componentWorkflow = &models.ComponentWorkflow{
			Name: component.Spec.Workflow.Name,
			SystemParameters: &models.ComponentWorkflowSystemParams{
				Repository: models.ComponentWorkflowRepository{
					URL:       component.Spec.Workflow.SystemParameters.Repository.URL,
					SecretRef: component.Spec.Workflow.SystemParameters.Repository.SecretRef,
					Revision: models.ComponentWorkflowRepositoryRevision{
						Branch: component.Spec.Workflow.SystemParameters.Repository.Revision.Branch,
						Commit: component.Spec.Workflow.SystemParameters.Repository.Revision.Commit,
					},
					AppPath: component.Spec.Workflow.SystemParameters.Repository.AppPath,
				},
			},
			Parameters: component.Spec.Workflow.Parameters,
		}
	}

	componentType := component.Spec.ComponentType.Name

	// Get deletion timestamp if the component is being deleted
	var deletionTimestamp *time.Time
	if component.DeletionTimestamp != nil {
		t := component.DeletionTimestamp.Time
		deletionTimestamp = &t
	}

	response := &models.ComponentResponse{
		UID:               string(component.UID),
		Name:              component.Name,
		DisplayName:       component.Annotations[controller.AnnotationKeyDisplayName],
		Description:       component.Annotations[controller.AnnotationKeyDescription],
		Type:              componentType,
		AutoDeploy:        component.Spec.AutoDeploy,
		ProjectName:       projectName,
		NamespaceName:     component.Namespace,
		CreatedAt:         component.CreationTimestamp.Time,
		DeletionTimestamp: deletionTimestamp,
		Status:            status,
		ComponentWorkflow: componentWorkflow,
	}

	for _, v := range typeSpecs {
		switch spec := v.(type) {
		case *openchoreov1alpha1.WorkloadSpec:
			response.Workload = spec
		default:
			s.logger.Error("Unknown type in typeSpecs", "component", component.Name, "actualType", fmt.Sprintf("%T", v))
		}
	}

	return response
}

// GetComponentBindings retrieves bindings for a component in multiple environments
// If environments is empty, it will get all environments from the project's deployment pipeline
func (s *ComponentService) GetComponentBindings(ctx context.Context, namespaceName, projectName, componentName string, environments []string) ([]*models.BindingResponse, error) {
	s.logger.Debug("Getting component bindings", "namespace", namespaceName, "project", projectName, "component", componentName, "environments", environments)

	// First get the component to determine its type
	component, err := s.GetComponent(ctx, namespaceName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	// If no environments specified, get all environments from the deployment pipeline
	if len(environments) == 0 {
		pipelineEnvironments, err := s.getEnvironmentsFromDeploymentPipeline(ctx, namespaceName, projectName)
		if err != nil {
			return nil, err
		}
		environments = pipelineEnvironments
		s.logger.Debug("Using environments from deployment pipeline", "environments", environments)
	}

	bindings := make([]*models.BindingResponse, 0, len(environments))
	for _, environment := range environments {
		binding, err := s.getComponentBinding(ctx, namespaceName, projectName, componentName, environment, component.Type)
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
func (s *ComponentService) GetComponentBinding(ctx context.Context, namespaceName, projectName, componentName, environment string) (*models.BindingResponse, error) {
	s.logger.Debug("Getting component binding", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environment)

	// First get the component to determine its type
	component, err := s.GetComponent(ctx, namespaceName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	return s.getComponentBinding(ctx, namespaceName, projectName, componentName, environment, component.Type)
}

// getComponentBinding retrieves the binding for a component in a specific environment
func (s *ComponentService) getComponentBinding(ctx context.Context, namespaceName, projectName, componentName, environment, componentType string) (*models.BindingResponse, error) {
	// Determine binding type based on component type - all legacy types removed
	return nil, fmt.Errorf("legacy component types no longer supported: %s", componentType)
}

// getEnvironmentsFromDeploymentPipeline extracts all environments from the project's deployment pipeline
func (s *ComponentService) getEnvironmentsFromDeploymentPipeline(ctx context.Context, namespaceName, projectName string) ([]string, error) {
	// Get the project to determine the deployment pipeline reference
	project, err := s.projectService.getProject(ctx, namespaceName, projectName)
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
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, pipeline); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Deployment pipeline not found", "namespace", namespaceName, "project", projectName, "pipeline", pipelineName)
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
	s.logger.Debug("Promoting component", "namespace", req.NamespaceName, "project", req.ProjectName, "component", req.ComponentName,
		"source", req.SourceEnvironment, "target", req.TargetEnvironment)

	// Authorization check (promote uses same permission as deploy)
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionDeployComponent, ResourceTypeComponent, req.ComponentName,
		authz.ResourceHierarchy{Namespace: req.NamespaceName, Project: req.ProjectName, Component: req.ComponentName}); err != nil {
		return nil, err
	}

	if err := s.validatePromotionPath(ctx, req.NamespaceName, req.ProjectName, req.SourceEnvironment, req.TargetEnvironment); err != nil {
		return nil, err
	}

	sourceReleaseBinding, err := s.getReleaseBinding(ctx, req.NamespaceName, req.ProjectName, req.ComponentName, req.SourceEnvironment)
	if err != nil {
		return nil, fmt.Errorf("failed to get source release binding: %w", err)
	}

	if err := s.createOrUpdateReleaseBinding(ctx, req, sourceReleaseBinding); err != nil {
		return nil, fmt.Errorf("failed to create/update target release binding: %w", err)
	}

	targetReleaseBinding, err := s.getReleaseBinding(ctx, req.NamespaceName, req.ProjectName, req.ComponentName, req.TargetEnvironment)
	if err != nil {
		return nil, fmt.Errorf("failed to get release binding: %w", err)
	}

	return s.toReleaseBindingResponse(targetReleaseBinding, req.NamespaceName, req.ProjectName, req.ComponentName), nil
}

// validatePromotionPath validates that the promotion path is allowed by the deployment pipeline
func (s *ComponentService) validatePromotionPath(ctx context.Context, namespaceName, projectName, sourceEnv, targetEnv string) error {
	// Get the project to determine the deployment pipeline reference
	project, err := s.projectService.getProject(ctx, namespaceName, projectName)
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
		Namespace: namespaceName,
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
func (s *ComponentService) getReleaseBinding(ctx context.Context, namespaceName, projectName, componentName, environment string) (*openchoreov1alpha1.ReleaseBinding, error) {
	// List all ReleaseBindings in the namespace
	bindingList := &openchoreov1alpha1.ReleaseBindingList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
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
	existingTargetBinding, err := s.getReleaseBinding(ctx, req.NamespaceName, req.ProjectName, req.ComponentName, req.TargetEnvironment)
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
				Namespace: req.NamespaceName,
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
		s.logger.Debug("Created new ReleaseBinding", "name", targetBindingName, "namespace", req.NamespaceName, "environment", req.TargetEnvironment)
	} else {
		// Update existing binding
		if err := s.k8sClient.Update(ctx, targetBinding); err != nil {
			return fmt.Errorf("failed to update target release binding: %w", err)
		}
		s.logger.Debug("Updated existing ReleaseBinding", "name", targetBindingName, "namespace", req.NamespaceName, "environment", req.TargetEnvironment)
	}

	return nil
}

// UpdateComponentBinding updates a component binding
func (s *ComponentService) UpdateComponentBinding(ctx context.Context, namespaceName, projectName, componentName, bindingName string, req *models.UpdateBindingRequest) (*models.BindingResponse, error) {
	s.logger.Debug("Updating component binding", "namespace", namespaceName, "project", projectName, "component", componentName, "binding", bindingName)

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Verify component exists
	exists, err := s.componentExists(ctx, namespaceName, projectName, componentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if !exists {
		s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Get the component type to determine which binding type to update
	component, err := s.GetComponent(ctx, namespaceName, projectName, componentName, []string{})
	if err != nil {
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Update the appropriate binding based on component type - legacy types removed
	return nil, fmt.Errorf("legacy component types no longer supported: %s", component.Type)
}

// ComponentObserverResponse represents the response for observer URL requests
type ComponentObserverResponse struct {
	ObserverURL string `json:"observerUrl,omitempty"`
	Message     string `json:"message,omitempty"`
}

// GetComponentObserverURL retrieves the observer URL for component runtime logs
// NOTE: This function is to be deprecated in favor of the environment service to get the observer URL
func (s *ComponentService) GetComponentObserverURL(ctx context.Context, namespaceName, projectName, componentName, environmentName string) (*ComponentObserverResponse, error) {
	s.logger.Debug("Getting component observer URL", "namespace", namespaceName, "project", projectName, "component", componentName, "environment", environmentName)

	// 1. Verify component exists in project
	_, err := s.GetComponent(ctx, namespaceName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	// 2. Get the environment
	env := &openchoreov1alpha1.Environment{}
	envKey := client.ObjectKey{
		Name:      environmentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, envKey, env); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Environment not found", "namespace", namespaceName, "environment", environmentName)
			return nil, ErrEnvironmentNotFound
		}
		s.logger.Error("Failed to get environment", "error", err, "namespace", namespaceName, "environment", environmentName)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// 3. Check if environment has a dataplane reference
	if env.Spec.DataPlaneRef == nil || env.Spec.DataPlaneRef.Name == "" {
		s.logger.Error("Environment has no dataplane reference", "environment", environmentName)
		return nil, fmt.Errorf("environment %s has no dataplane reference", environmentName)
	}

	// Currently only supporting DataPlane (not ClusterDataPlane) for observer URL
	if env.Spec.DataPlaneRef.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane {
		s.logger.Debug("ClusterDataPlane observer URL not yet supported", "environment", environmentName)
		return &ComponentObserverResponse{
			Message: "observability-logs for ClusterDataPlane not yet supported",
		}, nil
	}

	// 4. Get the DataPlane configuration for the environment
	dp := &openchoreov1alpha1.DataPlane{}
	dpKey := client.ObjectKey{
		Name:      env.Spec.DataPlaneRef.Name,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, dpKey, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Error("DataPlane not found", "namespace", namespaceName, "dataplane", env.Spec.DataPlaneRef.Name)
			return nil, ErrDataPlaneNotFound
		}
		s.logger.Error("Failed to get dataplane", "error", err, "namespace", namespaceName, "dataplane", env.Spec.DataPlaneRef.Name)
		return nil, fmt.Errorf("failed to get dataplane: %w", err)
	}

	// 5. Get ObservabilityPlane via the reference helper
	observabilityResult, err := controller.GetObservabilityPlaneOrClusterObservabilityPlaneOfDataPlane(ctx, s.k8sClient, dp)
	if err != nil {
		// Only treat NotFound as "not configured" - other errors should be returned upstream
		if apierrors.IsNotFound(err) {
			s.logger.Debug("ObservabilityPlane not found", "error", err, "dataplane", dp.Name)
			return &ComponentObserverResponse{
				Message: "observability-logs have not been configured",
			}, nil
		}
		s.logger.Error("Failed to get observability plane", "error", err, "dataplane", dp.Name)
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}

	observerURL := observabilityResult.GetObserverURL()
	if observerURL == "" {
		s.logger.Debug("ObserverURL not configured in observability plane", "observabilityPlane", observabilityResult.GetName())
		return &ComponentObserverResponse{
			Message: "observability-logs have not been configured",
		}, nil
	}

	// 6. Return observer URL
	return &ComponentObserverResponse{
		ObserverURL: observerURL,
	}, nil
}

// GetBuildObserverURL retrieves the observer URL for component build logs
func (s *ComponentService) GetBuildObserverURL(ctx context.Context, namespaceName, projectName, componentName string) (*ComponentObserverResponse, error) {
	s.logger.Debug("Getting build observer URL", "namespace", namespaceName, "project", projectName, "component", componentName)

	// 1. Verify component exists in project
	_, err := s.GetComponent(ctx, namespaceName, projectName, componentName, []string{})
	if err != nil {
		return nil, err
	}

	// 2. Get BuildPlane configuration for the namespace
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	err = s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(namespaceName))
	if err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	// Check if any build planes exist
	if len(buildPlanes.Items) == 0 {
		s.logger.Error("No build planes found", "namespace", namespaceName)
		return nil, fmt.Errorf("no build planes found for namespace: %s", namespaceName)
	}

	// Get the first build plane (0th index)
	buildPlane := &buildPlanes.Items[0]
	s.logger.Debug("Found build plane", "name", buildPlane.Name, "namespace", namespaceName)

	// 3. Get ObservabilityPlane via the reference helper
	observabilityResult, err := controller.GetObservabilityPlaneOrClusterObservabilityPlaneOfBuildPlane(ctx, s.k8sClient, buildPlane)
	if err != nil {
		// Only treat NotFound as "not configured" - other errors should be returned upstream
		if apierrors.IsNotFound(err) {
			s.logger.Debug("ObservabilityPlane not found for build", "error", err, "buildPlane", buildPlane.Name)
			return &ComponentObserverResponse{
				Message: "observability-logs have not been configured",
			}, nil
		}
		s.logger.Error("Failed to get observability plane for build", "error", err, "buildPlane", buildPlane.Name)
		return nil, fmt.Errorf("failed to get observability plane: %w", err)
	}

	observerURL := observabilityResult.GetObserverURL()
	if observerURL == "" {
		s.logger.Debug("ObserverURL not configured in observability plane", "observabilityPlane", observabilityResult.GetName())
		return &ComponentObserverResponse{
			Message: "observability-logs have not been configured",
		}, nil
	}

	// 4. Return observer URL
	return &ComponentObserverResponse{
		ObserverURL: observerURL,
	}, nil
}

// GetComponentWorkloads retrieves workload data for a specific component
func (s *ComponentService) GetComponentWorkloads(ctx context.Context, namespaceName, projectName, componentName string) (interface{}, error) {
	s.logger.Debug("Getting component workloads", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewWorkload, ResourceTypeWorkload, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
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
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Use the WorkloadSpecFetcher to get workload data
	fetcher := &WorkloadSpecFetcher{}
	workloadSpec, err := fetcher.FetchSpec(ctx, s.k8sClient, namespaceName, componentName)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Workload not found for component", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, fmt.Errorf("workload not found for component: %s", componentName)
		}
		s.logger.Error("Failed to fetch workload spec", "error", err)
		return nil, fmt.Errorf("failed to fetch workload spec: %w", err)
	}

	return workloadSpec, nil
}

// CreateComponentWorkload creates or updates workload data for a specific component
func (s *ComponentService) CreateComponentWorkload(ctx context.Context, namespaceName, projectName, componentName string, workloadSpec *openchoreov1alpha1.WorkloadSpec) (*openchoreov1alpha1.WorkloadSpec, error) {
	s.logger.Debug("Creating/updating component workload", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateWorkload, ResourceTypeWorkload, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
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
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify that the component belongs to the specified project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName, "component", componentName)
		return nil, ErrComponentNotFound
	}

	// Check if workload already exists
	workloadList := &openchoreov1alpha1.WorkloadList{}
	if err := s.k8sClient.List(ctx, workloadList, client.InNamespace(namespaceName)); err != nil {
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
		s.logger.Debug("Updated existing workload", "name", existingWorkload.Name, "namespace", namespaceName)
	} else {
		// Create new workload
		workloadName = componentName + "-workload"
		workload := &openchoreov1alpha1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadName,
				Namespace: namespaceName,
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
		s.logger.Debug("Created new workload", "name", workload.Name, "namespace", namespaceName)
	}

	// Create the appropriate type-specific resource based on component type if it doesn't exist
	if err := s.createTypeSpecificResource(); err != nil {
		s.logger.Error("Failed to create type-specific resource", "componentType", component.Spec.ComponentType.Name, "error", err)
		return nil, fmt.Errorf("failed to create type-specific resource: %w", err)
	}

	return workloadSpec, nil
}

// createTypeSpecificResource creates the appropriate resource - legacy component types removed
func (s *ComponentService) createTypeSpecificResource() error {
	// Legacy component types (Service, WebApplication, ScheduledTask) have been removed
	// This method is now a no-op stub for backward compatibility
	return nil
}

// UpdateComponentWorkflowParameters updates the workflow parameters for a component
func (s *ComponentService) UpdateComponentWorkflowParameters(ctx context.Context, namespaceName, projectName, componentName string, req *models.UpdateComponentWorkflowRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Updating component workflow parameters", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Get the component
	componentKey := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}
	component := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, componentKey, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to the project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return nil, ErrComponentNotFound
	}

	// Check if component has workflow configuration
	if component.Spec.Workflow == nil {
		s.logger.Warn("Component does not have workflow configuration", "namespace", namespaceName, "project", projectName, "component", componentName)
		return nil, fmt.Errorf("component does not have workflow configuration")
	}

	if req.SystemParameters != nil {
		component.Spec.Workflow.SystemParameters = openchoreov1alpha1.SystemParametersValues{
			Repository: openchoreov1alpha1.RepositoryValues{
				URL:       req.SystemParameters.Repository.URL,
				SecretRef: req.SystemParameters.Repository.SecretRef,
				Revision: openchoreov1alpha1.RepositoryRevisionValues{
					Branch: req.SystemParameters.Repository.Revision.Branch,
					Commit: req.SystemParameters.Repository.Revision.Commit,
				},
				AppPath: req.SystemParameters.Repository.AppPath,
			},
		}
	}

	// Update developer parameters if provided
	if req.Parameters != nil {
		// Validate the parameters against the ComponentWorkflow CRD
		if err := s.validateComponentWorkflowParameters(ctx, namespaceName, component.Spec.Workflow.Name, req.Parameters); err != nil {
			s.logger.Warn("Invalid workflow parameters", "error", err, "workflow", component.Spec.Workflow.Name)
			return nil, ErrWorkflowSchemaInvalid
		}
		component.Spec.Workflow.Parameters = req.Parameters
	}

	// Update the component in Kubernetes
	if err := s.k8sClient.Update(ctx, component); err != nil {
		s.logger.Error("Failed to update component", "error", err)
		return nil, fmt.Errorf("failed to update component: %w", err)
	}

	s.logger.Debug("Updated component workflow schema successfully", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Return the updated component
	return s.GetComponent(ctx, namespaceName, projectName, componentName, []string{})
}

// UpdateComponentWorkflowSchema updates or initializes the workflow schema configuration for a component
func (s *ComponentService) UpdateComponentWorkflowSchema(ctx context.Context, namespaceName, projectName, componentName string, req *models.UpdateComponentWorkflowRequest) (*models.ComponentResponse, error) {
	s.logger.Debug("Updating component workflow schema", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			s.logger.Warn("Project not found", "namespace", namespaceName, "project", projectName)
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Get the component
	componentKey := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}
	component := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, componentKey, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to the project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return nil, ErrComponentNotFound
	}

	// Initialize workflow configuration if it doesn't exist
	if component.Spec.Workflow == nil {
		if req.WorkflowName == "" {
			s.logger.Warn("Workflow name is required to initialize workflow configuration", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, fmt.Errorf("workflow name is required to initialize workflow configuration")
		}
		component.Spec.Workflow = &openchoreov1alpha1.ComponentWorkflowRunConfig{
			Name: req.WorkflowName,
		}
	} else if req.WorkflowName != "" {
		// Update workflow name if provided
		component.Spec.Workflow.Name = req.WorkflowName
	}

	// Update system parameters if provided
	if req.SystemParameters != nil {
		component.Spec.Workflow.SystemParameters = openchoreov1alpha1.SystemParametersValues{
			Repository: openchoreov1alpha1.RepositoryValues{
				URL:       req.SystemParameters.Repository.URL,
				SecretRef: req.SystemParameters.Repository.SecretRef,
				Revision: openchoreov1alpha1.RepositoryRevisionValues{
					Branch: req.SystemParameters.Repository.Revision.Branch,
					Commit: req.SystemParameters.Repository.Revision.Commit,
				},
				AppPath: req.SystemParameters.Repository.AppPath,
			},
		}
	}

	// Update developer parameters if provided
	if req.Parameters != nil {
		// Validate the parameters against the ComponentWorkflow CRD
		if err := s.validateComponentWorkflowParameters(ctx, namespaceName, component.Spec.Workflow.Name, req.Parameters); err != nil {
			s.logger.Warn("Invalid workflow parameters", "error", err, "workflow", component.Spec.Workflow.Name)
			return nil, ErrWorkflowSchemaInvalid
		}
		component.Spec.Workflow.Parameters = req.Parameters
	}

	// Update the component in Kubernetes
	if err := s.k8sClient.Update(ctx, component); err != nil {
		s.logger.Error("Failed to update component", "error", err)
		return nil, fmt.Errorf("failed to update component: %w", err)
	}

	s.logger.Debug("Updated component workflow schema successfully", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Return the updated component
	return s.GetComponent(ctx, namespaceName, projectName, componentName, []string{})
}

// validateComponentWorkflowParameters validates the provided parameters against the ComponentWorkflow CRD's parameter schema
func (s *ComponentService) validateComponentWorkflowParameters(ctx context.Context, namespaceName, workflowName string, providedParameters *runtime.RawExtension) error {
	// Fetch the ComponentWorkflow CR
	workflowKey := client.ObjectKey{
		Name:      workflowName,
		Namespace: namespaceName,
	}
	componentWorkflow := &openchoreov1alpha1.ComponentWorkflow{}
	if err := s.k8sClient.Get(ctx, workflowKey, componentWorkflow); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ComponentWorkflow not found", "namespace", namespaceName, "workflow", workflowName)
			return fmt.Errorf("component workflow %s not found", workflowName)
		}
		s.logger.Error("Failed to get component workflow", "error", err)
		return fmt.Errorf("failed to get component workflow: %w", err)
	}

	// If component workflow has no parameter schema defined, any parameters are valid
	if componentWorkflow.Spec.Schema.Parameters == nil {
		return nil
	}

	// If provided parameters are nil or empty, it's valid (defaults will be applied)
	if providedParameters == nil || len(providedParameters.Raw) == 0 {
		return nil
	}

	// Unmarshal the component workflow's parameter schema definition
	var parameterSchemaMap map[string]any
	if err := json.Unmarshal(componentWorkflow.Spec.Schema.Parameters.Raw, &parameterSchemaMap); err != nil {
		s.logger.Error("Failed to unmarshal component workflow parameter schema", "error", err)
		return fmt.Errorf("failed to parse component workflow parameter schema: %w", err)
	}

	// Unmarshal the provided parameter values
	var providedValues map[string]any
	if err := json.Unmarshal(providedParameters.Raw, &providedValues); err != nil {
		s.logger.Error("Failed to unmarshal provided parameters", "error", err)
		return fmt.Errorf("failed to parse provided parameters: %w", err)
	}

	// Build structural schema from component workflow parameter schema
	def := openchoreoschema.Definition{
		Schemas: []map[string]any{parameterSchemaMap},
	}

	structural, err := openchoreoschema.ToStructural(def)
	if err != nil {
		s.logger.Error("Failed to build structural schema", "error", err)
		return fmt.Errorf("failed to build component workflow parameter schema structure: %w", err)
	}

	// Validate the provided values against the structural schema
	if err := openchoreoschema.ValidateAgainstSchema(providedValues, structural); err != nil {
		return err
	}

	return nil
}

// ListComponentTraits returns all trait instances attached to a component
func (s *ComponentService) ListComponentTraits(ctx context.Context, namespaceName, projectName, componentName string) ([]*models.ComponentTraitResponse, error) {
	s.logger.Debug("Listing component traits", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewComponent, ResourceTypeComponent, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Get component
	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return nil, ErrComponentNotFound
	}

	// Convert component traits to response format
	traits := make([]*models.ComponentTraitResponse, 0, len(component.Spec.Traits))
	for _, trait := range component.Spec.Traits {
		traitResponse := &models.ComponentTraitResponse{
			Kind:         string(trait.Kind),
			Name:         trait.Name,
			InstanceName: trait.InstanceName,
		}

		// Convert parameters from runtime.RawExtension to map
		if trait.Parameters != nil && trait.Parameters.Raw != nil {
			var params map[string]interface{}
			if err := json.Unmarshal(trait.Parameters.Raw, &params); err != nil {
				s.logger.Warn("Failed to unmarshal trait parameters", "trait", trait.Name, "instanceName", trait.InstanceName, "error", err)
				// Continue without parameters rather than failing
			} else {
				traitResponse.Parameters = params
			}
		}

		traits = append(traits, traitResponse)
	}

	s.logger.Debug("Listed component traits", "namespace", namespaceName, "project", projectName, "component", componentName, "count", len(traits))
	return traits, nil
}

// UpdateComponentTraits replaces all traits on a component
func (s *ComponentService) UpdateComponentTraits(ctx context.Context, namespaceName, projectName, componentName string, req *models.UpdateComponentTraitsRequest) ([]*models.ComponentTraitResponse, error) {
	s.logger.Debug("Updating component traits", "namespace", namespaceName, "project", projectName, "component", componentName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionUpdateComponent, ResourceTypeComponent, componentName,
		authz.ResourceHierarchy{Namespace: namespaceName, Project: projectName, Component: componentName}); err != nil {
		return nil, err
	}

	// Verify project exists
	_, err := s.projectService.getProject(ctx, namespaceName, projectName)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to verify project: %w", err)
	}

	// Get component
	componentKey := client.ObjectKey{
		Namespace: namespaceName,
		Name:      componentName,
	}
	var component openchoreov1alpha1.Component
	if err := s.k8sClient.Get(ctx, componentKey, &component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "project", projectName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Verify component belongs to project
	if component.Spec.Owner.ProjectName != projectName {
		s.logger.Warn("Component belongs to different project", "namespace", namespaceName, "expected_project", projectName, "actual_project", component.Spec.Owner.ProjectName)
		return nil, ErrComponentNotFound
	}

	// Validate that all referenced traits exist
	for _, traitReq := range req.Traits {
		kind := openchoreov1alpha1.TraitRefKind(traitReq.Kind)
		traitSpec, err := s.fetchTraitSpec(ctx, kind, traitReq.Name, namespaceName)
		if err != nil {
			s.logger.Error("Failed to get trait", "kind", traitReq.Kind, "error", err)
			return nil, fmt.Errorf("failed to get trait %s: %w", traitReq.Name, err)
		}
		if traitSpec == nil {
			s.logger.Warn("Trait not found", "kind", traitReq.Kind, "namespace", namespaceName, "trait", traitReq.Name)
			return nil, fmt.Errorf("%w: %s", ErrTraitNotFound, traitReq.Name)
		}
	}

	// Convert request traits to component traits
	componentTraits := make([]openchoreov1alpha1.ComponentTrait, 0, len(req.Traits))
	for _, traitReq := range req.Traits {
		componentTrait := openchoreov1alpha1.ComponentTrait{
			Kind:         openchoreov1alpha1.TraitRefKind(traitReq.Kind),
			Name:         traitReq.Name,
			InstanceName: traitReq.InstanceName,
		}

		// Convert parameters map to runtime.RawExtension
		if len(traitReq.Parameters) > 0 {
			paramsBytes, err := json.Marshal(traitReq.Parameters)
			if err != nil {
				s.logger.Error("Failed to marshal trait parameters", "trait", traitReq.Name, "error", err)
				return nil, fmt.Errorf("failed to marshal trait parameters for %s: %w", traitReq.Name, err)
			}
			componentTrait.Parameters = &runtime.RawExtension{Raw: paramsBytes}
		}

		componentTraits = append(componentTraits, componentTrait)
	}

	// Create a patch base
	patchBase := component.DeepCopy()

	// Update the traits
	component.Spec.Traits = componentTraits

	// Only patch if there are actual changes
	if !reflect.DeepEqual(patchBase.Spec.Traits, component.Spec.Traits) {
		patch := client.MergeFrom(patchBase)
		if err := s.k8sClient.Patch(ctx, &component, patch); err != nil {
			s.logger.Error("Failed to patch component traits", "error", err)
			return nil, fmt.Errorf("failed to patch component traits: %w", err)
		}
		s.logger.Debug("Component traits updated successfully", "namespace", namespaceName, "project", projectName, "component", componentName)
	} else {
		s.logger.Debug("No trait changes detected", "namespace", namespaceName, "project", projectName, "component", componentName)
	}

	// Return the updated traits
	return s.ListComponentTraits(ctx, namespaceName, projectName, componentName)
}
