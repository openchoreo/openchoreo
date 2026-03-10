// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	openchoreoschema "github.com/openchoreo/openchoreo/internal/schema"
)

var componentTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "Component",
}

var componentReleaseTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ComponentRelease",
}

// componentService handles component-related business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type componentService struct {
	k8sClient      client.Client
	projectService projectsvc.Service
	logger         *slog.Logger
}

var _ Service = (*componentService)(nil)

// NewService creates a new component service without authorization.
// It internally creates an unwrapped project service for project validation,
// avoiding double authz when used within the authz-wrapped component service.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &componentService{
		k8sClient:      k8sClient,
		projectService: projectsvc.NewService(k8sClient, logger.With("component", "project-service-internal")),
		logger:         logger,
	}
}

func (s *componentService) CreateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if component == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}

	s.logger.Debug("Creating component", "namespace", namespaceName, "component", component.Name)

	// Validate that the referenced project exists
	if _, err := s.projectService.GetProject(ctx, namespaceName, component.Spec.Owner.ProjectName); err != nil {
		return nil, err
	}

	exists, err := s.componentExists(ctx, namespaceName, component.Name)
	if err != nil {
		s.logger.Error("Failed to check component existence", "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}
	if exists {
		s.logger.Warn("Component already exists", "namespace", namespaceName, "component", component.Name)
		return nil, ErrComponentAlreadyExists
	}

	// Set defaults
	component.Status = openchoreov1alpha1.ComponentStatus{}
	component.Namespace = namespaceName
	if component.Labels == nil {
		component.Labels = make(map[string]string)
	}
	component.Labels[labels.LabelKeyProjectName] = component.Spec.Owner.ProjectName

	if err := s.k8sClient.Create(ctx, component); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Component already exists", "namespace", namespaceName, "component", component.Name)
			return nil, ErrComponentAlreadyExists
		}
		if apierrors.IsInvalid(err) {
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to create component CR", "error", err)
		return nil, fmt.Errorf("failed to create component: %w", err)
	}

	s.logger.Debug("Component created successfully", "namespace", namespaceName, "component", component.Name)
	component.TypeMeta = componentTypeMeta
	return component, nil
}

func (s *componentService) UpdateComponent(ctx context.Context, namespaceName string, component *openchoreov1alpha1.Component) (*openchoreov1alpha1.Component, error) {
	if component == nil {
		return nil, fmt.Errorf("component cannot be nil")
	}

	s.logger.Debug("Updating component", "namespace", namespaceName, "component", component.Name)

	existing := &openchoreov1alpha1.Component{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: component.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", component.Name)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	// Clear status from user input — status is server-managed
	component.Status = openchoreov1alpha1.ComponentStatus{}

	// Prevent project reassignment: if the incoming component specifies a project,
	// it must match the existing component's project
	if component.Spec.Owner.ProjectName != existing.Spec.Owner.ProjectName {
		return nil, &services.ValidationError{Msg: "spec.owner.projectName is immutable"}
	}

	// Only apply user-mutable fields to the existing object, preserving server-managed fields
	existing.Spec = component.Spec
	existing.Labels = component.Labels
	existing.Annotations = component.Annotations

	// Preserve special labels
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	existing.Labels[labels.LabelKeyProjectName] = existing.Spec.Owner.ProjectName

	if err := s.k8sClient.Update(ctx, existing); err != nil {
		if apierrors.IsInvalid(err) {
			s.logger.Error("Component update rejected by validation", "error", err)
			return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
		}
		s.logger.Error("Failed to update component CR", "error", err)
		return nil, fmt.Errorf("failed to update component: %w", err)
	}

	s.logger.Debug("Component updated successfully", "namespace", namespaceName, "component", component.Name)
	existing.TypeMeta = componentTypeMeta
	return existing, nil
}

func (s *componentService) ListComponents(ctx context.Context, namespaceName, projectName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
	s.logger.Debug("Listing components", "namespace", namespaceName, "project", projectName, "limit", opts.Limit, "cursor", opts.Cursor)

	// Validate that the referenced project exists when filtering by project
	if projectName != "" {
		if _, err := s.projectService.GetProject(ctx, namespaceName, projectName); err != nil {
			return nil, err
		}
	}

	listResource := s.listComponentsResource(namespaceName)

	// Apply project filter if specified. PreFilteredList handles over-fetching
	// and cursor tracking so pagination remains correct.
	var filters []services.ItemFilter[openchoreov1alpha1.Component]
	if projectName != "" {
		filters = append(filters, func(c openchoreov1alpha1.Component) bool {
			return c.Spec.Owner.ProjectName == projectName
		})
	}

	return services.PreFilteredList(listResource, filters...)(ctx, opts)
}

// listComponentsResource returns a ListResource that fetches components from K8s for the given namespace.
func (s *componentService) listComponentsResource(namespaceName string) services.ListResource[openchoreov1alpha1.Component] {
	return func(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.Component], error) {
		commonOpts, err := services.BuildListOptions(opts)
		if err != nil {
			return nil, err
		}
		listOpts := append([]client.ListOption{client.InNamespace(namespaceName)}, commonOpts...)

		var componentList openchoreov1alpha1.ComponentList
		if err := s.k8sClient.List(ctx, &componentList, listOpts...); err != nil {
			s.logger.Error("Failed to list components", "error", err)
			return nil, fmt.Errorf("failed to list components: %w", err)
		}

		for i := range componentList.Items {
			componentList.Items[i].TypeMeta = componentTypeMeta
		}

		result := &services.ListResult[openchoreov1alpha1.Component]{
			Items:      componentList.Items,
			NextCursor: componentList.Continue,
		}
		if componentList.RemainingItemCount != nil {
			remaining := *componentList.RemainingItemCount
			result.RemainingCount = &remaining
		}

		return result, nil
	}
}

