// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package renderer handles ResourceTemplate orchestration for ComponentTypeDefinitions.
//
// The renderer evaluates ResourceTemplate control flow (includeWhen, forEach) and
// uses the template engine to render Kubernetes resources.
package renderer

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/clone"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Renderer orchestrates the rendering of ResourceTemplates from ComponentTypeDefinitions.
type Renderer struct {
	templateEngine *template.Engine
}

// NewRenderer creates a new ResourceTemplate renderer.
func NewRenderer(templateEngine *template.Engine) *Renderer {
	return &Renderer{
		templateEngine: templateEngine,
	}
}

// RenderResources renders all resources from a ComponentTypeDefinition.
//
// The process:
//  1. Iterate through ComponentTypeDefinition.Spec.Resources
//  2. For each ResourceTemplate:
//     - Evaluate includeWhen (skip if false)
//     - Check forEach (render multiple times if present)
//     - Render template field using template engine
//  3. Return all rendered resources
//
// Returns an error if any template fails to render (unless it's a missing data error
// for includeWhen evaluation).
func (r *Renderer) RenderResources(
	templates []v1alpha1.ResourceTemplate,
	context map[string]any,
) ([]map[string]any, error) {
	resources := make([]map[string]any, 0, len(templates))

	for _, tmpl := range templates {
		// Check if resource should be included
		include, err := r.shouldInclude(tmpl, context)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate includeWhen for resource %s: %w", tmpl.ID, err)
		}
		if !include {
			continue
		}

		// Handle forEach iteration
		if tmpl.ForEach != "" {
			rendered, err := r.renderWithForEach(tmpl, context)
			if err != nil {
				return nil, err
			}
			resources = append(resources, rendered...)
			continue
		}

		// Render single resource
		rendered, err := r.renderSingleResource(tmpl, context)
		if err != nil {
			return nil, err
		}
		resources = append(resources, rendered)
	}

	return resources, nil
}

// shouldInclude evaluates the ResourceTemplate.includeWhen condition.
//
// Returns:
//   - true if includeWhen is not set (default)
//   - true if includeWhen evaluates to true
//   - false if includeWhen evaluates to false
//   - false if includeWhen evaluation fails with missing data (graceful degradation)
//   - error for other evaluation failures
func (r *Renderer) shouldInclude(tmpl v1alpha1.ResourceTemplate, context map[string]any) (bool, error) {
	if tmpl.IncludeWhen == "" {
		return true, nil
	}

	result, err := r.templateEngine.Render(tmpl.IncludeWhen, context)
	if err != nil {
		// Gracefully handle missing data - treat as false
		if template.IsMissingDataError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to evaluate includeWhen expression: %w", err)
	}

	boolResult, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("includeWhen must evaluate to bool, got %T", result)
	}

	return boolResult, nil
}

// renderWithForEach handles ResourceTemplate.forEach iteration.
//
// The process:
//  1. Evaluate forEach expression to get array of items
//  2. For each item:
//     - Clone context
//     - Bind item to variable (tmpl.var or "item")
//     - Render template with item context
//  3. Return all rendered resources
func (r *Renderer) renderWithForEach(
	tmpl v1alpha1.ResourceTemplate,
	context map[string]any,
) ([]map[string]any, error) {
	// Evaluate forEach expression
	result, err := r.templateEngine.Render(tmpl.ForEach, context)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate forEach expression for resource %s: %w", tmpl.ID, err)
	}

	// Ensure result is an array
	items, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("forEach must evaluate to array for resource %s, got %T", tmpl.ID, result)
	}

	// Determine variable name
	varName := tmpl.Var
	if varName == "" {
		varName = "item"
	}

	// Render template for each item
	resources := make([]map[string]any, 0, len(items))
	for i, item := range items {
		// Clone context and bind item
		itemContext := clone.DeepCopyMap(context)
		itemContext[varName] = item

		// Render resource with item context
		rendered, err := r.renderSingleResource(tmpl, itemContext)
		if err != nil {
			return nil, fmt.Errorf("failed to render forEach iteration %d for resource %s: %w", i, tmpl.ID, err)
		}

		resources = append(resources, rendered)
	}

	return resources, nil
}

// renderSingleResource renders a single ResourceTemplate.
//
// The process:
//  1. Extract template from runtime.RawExtension
//  2. Render using template engine
//  3. Remove omitted fields
//  4. Validate basic structure (kind, apiVersion, metadata.name)
func (r *Renderer) renderSingleResource(
	tmpl v1alpha1.ResourceTemplate,
	context map[string]any,
) (map[string]any, error) {
	// Extract template structure
	var templateData any
	if err := json.Unmarshal(tmpl.Template.Raw, &templateData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal template for resource %s: %w", tmpl.ID, err)
	}

	// Render template
	rendered, err := r.templateEngine.Render(templateData, context)
	if err != nil {
		return nil, fmt.Errorf("failed to render template for resource %s: %w", tmpl.ID, err)
	}

	// Ensure result is an object
	resourceMap, ok := rendered.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template must render to object for resource %s, got %T", tmpl.ID, rendered)
	}

	// Remove omitted fields
	cleanedAny := template.RemoveOmittedFields(resourceMap)
	cleaned, ok := cleanedAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("RemoveOmittedFields returned unexpected type %T for resource %s", cleanedAny, tmpl.ID)
	}

	// Validate basic structure
	if err := validateResource(cleaned, tmpl.ID); err != nil {
		return nil, err
	}

	return cleaned, nil
}

// validateResource checks that a rendered resource has required fields.
func validateResource(resource map[string]any, resourceID string) error {
	// Check kind
	kind, ok := resource["kind"].(string)
	if !ok || kind == "" {
		return fmt.Errorf("resource %s missing required field 'kind'", resourceID)
	}

	// Check apiVersion
	apiVersion, ok := resource["apiVersion"].(string)
	if !ok || apiVersion == "" {
		return fmt.Errorf("resource %s missing required field 'apiVersion'", resourceID)
	}

	// Check metadata.name
	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("resource %s missing required field 'metadata'", resourceID)
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("resource %s missing required field 'metadata.name'", resourceID)
	}

	return nil
}

// ExtractTemplateData extracts the template data from a ResourceTemplate.
// This is a helper for testing.
func ExtractTemplateData(tmpl v1alpha1.ResourceTemplate) (any, error) {
	if tmpl.Template.Raw == nil {
		return nil, fmt.Errorf("template is nil")
	}

	var data any
	if err := json.Unmarshal(tmpl.Template.Raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}

	return data, nil
}

// MustRawExtension creates a runtime.RawExtension from any value.
// Panics if marshaling fails. For testing only.
func MustRawExtension(v any) runtime.RawExtension {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal value: %v", err))
	}
	return runtime.RawExtension{Raw: data}
}
