// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package component provides the main rendering pipeline for Component resources.
//
// The pipeline combines Component, ComponentType, Traits, Workload and ReleaseBinding
// to generate fully resolved Kubernetes resource manifests by:
//   - Building CEL evaluation contexts with parameters, overrides, and defaults
//   - Rendering base resources from ComponentType
//   - Processing traits (creates and patches)
//   - Post-processing (validation, labels, annotations)
package component

import (
	"fmt"
	"sort"

	"github.com/go-playground/validator/v10"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/trait"
	"github.com/openchoreo/openchoreo/internal/template"
	"github.com/openchoreo/openchoreo/pkg/hash"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// Workload type constants for internal use
const (
	workloadTypeDeployment  = "deployment"
	workloadTypeStatefulSet = "statefulset"
)

// Kubernetes resource kind constants
const (
	kindDeployment  = "Deployment"
	kindStatefulSet = "StatefulSet"
)

// Option is a function that configures a Pipeline.
type Option func(*Pipeline)

// NewPipeline creates a new component rendering pipeline.
func NewPipeline(opts ...Option) *Pipeline {
	p := &Pipeline{}
	for _, opt := range opts {
		opt(p)
	}
	if p.templateEngine == nil {
		p.templateEngine = template.NewEngineWithOptions(
			template.WithCELExtensions(context.CELExtensions()...),
		)
	}
	return p
}

// Render orchestrates the complete rendering workflow for a Component.
//
// Workflow:
//   - Validate input
//   - Build component context (parameters + overrides + defaults)
//   - Render base resources from ComponentType
//   - Process traits (creates and patches)
//   - Post-process (validate, add labels/annotations, sort)
//   - Return output
//
// Returns an error if any step fails.
func (p *Pipeline) Render(input *RenderInput) (*RenderOutput, error) {
	// Validate input
	if err := p.validateInput(input); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	metadata := &RenderMetadata{
		Warnings: []string{},
	}

	// Apply workload overrides from ReleaseBinding if present
	workload := input.Workload
	if input.Workload != nil && input.ReleaseBinding != nil && input.ReleaseBinding.Spec.WorkloadOverrides != nil {
		workload = context.MergeWorkloadOverrides(input.Workload, input.ReleaseBinding.Spec.WorkloadOverrides)
	}

	// Pre-compute workload data and configurations once and share across all contexts
	workloadData := context.ExtractWorkloadData(workload)
	configurations := context.ExtractConfigurationsFromWorkload(input.SecretReferences, workload)

	// Build component context
	componentContext, err := context.BuildComponentContext(&context.ComponentContextInput{
		Component:                  input.Component,
		ComponentType:              input.ComponentType,
		ReleaseBinding:             input.ReleaseBinding,
		DataPlane:                  input.DataPlane,
		Environment:                input.Environment,
		WorkloadData:               workloadData,
		Configurations:             configurations,
		Metadata:                   input.Metadata,
		DefaultNotificationChannel: input.DefaultNotificationChannel,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build component context: %w", err)
	}

	// Evaluate ComponentType validation rules
	if err := renderer.EvaluateValidationRules(
		p.templateEngine,
		input.ComponentType.Spec.Validations,
		componentContext.ToMap(),
	); err != nil {
		return nil, fmt.Errorf("component type validation failed: %w", err)
	}

	input.ApplyTargetPlaneDefaults()

	// Render base resources from ComponentType
	resourceRenderer := renderer.NewRenderer(p.templateEngine)
	renderedResources, err := resourceRenderer.RenderResources(
		input.ComponentType.Spec.Resources,
		componentContext.ToMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to render base resources: %w", err)
	}
	metadata.BaseResourceCount = len(renderedResources)

	// Process traits
	traitProcessor := trait.NewProcessor(p.templateEngine)

	// Build trait map
	traitMap := make(map[string]*v1alpha1.Trait)
	for i := range input.Traits {
		t := &input.Traits[i]
		traitMap[t.Name] = t
	}

	// Create schema cache for trait reuse within this render
	schemaCache := make(map[string]*context.SchemaBundle)

	// Process embedded traits from ComponentType (before component-level traits)
	for _, embeddedTrait := range input.ComponentType.Spec.Traits {
		t, ok := traitMap[embeddedTrait.Name]
		if !ok {
			return nil, fmt.Errorf("embedded trait %s referenced but not found in traits list", embeddedTrait.Name)
		}

		// Resolve CEL bindings against component context
		resolvedParams, resolvedEnvOverrides, err := context.ResolveEmbeddedTraitBindings(
			p.templateEngine,
			embeddedTrait,
			componentContext.ToMap(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve embedded trait bindings for %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		// Build embedded trait context
		traitContext, err := context.BuildEmbeddedTraitContext(&context.EmbeddedTraitContextInput{
			Trait:                      t,
			InstanceName:               embeddedTrait.InstanceName,
			ResolvedParameters:         resolvedParams,
			ResolvedEnvOverrides:       resolvedEnvOverrides,
			Component:                  input.Component,
			WorkloadData:               workloadData,
			Configurations:             configurations,
			Metadata:                   input.Metadata,
			SchemaCache:                schemaCache,
			DataPlane:                  input.DataPlane,
			Environment:                input.Environment,
			DefaultNotificationChannel: input.DefaultNotificationChannel,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build embedded trait context for %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		// Evaluate trait validation rules
		if err := renderer.EvaluateValidationRules(
			p.templateEngine,
			t.Spec.Validations,
			traitContext.ToMap(),
		); err != nil {
			return nil, fmt.Errorf("trait %s/%s validation failed: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		// Process trait (creates + patches)
		beforeCount := len(renderedResources)
		renderedResources, err = traitProcessor.ProcessTraits(renderedResources, t, traitContext.ToMap())
		if err != nil {
			return nil, fmt.Errorf("failed to process embedded trait %s/%s: %w",
				embeddedTrait.Name, embeddedTrait.InstanceName, err)
		}

		metadata.TraitCount++
		metadata.TraitResourceCount += len(renderedResources) - beforeCount
	}

	// Process each component-level trait instance
	for _, traitInstance := range input.Component.Spec.Traits {
		t, ok := traitMap[traitInstance.Name]
		if !ok {
			return nil, fmt.Errorf("trait %s referenced but not found in traits list", traitInstance.Name)
		}

		// Build trait context (BuildTraitContext will handle schema caching)
		traitContext, err := context.BuildTraitContext(&context.TraitContextInput{
			Trait:                      t,
			Instance:                   traitInstance,
			Component:                  input.Component,
			ReleaseBinding:             input.ReleaseBinding,
			WorkloadData:               workloadData,
			Configurations:             configurations,
			Metadata:                   input.Metadata,
			SchemaCache:                schemaCache,
			DataPlane:                  input.DataPlane,
			Environment:                input.Environment,
			DefaultNotificationChannel: input.DefaultNotificationChannel,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build trait context for %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		// Evaluate trait validation rules
		if err := renderer.EvaluateValidationRules(
			p.templateEngine,
			t.Spec.Validations,
			traitContext.ToMap(),
		); err != nil {
			return nil, fmt.Errorf("trait %s/%s validation failed: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		// Process trait (creates + patches)
		beforeCount := len(renderedResources)
		renderedResources, err = traitProcessor.ProcessTraits(renderedResources, t, traitContext.ToMap())
		if err != nil {
			return nil, fmt.Errorf("failed to process trait %s/%s: %w",
				traitInstance.Name, traitInstance.InstanceName, err)
		}

		metadata.TraitCount++
		metadata.TraitResourceCount += len(renderedResources) - beforeCount
	}

	if err := p.postProcessResources(renderedResources, input); err != nil {
		return nil, fmt.Errorf("failed to post-process resources: %w", err)
	}

	if err := p.validateResources(renderedResources); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	sortRenderedResources(renderedResources)

	metadata.ResourceCount = len(renderedResources)

	return &RenderOutput{
		Resources: renderedResources,
		Metadata:  metadata,
	}, nil
}

// validateInput ensures the input has all required fields.
func (p *Pipeline) validateInput(input *RenderInput) error {
	if err := validate.Struct(input); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Additional validation that can't be expressed with struct tags
	if input.ComponentType.Spec.Resources == nil {
		return fmt.Errorf("component type has no resources")
	}

	return nil
}

// postProcessResources adds labels, annotations, and performs cleanup.
func (p *Pipeline) postProcessResources(resources []renderer.RenderedResource, input *RenderInput) error {
	for _, rr := range resources {
		if err := addLabels(rr.Resource, input.Metadata.Labels); err != nil {
			return fmt.Errorf("failed to add labels: %w", err)
		}
	}

	if err := p.addDPResourceHashAnnotation(resources, input); err != nil {
		return fmt.Errorf("failed to add dp-resource-hash annotation: %w", err)
	}

	return nil
}

// addLabels adds labels to a resource's metadata.
func addLabels(resource map[string]any, labelsToAdd map[string]string) error {
	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("resource missing metadata")
	}
	existingLabels, _ := metadata["labels"].(map[string]any)
	if existingLabels == nil {
		existingLabels = make(map[string]any)
	}
	for k, v := range labelsToAdd {
		existingLabels[k] = v
	}
	metadata["labels"] = existingLabels
	return nil
}

// validateResources performs basic validation on rendered resources.
func (p *Pipeline) validateResources(resources []renderer.RenderedResource) error {
	for i, rr := range resources {
		resource := rr.Resource
		kind, _ := resource["kind"].(string)
		apiVersion, _ := resource["apiVersion"].(string)
		var resourceID string
		if kind != "" {
			resourceID = fmt.Sprintf("resource #%d (%s)", i, kind)
		} else {
			resourceID = fmt.Sprintf("resource #%d", i)
		}

		if apiVersion == "" {
			return fmt.Errorf("%s missing apiVersion", resourceID)
		}
		if kind == "" {
			return fmt.Errorf("%s missing kind", resourceID)
		}

		metadata, ok := resource["metadata"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s missing metadata", resourceID)
		}

		name, _ := metadata["name"].(string)
		if name == "" {
			return fmt.Errorf("%s missing metadata.name", resourceID)
		}
	}
	return nil
}

func sortRenderedResources(renderedResources []renderer.RenderedResource) {
	sort.SliceStable(renderedResources, func(i, j int) bool {
		return compareResources(renderedResources[i].Resource, renderedResources[j].Resource)
	})
}

func compareResources(a, b map[string]any) bool {
	kind1, _ := a["kind"].(string)
	kind2, _ := b["kind"].(string)
	if kind1 != kind2 {
		return kind1 < kind2
	}

	apiVersion1, _ := a["apiVersion"].(string)
	apiVersion2, _ := b["apiVersion"].(string)
	if apiVersion1 != apiVersion2 {
		return apiVersion1 < apiVersion2
	}

	meta1, _ := a["metadata"].(map[string]any)
	meta2, _ := b["metadata"].(map[string]any)

	var ns1, ns2, name1, name2 string
	if meta1 != nil {
		ns1, _ = meta1["namespace"].(string)
		name1, _ = meta1["name"].(string)
	}
	if meta2 != nil {
		ns2, _ = meta2["namespace"].(string)
		name2, _ = meta2["name"].(string)
	}

	if ns1 != ns2 {
		return ns1 < ns2
	}
	return name1 < name2
}

// addDPResourceHashAnnotation adds an annotation to the pod template of Deployment/StatefulSet
// workloads containing a hash of all non-workload dataplane resources. This triggers pod
// rollout when ConfigMaps, Secrets, or other dependent resources change.
func (p *Pipeline) addDPResourceHashAnnotation(resources []renderer.RenderedResource, input *RenderInput) error {
	workloadType := input.ComponentType.Spec.WorkloadType

	// Only apply to deployment and statefulset workload types
	if workloadType != workloadTypeDeployment && workloadType != workloadTypeStatefulSet {
		return nil
	}

	// Collect all non-workload dataplane resources
	resourcesToHash := make([]map[string]any, 0, len(resources))
	for _, rr := range resources {
		// Skip non-dataplane resources
		if rr.TargetPlane != v1alpha1.TargetPlaneDataPlane {
			continue
		}

		// Skip the main workload resource
		kind, _ := rr.Resource["kind"].(string)
		if isMainWorkloadKind(kind, workloadType) {
			continue
		}

		resourcesToHash = append(resourcesToHash, rr.Resource)
	}

	// If no non-workload dataplane resources, nothing to hash
	if len(resourcesToHash) == 0 {
		return nil
	}

	// Sort resources to ensure deterministic hash regardless of resource ordering
	sort.SliceStable(resourcesToHash, func(i, j int) bool {
		return compareResources(resourcesToHash[i], resourcesToHash[j])
	})

	// Extract content excluding metadata for hashing
	hashContent := make([]map[string]any, 0, len(resourcesToHash))
	for _, resource := range resourcesToHash {
		hashContent = append(hashContent, extractContentExcludingMetadata(resource))
	}

	// Compute hash
	resourceHash := hash.ComputeHash(hashContent, nil)

	// Find the main workload resource and add annotation to pod template
	for _, rr := range resources {
		kind, _ := rr.Resource["kind"].(string)
		if !isMainWorkloadKind(kind, workloadType) {
			continue
		}

		if err := addPodTemplateAnnotation(rr.Resource, labels.AnnotationKeyDPResourceHash, resourceHash); err != nil {
			return fmt.Errorf("failed to add annotation to %s: %w", kind, err)
		}
		break // Only one main workload
	}

	return nil
}

// isMainWorkloadKind returns true if the kind matches the expected main workload for the given workloadType.
func isMainWorkloadKind(kind, workloadType string) bool {
	switch workloadType {
	case workloadTypeDeployment:
		return kind == kindDeployment
	case workloadTypeStatefulSet:
		return kind == kindStatefulSet
	default:
		return false
	}
}

// extractContentExcludingMetadata returns a copy of the resource with metadata removed.
func extractContentExcludingMetadata(resource map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range resource {
		if k == "metadata" {
			continue
		}
		result[k] = v
	}
	return result
}

// addPodTemplateAnnotation adds an annotation to the pod template of a workload resource.
func addPodTemplateAnnotation(resource map[string]any, key, value string) error {
	// For Deployment/StatefulSet, pod template is at spec.template
	spec, ok := resource["spec"].(map[string]any)
	if !ok {
		return fmt.Errorf("resource missing spec")
	}

	template, ok := spec["template"].(map[string]any)
	if !ok {
		// Create template if it doesn't exist
		template = make(map[string]any)
		spec["template"] = template
	}

	templateMeta, ok := template["metadata"].(map[string]any)
	if !ok {
		templateMeta = make(map[string]any)
		template["metadata"] = templateMeta
	}

	annotations, ok := templateMeta["annotations"].(map[string]any)
	if !ok {
		annotations = make(map[string]any)
		templateMeta["annotations"] = annotations
	}

	annotations[key] = value
	return nil
}
