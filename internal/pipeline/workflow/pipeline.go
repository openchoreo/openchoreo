// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package workflowpipeline provides workflow rendering by combining CRs and evaluating CEL expressions.
package workflowpipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/template"
)

// NewPipeline creates a new workflow rendering pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{
		templateEngine: template.NewEngine(),
	}
}

// Render orchestrates the complete workflow rendering process.
// It validates input, builds CEL context, renders the template, and validates output.
func (p *Pipeline) Render(input *RenderInput) (*RenderOutput, error) {
	if err := p.validateInput(input); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	metadata := &RenderMetadata{
		Warnings: []string{},
	}

	celContext, err := p.buildCELContext(input)
	if err != nil {
		return nil, fmt.Errorf("failed to build CEL context: %w", err)
	}

	resource, err := p.renderTemplate(input.Workflow.Spec.RunTemplate, celContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	if err := p.validateRenderedResource(resource); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Render additional resources if defined
	resources, err := p.renderResources(input.Workflow.Spec.Resources, celContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render resources: %w", err)
	}

	return &RenderOutput{
		Resource:  resource,
		Resources: resources,
		Metadata:  metadata,
	}, nil
}

// validateInput ensures the input has all required fields.
func (p *Pipeline) validateInput(input *RenderInput) error {
	if input == nil {
		return fmt.Errorf("input is nil")
	}
	if input.WorkflowRun == nil {
		return fmt.Errorf("workflow run is nil")
	}
	if input.Workflow == nil {
		return fmt.Errorf("workflow is nil")
	}
	if input.Workflow.Spec.RunTemplate == nil {
		return fmt.Errorf("workflow has no runTemplate")
	}

	if input.Context.NamespaceName == "" {
		return fmt.Errorf("context.namespaceName is required")
	}
	if input.Context.WorkflowRunName == "" {
		return fmt.Errorf("context.workflowRunName is required")
	}

	return nil
}

// renderTemplate renders the workflow template with CEL context and post-processes the result.
func (p *Pipeline) renderTemplate(tmpl *runtime.RawExtension, celContext map[string]any) (map[string]any, error) {
	templateData, err := rawExtensionToMap(tmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse runTemplate: %w", err)
	}

	rendered, err := p.templateEngine.Render(templateData, celContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render workflow resource: %w", err)
	}

	rendered = template.RemoveOmittedFields(rendered)
	rendered = convertComplexValuesToJSONStrings(rendered)
	rendered = convertFlowStyleArraysToSlices(rendered)

	resource, ok := rendered.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("rendered resource is not a map, got %T", rendered)
	}

	return resource, nil
}

// renderResources renders additional resources defined in the Workflow.
func (p *Pipeline) renderResources(resources []v1alpha1.WorkflowResource, celContext map[string]any) ([]RenderedResource, error) {
	if len(resources) == 0 {
		return nil, nil
	}

	renderedResources := make([]RenderedResource, 0, len(resources))
	for _, res := range resources {
		// Check if resource should be included based on includeWhen condition
		include, err := p.shouldIncludeResource(res.IncludeWhen, celContext)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate includeWhen for resource %q: %w", res.ID, err)
		}
		if !include {
			continue
		}

		rendered, err := p.renderTemplate(res.Template, celContext)
		if err != nil {
			return nil, fmt.Errorf("failed to render resource %q: %w", res.ID, err)
		}

		if err := p.validateRenderedResource(rendered); err != nil {
			return nil, fmt.Errorf("validation failed for resource %q: %w", res.ID, err)
		}

		// Skip resources with empty or invalid names (e.g., "-git-secret" when gitSecret.name is empty)
		if shouldSkipResource(rendered) {
			continue
		}

		renderedResources = append(renderedResources, RenderedResource{
			ID:       res.ID,
			Resource: rendered,
		})
	}

	return renderedResources, nil
}

// shouldIncludeResource evaluates the includeWhen expression to determine if a resource should be rendered.
// Returns true if includeWhen is empty (default behavior - resource is always created).
func (p *Pipeline) shouldIncludeResource(includeWhen string, context map[string]any) (bool, error) {
	if includeWhen == "" {
		return true, nil
	}

	result, err := p.templateEngine.Render(includeWhen, context)
	if err != nil {
		return false, err
	}

	boolResult, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("includeWhen must evaluate to boolean, got %T", result)
	}

	return boolResult, nil
}

// shouldSkipResource checks if a rendered resource should be skipped.
// Resources with empty or invalid names (e.g., starting with dash) are skipped.
func shouldSkipResource(resource map[string]any) bool {
	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return false
	}

	name, ok := metadata["name"].(string)
	if !ok {
		return false
	}

	if name == "" || strings.HasPrefix(name, "-") {
		return true
	}

	return false
}

// buildCELContext builds the CEL evaluation context with metadata.* and parameters.* variables.
func (p *Pipeline) buildCELContext(input *RenderInput) (map[string]any, error) {
	// Enforced namespace
	ciNamespace := fmt.Sprintf("openchoreo-ci-%s", input.Context.NamespaceName)

	metadata := map[string]any{
		"namespaceName":   input.Context.NamespaceName,
		"workflowRunName": input.Context.WorkflowRunName,
		"namespace":       ciNamespace, // Enforced CI namespace
	}

	// Build developer parameters with defaults applied from schema
	parameters, err := p.buildParameters(input)
	if err != nil {
		return nil, fmt.Errorf("failed to build parameters: %w", err)
	}

	return map[string]any{
		"metadata":   metadata,
		"parameters": parameters,
		"secretRef":  buildSecretRefCELContext(input.Context.SecretRef),
	}, nil
}