func (s *componentService) GetComponent(ctx context.Context, namespaceName, componentName string) (*openchoreov1alpha1.Component, error) {
	s.logger.Debug("Getting component", "namespace", namespaceName, "component", componentName)

	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, component); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Component not found", "namespace", namespaceName, "component", componentName)
			return nil, ErrComponentNotFound
		}
		s.logger.Error("Failed to get component", "error", err)
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	component.TypeMeta = componentTypeMeta
	return component, nil
}

func (s *componentService) DeleteComponent(ctx context.Context, namespaceName, componentName string) error {
	s.logger.Debug("Deleting component", "namespace", namespaceName, "component", componentName)

	component := &openchoreov1alpha1.Component{}
	component.Name = componentName
	component.Namespace = namespaceName

	if err := s.k8sClient.Delete(ctx, component); err != nil {
		if apierrors.IsNotFound(err) {
			return ErrComponentNotFound
		}
		s.logger.Error("Failed to delete component CR", "error", err)
		return fmt.Errorf("failed to delete component: %w", err)
	}

	s.logger.Debug("Component deleted successfully", "namespace", namespaceName, "component", componentName)
	return nil
}

func (s *componentService) GenerateRelease(ctx context.Context, namespaceName, componentName string, req *GenerateReleaseRequest) (*openchoreov1alpha1.ComponentRelease, error) {
	releaseName := strings.TrimSpace(req.ReleaseName)

	s.logger.Debug("Generating component release", "namespace", namespaceName, "component", componentName, "release", releaseName)

	// Get the component to derive the project and component type
	component, err := s.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	projectName := component.Spec.Owner.ProjectName

	// Find the workload for this component
	workload, err := s.findWorkload(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, err
	}

	// Generate release name if not provided
	if releaseName == "" {
		generated, err := s.generateReleaseName(ctx, namespaceName, projectName, componentName)
		if err != nil {
			return nil, err
		}
		releaseName = generated
	}

	// Fetch ComponentType spec
	componentTypeSpec, err := s.fetchComponentTypeSpec(ctx, &component.Spec.ComponentType, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch component type: %w", err)
	}

	// Fetch and validate Trait specs
	traits, err := s.fetchTraitSpecs(ctx, component.Spec.Traits, namespaceName)
	if err != nil {
		return nil, err
	}

	// Build ComponentProfile from Component parameters
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

	// Build workload template spec
	workloadTemplateSpec := openchoreov1alpha1.WorkloadTemplateSpec{
		Container: workload.Spec.Container,
		Endpoints: workload.Spec.Endpoints,
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

	s.logger.Debug("ComponentRelease created successfully", "namespace", namespaceName, "component", componentName, "release", releaseName)
	componentRelease.TypeMeta = componentReleaseTypeMeta
	return componentRelease, nil
}

// findWorkload finds the workload for a given component within a namespace.
func (s *componentService) findWorkload(ctx context.Context, namespaceName, projectName, componentName string) (*openchoreov1alpha1.Workload, error) {
	workloadList := &openchoreov1alpha1.WorkloadList{}
	if err := s.k8sClient.List(ctx, workloadList, client.InNamespace(namespaceName)); err != nil {
		s.logger.Error("Failed to list workloads", "error", err)
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}

	for i := range workloadList.Items {
		w := &workloadList.Items[i]
		if w.Spec.Owner.ComponentName == componentName && w.Spec.Owner.ProjectName == projectName {
			return w, nil
		}
	}

	return nil, ErrWorkloadNotFound
}

// generateReleaseName generates a unique release name for a component.
// Format: <component_name>-<date>-<number>, e.g., my-component-20240118-1
func (s *componentService) generateReleaseName(ctx context.Context, namespaceName, projectName, componentName string) (string, error) {
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

	now := metav1.Now()
	dateStr := now.Format("20060102")
	todayPrefix := fmt.Sprintf("%s-%s-", componentName, dateStr)
	todayCount := 0
	for _, release := range releaseList.Items {
		if len(release.Name) >= len(todayPrefix) && release.Name[:len(todayPrefix)] == todayPrefix {
			todayCount++
		}
	}

	return fmt.Sprintf("%s-%s-%d", componentName, dateStr, todayCount+1), nil
}

// fetchComponentTypeSpec fetches the ComponentTypeSpec from the cluster based on the ComponentTypeRef.
func (s *componentService) fetchComponentTypeSpec(ctx context.Context, ctRef *openchoreov1alpha1.ComponentTypeRef, namespaceName string) (*openchoreov1alpha1.ComponentTypeSpec, error) {
	componentTypeName, err := parseComponentTypeName(ctRef.Name)
	if err != nil {
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
			return nil, fmt.Errorf("failed to get ClusterComponentType: %w", err)
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
				Kind:               openchoreov1alpha1.TraitRefKind(t.Kind),
				Name:               t.Name,
				InstanceName:       t.InstanceName,
				Parameters:         t.Parameters,
				EnvironmentConfigs: t.EnvironmentConfigs,
			}
		}
		allowedWorkflows := make([]openchoreov1alpha1.WorkflowRef, len(cct.Spec.AllowedWorkflows))
		for i, ref := range cct.Spec.AllowedWorkflows {
			allowedWorkflows[i] = openchoreov1alpha1.WorkflowRef{
				Kind: openchoreov1alpha1.WorkflowRefKind(ref.Kind),
				Name: ref.Name,
			}
		}
		spec := openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType:       cct.Spec.WorkloadType,
			AllowedWorkflows:   allowedWorkflows,
			Parameters:         cct.Spec.Parameters,
			EnvironmentConfigs: cct.Spec.EnvironmentConfigs,
			Traits:             traits,
			AllowedTraits:      allowedTraits,
			Validations:        cct.Spec.Validations,
			Resources:          cct.Spec.Resources,
		}
		return &spec, nil
	default:
		ct := &openchoreov1alpha1.ComponentType{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: componentTypeName, Namespace: namespaceName}, ct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				s.logger.Warn("ComponentType not found", "componentType", ctRef.Name)
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get ComponentType: %w", err)
		}
		return &ct.Spec, nil
	}
}

