// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package component provides the main rendering pipeline for Component resources.
//
// The pipeline combines Component, ComponentTypeDefinition, Addons, Workload and ComponentDeployment
// to generate fully resolved Kubernetes resource manifests by:
//  1. Building CEL evaluation contexts with parameters, overrides, and defaults
//  2. Rendering base resources from ComponentTypeDefinition
//  3. Processing addons (creates and patches)
//  4. Post-processing (validation, labels, annotations)
package component

import (
	"fmt"
	"maps"
	"sort"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/addon"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/template"
)

// NewPipeline creates a new component rendering pipeline.
func NewPipeline(opts ...Option) *Pipeline {
	p := &Pipeline{
		templateEngine: template.NewEngine(),
		options:        DefaultRenderOptions(),
	}

	// Apply options
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Render orchestrates the complete rendering workflow for a Component.
//
// Workflow:
//  1. Validate input
//  2. Build component context (parameters + overrides + defaults)
//  3. Render base resources from ComponentTypeDefinition
//  4. Process addons (creates and patches)
//  5. Post-process (validate, add labels/annotations, sort)
//  6. Return output
//
// Returns an error if any step fails.
func (p *Pipeline) Render(input *RenderInput) (*RenderOutput, error) {
	// 1. Validate input
	if err := p.validateInput(input); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	metadata := &RenderMetadata{
		Warnings: []string{},
	}

	// 2. Build component context
	componentContext, err := context.BuildComponentContext(&context.ComponentContextInput{
		Component:               input.Component,
		ComponentTypeDefinition: input.ComponentTypeDefinition,
		Workload:                input.Workload,
		Environment:             input.Environment,
		ComponentDeployment:     input.ComponentDeployment,
		Metadata:                input.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build component context: %w", err)
	}

	// 3. Render base resources from ComponentTypeDefinition
	resourceRenderer := renderer.NewRenderer(p.templateEngine)
	resources, err := resourceRenderer.RenderResources(
		input.ComponentTypeDefinition.Spec.Resources,
		componentContext,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to render base resources: %w", err)
	}
	metadata.BaseResourceCount = len(resources)

	// 4. Process addons
	addonProcessor := addon.NewProcessor(p.templateEngine)

	// Build addon map
	addonMap := make(map[string]*v1alpha1.Addon)
	for i := range input.Addons {
		addon := &input.Addons[i]
		addonMap[addon.Name] = addon
	}

	// Create schema cache for addon reuse within this render
	schemaCache := make(map[string]*apiextschema.Structural)

	// Process each addon instance from the component
	for _, addonInstance := range input.Component.Spec.Addons {
		addon, ok := addonMap[addonInstance.Name]
		if !ok {
			return nil, fmt.Errorf("addon %s referenced but not found in addons list", addonInstance.Name)
		}

		// Build addon context (BuildAddonContext will handle schema caching)
		addonContext, err := context.BuildAddonContext(&context.AddonContextInput{
			Addon:               addon,
			Instance:            addonInstance,
			Component:           input.Component,
			Environment:         input.Environment,
			ComponentDeployment: input.ComponentDeployment,
			Metadata:            input.Metadata,
			SchemaCache:         schemaCache,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build addon context for %s/%s: %w",
				addonInstance.Name, addonInstance.InstanceName, err)
		}

		// Process addon (creates + patches)
		resources, err = addonProcessor.ProcessAddons(resources, addon, addonContext)
		if err != nil {
			return nil, fmt.Errorf("failed to process addon %s/%s: %w",
				addonInstance.Name, addonInstance.InstanceName, err)
		}

		metadata.AddonCount++
	}

	metadata.AddonResourceCount = len(resources) - metadata.BaseResourceCount

	// 5. Post-process resources
	if err := p.postProcessResources(resources, input); err != nil {
		return nil, fmt.Errorf("failed to post-process resources: %w", err)
	}

	// 6. Validate if enabled
	if p.options.EnableValidation {
		if err := p.validateResources(resources); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	// Sort resources for deterministic output
	sortResources(resources)

	metadata.ResourceCount = len(resources)

	return &RenderOutput{
		Resources: resources,
		Metadata:  metadata,
	}, nil
}

// validateInput ensures the input has all required fields.
func (p *Pipeline) validateInput(input *RenderInput) error {
	if input == nil {
		return fmt.Errorf("input is nil")
	}
	if input.ComponentTypeDefinition == nil {
		return fmt.Errorf("component type definition is nil")
	}
	if input.ComponentTypeDefinition.Spec.Resources == nil {
		return fmt.Errorf("component type definition has no resources")
	}
	if input.Component == nil {
		return fmt.Errorf("component is nil")
	}
	if input.Workload == nil {
		return fmt.Errorf("workload is nil")
	}
	if input.Environment == "" {
		return fmt.Errorf("environment is required")
	}

	// Validate metadata context
	if input.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if input.Metadata.Namespace == "" {
		return fmt.Errorf("metadata.namespace is required")
	}

	return nil
}

// postProcessResources adds labels, annotations, and performs cleanup.
func (p *Pipeline) postProcessResources(resources []map[string]any, input *RenderInput) error {
	// Build common labels/annotations
	commonLabels := make(map[string]string)
	commonAnnotations := make(map[string]string)

	// Add component metadata
	if input.Component != nil {
		commonLabels["openchoreo.org/component"] = input.Component.Name
		commonLabels["openchoreo.org/environment"] = input.Environment
	}

	// Add configured labels/annotations
	maps.Copy(commonLabels, p.options.ResourceLabels)
	maps.Copy(commonAnnotations, p.options.ResourceAnnotations)

	// Apply to all resources
	for _, resource := range resources {
		if err := addLabelsAndAnnotations(resource, commonLabels, commonAnnotations); err != nil {
			return fmt.Errorf("failed to add labels/annotations: %w", err)
		}
	}

	return nil
}

// addLabelsAndAnnotations adds labels and annotations to a resource.
func addLabelsAndAnnotations(resource map[string]any, labels, annotations map[string]string) error {
	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("resource missing metadata")
	}

	// Add labels
	if len(labels) > 0 {
		existingLabels, _ := metadata["labels"].(map[string]any)
		if existingLabels == nil {
			existingLabels = make(map[string]any)
		}
		for k, v := range labels {
			existingLabels[k] = v
		}
		metadata["labels"] = existingLabels
	}

	// Add annotations
	if len(annotations) > 0 {
		existingAnnotations, _ := metadata["annotations"].(map[string]any)
		if existingAnnotations == nil {
			existingAnnotations = make(map[string]any)
		}
		for k, v := range annotations {
			existingAnnotations[k] = v
		}
		metadata["annotations"] = existingAnnotations
	}

	return nil
}

// validateResources performs basic validation on rendered resources.
func (p *Pipeline) validateResources(resources []map[string]any) error {
	for i, resource := range resources {
		// Try to extract resource identity for better error messages
		kind, _ := resource["kind"].(string)
		apiVersion, _ := resource["apiVersion"].(string)
		var resourceID string
		if kind != "" {
			resourceID = fmt.Sprintf("resource #%d (%s)", i, kind)
		} else {
			resourceID = fmt.Sprintf("resource #%d", i)
		}

		// Check required fields
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

// sortResources sorts resources for deterministic output.
// Sorts by: kind, apiVersion, metadata.namespace, metadata.name
func sortResources(resources []map[string]any) {
	sort.Slice(resources, func(i, j int) bool {
		// Extract fields
		kind1, _ := resources[i]["kind"].(string)
		kind2, _ := resources[j]["kind"].(string)
		if kind1 != kind2 {
			return kind1 < kind2
		}

		apiVersion1, _ := resources[i]["apiVersion"].(string)
		apiVersion2, _ := resources[j]["apiVersion"].(string)
		if apiVersion1 != apiVersion2 {
			return apiVersion1 < apiVersion2
		}

		meta1, _ := resources[i]["metadata"].(map[string]any)
		meta2, _ := resources[j]["metadata"].(map[string]any)

		ns1, _ := meta1["namespace"].(string)
		ns2, _ := meta2["namespace"].(string)
		if ns1 != ns2 {
			return ns1 < ns2
		}

		name1, _ := meta1["name"].(string)
		name2, _ := meta2["name"].(string)
		return name1 < name2
	})
}