// buildSecretRefCELContext converts optional resolved secret reference data into CEL context shape.
// If no valid secret reference is provided, empty defaults are returned to keep template access safe.
func buildSecretRefCELContext(secretRef *SecretRefInfo) map[string]any {
	if secretRef == nil || secretRef.Name == "" || len(secretRef.Data) == 0 {
		return map[string]any{
			"name": "",
			"type": "",
			"data": []map[string]any{},
		}
	}

	dataArray := make([]map[string]any, len(secretRef.Data))
	for i, d := range secretRef.Data {
		dataArray[i] = map[string]any{
			"secretKey": d.SecretKey,
			"remoteRef": map[string]any{
				"key":      d.RemoteRef.Key,
				"property": d.RemoteRef.Property,
			},
		}
	}

	return map[string]any{
		"name": secretRef.Name,
		"type": secretRef.Type,
		"data": dataArray,
	}
}

// buildParameters builds the developer parameters with defaults applied from the Workflow schema.
func (p *Pipeline) buildParameters(input *RenderInput) (map[string]any, error) {
	// Build structural schema from Workflow for applying defaults
	structural, err := p.buildStructuralSchema(input.Workflow)
	if err != nil {
		return nil, err
	}

	// Extract developer parameters from WorkflowRun
	developerParams, err := extractParameters(input.WorkflowRun.Spec.Workflow.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract workflow run parameters: %w", err)
	}

	// Apply defaults from schema
	if structural != nil {
		return schema.ApplyDefaults(developerParams, structural), nil
	}

	return developerParams, nil
}

// buildStructuralSchema builds the structural schema from Workflow for applying defaults.
// Workflow.Spec.Schema has Types (optional) and Parameters (the actual schema).
func (p *Pipeline) buildStructuralSchema(wf *v1alpha1.Workflow) (*apiextschema.Structural, error) {
	if wf.Spec.Schema == nil {
		return nil, nil
	}

	// Extract types if present
	var types map[string]any
	if wf.Spec.Schema.Types != nil {
		if err := json.Unmarshal(wf.Spec.Schema.Types.Raw, &types); err != nil {
			return nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	// Extract parameters schema (the main schema for Workflow)
	var parameters map[string]any
	if wf.Spec.Schema.Parameters != nil {
		if err := json.Unmarshal(wf.Spec.Schema.Parameters.Raw, &parameters); err != nil {
			return nil, fmt.Errorf("failed to extract parameters schema: %w", err)
		}
	}

	if parameters == nil {
		return nil, nil
	}

	def := schema.Definition{
		Types:   types,
		Schemas: []map[string]any{parameters},
	}

	structural, err := schema.ToStructural(def)
	if err != nil {
		return nil, fmt.Errorf("failed to build structural schema: %w", err)
	}

	return structural, nil
}

// validateRenderedResource ensures the rendered resource has required Kubernetes fields.
func (p *Pipeline) validateRenderedResource(resource map[string]any) error {
	apiVersion, ok := resource["apiVersion"].(string)
	if !ok || apiVersion == "" {
		return fmt.Errorf("rendered resource missing apiVersion")
	}

	kind, ok := resource["kind"].(string)
	if !ok || kind == "" {
		return fmt.Errorf("rendered resource missing kind")
	}

	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("rendered resource missing metadata")
	}

	name, ok := metadata["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("rendered resource missing metadata.name")
	}

	return nil
}

// rawExtensionToMap converts a runtime.RawExtension to map[string]any.
func rawExtensionToMap(raw *runtime.RawExtension) (map[string]any, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw extension is nil")
	}

	var result map[string]any
	if err := json.Unmarshal(raw.Raw, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw extension: %w", err)
	}

	return result, nil
}

// generateShortUUID generates a short 8-character UUID for workflow naming.
func generateShortUUID() (string, error) {
	bytes := make([]byte, 4) // 4 bytes = 8 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// extractParameters unmarshals a runtime.RawExtension into a map for CEL evaluation.
// Returns an empty map if raw is nil (absent parameters are valid; defaults will be applied).
func extractParameters(raw *runtime.RawExtension) (map[string]any, error) {
	if raw == nil || raw.Raw == nil {
		return make(map[string]any), nil
	}

	var params map[string]any
	if err := json.Unmarshal(raw.Raw, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	return params, nil
}

// convertComplexValuesToJSONStrings recursively converts arrays and objects in "value" fields to JSON strings.
// This is required because Argo Workflow parameters expect scalar string values.
func convertComplexValuesToJSONStrings(data any) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, val := range v {
			if key == "value" {
				// If value is array or object, convert to JSON string
				switch val.(type) {
				case []any, map[string]any:
					if jsonBytes, err := json.Marshal(val); err == nil {
						result[key] = string(jsonBytes)
					} else {
						result[key] = val
					}
				default:
					result[key] = val
				}
			} else {
				result[key] = convertComplexValuesToJSONStrings(val)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = convertComplexValuesToJSONStrings(item)
		}
		return result
	default:
		return data
	}
}

// convertFlowStyleArraysToSlices recursively converts flow-style array strings to proper slices.
// Flow-style arrays are YAML arrays written as "[item1, item2]" which get parsed as strings.
func convertFlowStyleArraysToSlices(data any) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, val := range v {
			result[key] = convertFlowStyleArraysToSlices(val)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = convertFlowStyleArraysToSlices(item)
		}
		return result
	case string:
		// Try to parse as JSON array
		if len(v) > 0 && v[0] == '[' {
			var arr []any
			if err := json.Unmarshal([]byte(v), &arr); err == nil {
				return arr
			}
		}
		return v
	default:
		return data
	}
}