// fetchTraitSpecs fetches trait specs for all component traits, detecting cross-kind collisions.
func (s *componentService) fetchTraitSpecs(ctx context.Context, componentTraits []openchoreov1alpha1.ComponentTrait, namespaceName string) (map[string]openchoreov1alpha1.TraitSpec, error) {
	traits := make(map[string]openchoreov1alpha1.TraitSpec)
	traitKindByName := make(map[string]openchoreov1alpha1.TraitRefKind)

	for _, ct := range componentTraits {
		kind := ct.Kind
		if kind == "" {
			kind = openchoreov1alpha1.TraitRefKindTrait
		}
		if prevKind, exists := traitKindByName[ct.Name]; exists && prevKind != kind {
			return nil, fmt.Errorf("trait %q is referenced as both %s and %s: %w", ct.Name, prevKind, kind, ErrTraitNameCollision)
		}

		traitSpec, err := s.fetchTraitSpec(ctx, kind, ct.Name, namespaceName)
		if err != nil {
			s.logger.Error("Failed to get Trait", "kind", kind, "trait", ct.Name, "error", err)
			continue
		}
		if traitSpec == nil {
			s.logger.Warn("Trait not found", "kind", kind, "trait", ct.Name)
			continue
		}
		traitKindByName[ct.Name] = kind
		traits[ct.Name] = *traitSpec
	}

	return traits, nil
}

