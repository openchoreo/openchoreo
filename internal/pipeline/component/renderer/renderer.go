// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package renderer handles ResourceTemplate orchestration for ComponentTypes.
//
// The renderer evaluates ResourceTemplate control flow (includeWhen, forEach) and
// uses the template engine to render Kubernetes resources.
package renderer

import (
	"encoding/json"
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Renderer orchestrates the rendering of ResourceTemplates from ComponentTypes.
type Renderer struct {
	templateEngine *template.Engine
}

// NewRenderer creates a new ResourceTemplate renderer.
func NewRenderer(templateEngine *template.Engine) *Renderer {
	return &Renderer{
		templateEngine: templateEngine,
	}
}

// RenderedResource wraps a rendered Kubernetes resource with metadata about its target plane.
type RenderedResource struct {
	// Resource is the fully rendered Kubernetes resource manifest.
	Resource map[string]any

	// TargetPlane indicates which plane this resource should be deployed to.
	TargetPlane string
}

// RenderResources renders all resources from a ComponentType.
//
// The process:
//   - Iterate through ComponentType.Spec.Resources
//   - For each ResourceTemplate:
//   - Evaluate includeWhen (skip if false)
//   - Check forEach (render multiple times if present, supports arrays and maps)
//   - Render template field using template engine
//   - Return all rendered resources with their target planes
//
// For forEach with maps, keys are iterated in sorted order with each item having
// .key and .value fields.
//
// Returns an error if any template fails to render.
func (r *Renderer) RenderResources(
	templates []v1alpha1.ResourceTemplate,
	context map[string]any,
) ([]RenderedResource, error) {
	resources := make([]RenderedResource, 0, len(templates))

	for _, tmpl := range templates {
		// Check if resource should be included
		include, err := ShouldInclude(r.templateEngine, tmpl.IncludeWhen, context)
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
			// Wrap each rendered resource with target plane
			for _, res := range rendered {
				resources = append(resources, RenderedResource{
					Resource:    res,
					TargetPlane: tmpl.TargetPlane,
				})
			}
			continue
		}

		// Render single resource
		rendered, err := r.renderSingleResource(tmpl, context)
		if err != nil {
			return nil, err
		}
		resources = append(resources, RenderedResource{
			Resource:    rendered,
			TargetPlane: tmpl.TargetPlane,
		})
	}

	return resources, nil
}

// renderWithForEach handles ResourceTemplate.forEach iteration.
// Delegates to the shared EvalForEach helper for iteration logic.
func (r *Renderer) renderWithForEach(
	tmpl v1alpha1.ResourceTemplate,
	context map[string]any,
) ([]map[string]any, error) {
	itemContexts, err := EvalForEach(r.templateEngine, tmpl.ForEach, tmpl.Var, context)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate forEach for resource %s: %w", tmpl.ID, err)
	}

	resources := make([]map[string]any, 0, len(itemContexts))
	for _, itemContext := range itemContexts {
		rendered, err := r.renderSingleResource(tmpl, itemContext)
		if err != nil {
			return nil, err
		}
		resources = append(resources, rendered)
	}

	return resources, nil
}

// renderSingleResource renders a single ResourceTemplate.
//
// The process:
//   - Extract template from runtime.RawExtension
//   - Render using template engine
//   - Remove omitted fields
//   - Validate basic structure (kind, apiVersion, metadata.name)
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
