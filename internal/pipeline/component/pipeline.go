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
)

var validate = validator.New(validator.WithRequiredStructEnabled())

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

	// Build component context
	componentContext, err := context.BuildComponentContext(&context.ComponentContextInput{
		Component:        input.Component,
		ComponentType:    input.ComponentType,
		Workload:         input.Workload,
		ReleaseBinding:   input.ReleaseBinding,
		DataPlane:        input.DataPlane,
		SecretReferences: input.SecretReferences,
		Metadata:         input.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build component context: %w", err)
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

	// Process each trait instance from the component
	for _, traitInstance := range input.Component.Spec.Traits {
		t, ok := traitMap[traitInstance.Name]
		if !ok {
			return nil, fmt.Errorf("trait %s referenced but not found in traits list", traitInstance.Name)
		}

		// Build trait context (BuildTraitContext will handle schema caching)
		traitContext, err := context.BuildTraitContext(&context.TraitContextInput{
			Trait:          t,
			Instance:       traitInstance,
			Component:      input.Component,
			ReleaseBinding: input.ReleaseBinding,
			Metadata:       input.Metadata,
			SchemaCache:    schemaCache,
			DataPlane:      input.DataPlane,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build trait context for %s/%s: %w",
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

// postProcessResources adds labels and performs cleanup.
func (p *Pipeline) postProcessResources(resources []renderer.RenderedResource, input *RenderInput) error {
	commonLabels := map[string]string{
		labels.LabelKeyComponentName:   input.Metadata.ComponentName,
		labels.LabelKeyEnvironmentName: input.Metadata.EnvironmentName,
		labels.LabelKeyProjectName:     input.Metadata.ProjectName,
	}
	for _, rr := range resources {
		if err := addLabels(rr.Resource, commonLabels); err != nil {
			return fmt.Errorf("failed to add labels: %w", err)
		}
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

	ns1, _ := meta1["namespace"].(string)
	ns2, _ := meta2["namespace"].(string)
	if ns1 != ns2 {
		return ns1 < ns2
	}

	name1, _ := meta1["name"].(string)
	name2, _ := meta2["name"].(string)
	return name1 < name2
}