// fetchTraitSpec fetches a TraitSpec from the cluster based on the trait kind and name.
func (s *componentService) fetchTraitSpec(ctx context.Context, kind openchoreov1alpha1.TraitRefKind, name, namespaceName string) (*openchoreov1alpha1.TraitSpec, error) {
	switch kind {
	case openchoreov1alpha1.TraitRefKindClusterTrait:
		ct := &openchoreov1alpha1.ClusterTrait{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, ct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get ClusterTrait: %w", err)
		}
		return &openchoreov1alpha1.TraitSpec{
			Parameters:         ct.Spec.Parameters,
			EnvironmentConfigs: ct.Spec.EnvironmentConfigs,
			Creates:            ct.Spec.Creates,
			Patches:            ct.Spec.Patches,
		}, nil
	default:
		trait := &openchoreov1alpha1.Trait{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespaceName}, trait); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get Trait: %w", err)
		}
		return &trait.Spec, nil
	}
}

// parseComponentTypeName extracts the ComponentType name from the ComponentType string.
// Format: {workloadType}/{componentTypeName}, e.g., "deployment/web-app" → "web-app"
func parseComponentTypeName(componentType string) (string, error) {
	parts := strings.Split(componentType, "/")
	if len(parts) != 2 || parts[1] == "" {
		return "", fmt.Errorf("invalid component type format: %s", componentType)
	}
	return parts[1], nil
}

func (s *componentService) GetComponentSchema(ctx context.Context, namespaceName, componentName string) (*extv1.JSONSchemaProps, error) {
	componentName = strings.TrimSpace(componentName)
	if componentName == "" {
		return nil, fmt.Errorf("componentName is required: %w", ErrValidation)
	}

	s.logger.Debug("Getting component schema", "namespace", namespaceName, "component", componentName)

	// Get the component
	component, err := s.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}

	// Parse ComponentType name from format: {workloadType}/{componentTypeName}
	ctName, err := parseComponentTypeName(component.Spec.ComponentType.Name)
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
				return nil, ErrComponentTypeNotFound
			}
			return nil, fmt.Errorf("failed to get ClusterComponentType: %w", err)
		}
		allowedWfs := make([]openchoreov1alpha1.WorkflowRef, len(cct.Spec.AllowedWorkflows))
		for i, ref := range cct.Spec.AllowedWorkflows {
			allowedWfs[i] = openchoreov1alpha1.WorkflowRef{
				Kind: openchoreov1alpha1.WorkflowRefKind(ref.Kind),
				Name: ref.Name,
			}
		}
		ct = openchoreov1alpha1.ComponentType{
			ObjectMeta: cct.ObjectMeta,
			Spec: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType:       cct.Spec.WorkloadType,
				AllowedWorkflows:   allowedWfs,
				Parameters:         cct.Spec.Parameters,
				EnvironmentConfigs: cct.Spec.EnvironmentConfigs,
				Resources:          cct.Spec.Resources,
			},
		}
	default:
		ctKey := client.ObjectKey{
			Namespace: namespaceName,
			Name:      ctName,
		}
		if err := s.k8sClient.Get(ctx, ctKey, &ct); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil, ErrComponentTypeNotFound
			}
			return nil, fmt.Errorf("failed to get ComponentType: %w", err)
		}
	}

	// Build the wrapped schema properties
	wrappedSchema := &extv1.JSONSchemaProps{
		Type:       "object",
		Properties: make(map[string]extv1.JSONSchemaProps),
	}

	// Only add componentTypeEnvironmentConfigs if there are actual environmentConfigs
	if envRaw := ct.Spec.EnvironmentConfigs.GetRaw(); envRaw != nil && envRaw.Raw != nil {
		jsonSchema, err := openchoreoschema.SectionToJSONSchema(ct.Spec.EnvironmentConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
		}
		wrappedSchema.Properties["componentTypeEnvironmentConfigs"] = *jsonSchema
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

		traitJSONSchema, err := buildTraitEnvironmentConfigsSchema(*traitSpec, componentTrait.Name)
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

	hasComponentTypeEnvironmentConfigs := false
	for key := range wrappedSchema.Properties {
		if key != "traitOverrides" {
			hasComponentTypeEnvironmentConfigs = true
			break
		}
	}
	s.logger.Debug("Retrieved component schema successfully", "namespace", namespaceName, "component", componentName, "hasComponentTypeEnvironmentConfigs", hasComponentTypeEnvironmentConfigs, "traitCount", len(traitSchemas))
	return wrappedSchema, nil
}

