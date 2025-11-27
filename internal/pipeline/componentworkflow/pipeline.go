// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package componentworkflowpipeline provides component workflow rendering by combining CRs and evaluating CEL expressions.
package componentworkflowpipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/template"
)

// NewPipeline creates a new component workflow rendering pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{
		templateEngine: template.NewEngine(),
	}
}

// Render orchestrates the complete component workflow rendering process.
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

	resource, err := p.renderTemplate(input.ComponentWorkflow.Spec.RunTemplate, celContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	if err := p.validateRenderedResource(resource); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &RenderOutput{
		Resource: resource,
		Metadata: metadata,
	}, nil
}

// validateInput ensures the input has all required fields.
func (p *Pipeline) validateInput(input *RenderInput) error {
	if input == nil {
		return fmt.Errorf("input is nil")
	}
	if input.ComponentWorkflowRun == nil {
		return fmt.Errorf("component workflow run is nil")
	}
	if input.ComponentWorkflow == nil {
		return fmt.Errorf("component workflow is nil")
	}
	if input.ComponentWorkflow.Spec.RunTemplate == nil {
		return fmt.Errorf("component workflow has no runTemplate")
	}

	if input.Context.OrgName == "" {
		return fmt.Errorf("context.orgName is required")
	}
	if input.Context.ProjectName == "" {
		return fmt.Errorf("context.projectName is required")
	}
	if input.Context.ComponentName == "" {
		return fmt.Errorf("context.componentName is required")
	}
	if input.Context.ComponentWorkflowRunName == "" {
		return fmt.Errorf("context.componentWorkflowRunName is required")
	}

	return nil
}

// renderTemplate renders the component workflow template with CEL context and post-processes the result.
func (p *Pipeline) renderTemplate(tmpl *runtime.RawExtension, celContext map[string]any) (map[string]any, error) {
	templateData, err := rawExtensionToMap(tmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse runTemplate: %w", err)
	}

	rendered, err := p.templateEngine.Render(templateData, celContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render component workflow resource: %w", err)
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

// buildCELContext builds the CEL evaluation context with ctx.*, systemParameters.*, and parameters.* variables.
func (p *Pipeline) buildCELContext(input *RenderInput) (map[string]any, error) {
	ctx := map[string]any{
		"orgName":                  input.Context.OrgName,
		"projectName":              input.Context.ProjectName,
		"componentName":            input.Context.ComponentName,
		"componentWorkflowRunName": input.Context.ComponentWorkflowRunName,
	}

	// Build system parameters - these are the actual values from ComponentWorkflowRun
	systemParameters := buildSystemParameters(input.ComponentWorkflowRun.Spec.Workflow.SystemParameters)

	// Build developer parameters with defaults applied from schema
	parameters, err := p.buildParameters(input)
	if err != nil {
		return nil, fmt.Errorf("failed to build parameters: %w", err)
	}

	return map[string]any{
		"ctx":              ctx,
		"systemParameters": systemParameters,
		"parameters":       parameters,
	}, nil
}

// buildSystemParameters converts the system parameters values to a map for CEL context.
func buildSystemParameters(sysParams v1alpha1.SystemParametersValues) map[string]any {
	return map[string]any{
		"repository": map[string]any{
			"url": sysParams.Repository.URL,
			"revision": map[string]any{
				"branch": sysParams.Repository.Revision.Branch,
				"commit": sysParams.Repository.Revision.Commit,
			},
			"appPath": sysParams.Repository.AppPath,
		},
	}
}

// buildParameters builds the developer parameters with defaults applied from the ComponentWorkflow schema.
func (p *Pipeline) buildParameters(input *RenderInput) (map[string]any, error) {
	// Build structural schema from ComponentWorkflow for applying defaults
	structural, err := p.buildStructuralSchema(input.ComponentWorkflow)
	if err != nil {
		return nil, err
	}

	// Extract developer parameters from ComponentWorkflowRun
	developerParams, err := extractParameters(input.ComponentWorkflowRun.Spec.Workflow.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract component workflow run parameters: %w", err)
	}

	// Apply defaults from schema
	if structural != nil {
		return schema.ApplyDefaults(developerParams, structural), nil
	}

	return developerParams, nil
}

// buildStructuralSchema builds the structural schema from ComponentWorkflow for applying defaults.
func (p *Pipeline) buildStructuralSchema(cwf *v1alpha1.ComponentWorkflow) (*apiextschema.Structural, error) {
	if cwf.Spec.Schema.Parameters == nil {
		return nil, nil
	}

	schemaMap, err := extractParameters(cwf.Spec.Schema.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract schema parameters: %w", err)
	}

	def := schema.Definition{
		Types:   make(map[string]any),
		Schemas: []map[string]any{schemaMap},
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