func (s *componentService) GetComponentReleaseSchema(ctx context.Context, namespaceName, releaseName, componentName string) (*extv1.JSONSchemaProps, error) {
	releaseName = strings.TrimSpace(releaseName)
	if releaseName == "" {
		return nil, fmt.Errorf("releaseName is required: %w", ErrValidation)
	}
	componentName = strings.TrimSpace(componentName)
	if componentName == "" {
		return nil, fmt.Errorf("componentName is required: %w", ErrValidation)
	}

	s.logger.Debug("Getting component release schema", "namespace", namespaceName, "component", componentName, "release", releaseName)

	// Get the ComponentRelease
	var release openchoreov1alpha1.ComponentRelease
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: releaseName}, &release); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrComponentReleaseNotFound
		}
		return nil, fmt.Errorf("failed to get component release: %w", err)
	}

	if release.Spec.Owner.ComponentName != componentName {
		return nil, ErrComponentReleaseNotFound
	}

	wrappedSchema := &extv1.JSONSchemaProps{
		Type:       "object",
		Properties: make(map[string]extv1.JSONSchemaProps),
	}

	if envRaw := release.Spec.ComponentType.EnvironmentConfigs.GetRaw(); envRaw != nil && envRaw.Raw != nil {
		jsonSchema, err := openchoreoschema.SectionToJSONSchema(release.Spec.ComponentType.EnvironmentConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to convert to JSON schema: %w", err)
		}
		wrappedSchema.Properties["componentTypeEnvironmentConfigs"] = *jsonSchema
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

			traitJSONSchema, err := buildTraitEnvironmentConfigsSchema(traitSpec, componentTrait.Name)
			if err != nil {
				return nil, err
			}

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

	s.logger.Debug("Retrieved component release schema successfully", "namespace", namespaceName, "component", componentName, "release", releaseName, "traitCount", len(traitSchemas))
	return wrappedSchema, nil
}

// buildTraitEnvironmentConfigsSchema extracts and converts a TraitSpec's environmentConfigs to JSON schema.
// Returns nil if the trait has no environmentConfigs.
func buildTraitEnvironmentConfigsSchema(traitSpec openchoreov1alpha1.TraitSpec, traitName string) (*extv1.JSONSchemaProps, error) {
	if envRaw := traitSpec.EnvironmentConfigs.GetRaw(); envRaw == nil || envRaw.Raw == nil {
		return nil, nil
	}

	traitJSONSchema, err := openchoreoschema.SectionToJSONSchema(traitSpec.EnvironmentConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to convert trait %s to JSON schema: %w", traitName, err)
	}

	return traitJSONSchema, nil
}

func (s *componentService) componentExists(ctx context.Context, namespaceName, componentName string) (bool, error) {
	component := &openchoreov1alpha1.Component{}
	key := client.ObjectKey{
		Name:      componentName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, component)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of component %s/%s: %w", namespaceName, componentName, err)
	}
	return true, nil
}
