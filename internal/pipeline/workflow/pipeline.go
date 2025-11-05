// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package workflowpipeline provides workflow rendering by combining CRs and evaluating CEL expressions.
package workflowpipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

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

	if err := p.enrichContext(&input.Context); err != nil {
		return nil, fmt.Errorf("failed to enrich context: %w", err)
	}

	celContext, err := p.buildCELContext(input)
	if err != nil {
		return nil, fmt.Errorf("failed to build CEL context: %w", err)
	}

	resource, err := p.renderTemplate(input.WorkflowDefinition.Spec.Resource.Template, celContext)
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
	if input.Workflow == nil {
		return fmt.Errorf("workflow is nil")
	}
	if input.WorkflowDefinition == nil {
		return fmt.Errorf("workflow definition is nil")
	}
	if input.WorkflowDefinition.Spec.Resource.Template == nil {
		return fmt.Errorf("workflow definition has no resource template")
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

	return nil
}

// enrichContext adds auto-generated fields to the workflow context.
func (p *Pipeline) enrichContext(ctx *WorkflowContext) error {
	ctx.Timestamp = time.Now().Unix()

	uuid, err := generateShortUUID()
	if err != nil {
		return fmt.Errorf("failed to generate UUID: %w", err)
	}
	ctx.UUID = uuid

	return nil
}

// renderTemplate renders the workflow template with CEL context and post-processes the result.
func (p *Pipeline) renderTemplate(tmpl *runtime.RawExtension, celContext map[string]any) (map[string]any, error) {
	templateData, err := rawExtensionToMap(tmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource template: %w", err)
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

// buildCELContext builds the CEL evaluation context with ctx.*, schema.*, and fixedParameters.* variables.
func (p *Pipeline) buildCELContext(input *RenderInput) (map[string]any, error) {
	ctx := map[string]any{
		"orgName":       input.Context.OrgName,
		"projectName":   input.Context.ProjectName,
		"componentName": input.Context.ComponentName,
		"workflowName":  input.Context.WorkflowName,
		"timestamp":     input.Context.Timestamp,
		"uuid":          input.Context.UUID,
	}

	structural, err := p.buildStructuralSchema(input.WorkflowDefinition)
	if err != nil {
		return nil, err
	}

	developerParams, err := extractParameters(input.Workflow.Spec.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to extract workflow parameters: %w", err)
	}

	schemaParams := schema.ApplyDefaults(developerParams, structural)
	fixedParams := p.buildFixedParameters(input)

	return map[string]any{
		"ctx":             ctx,
		"schema":          schemaParams,
		"fixedParameters": fixedParams,
	}, nil
}

// buildStructuralSchema builds the structural schema from WorkflowDefinition for applying defaults.
func (p *Pipeline) buildStructuralSchema(wd *v1alpha1.WorkflowDefinition) (*apiextschema.Structural, error) {
	if wd.Spec.Schema == nil {
		return nil, nil
	}

	schemaMap, err := extractParameters(wd.Spec.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to extract schema: %w", err)
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

// buildFixedParameters merges fixed parameters from WorkflowDefinition and ComponentTypeDefinition.
// ComponentTypeDefinition parameters override WorkflowDefinition parameters.
func (p *Pipeline) buildFixedParameters(input *RenderInput) map[string]any {
	fixedParams := make(map[string]any)

	for _, param := range input.WorkflowDefinition.Spec.FixedParameters {
		fixedParams[param.Name] = param.Value
	}

	if input.ComponentTypeDefinition != nil && input.ComponentTypeDefinition.Spec.Build != nil {
		for _, allowedTemplate := range input.ComponentTypeDefinition.Spec.Build.AllowedTemplates {
			if allowedTemplate.Name == input.WorkflowDefinition.Name {
				for _, param := range allowedTemplate.FixedParameters {
					fixedParams[param.Name] = param.Value
				}
				break
			}
		}
	}

	return fixedParams
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
				result[key] = convertValueToString(val)
			} else {
				result[key] = convertComplexValuesToJSONStrings(val)
			}
		}
		return result

	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = convertComplexValuesToJSONStrings(val)
		}
		return result

	default:
		return v
	}
}

// convertValueToString converts values to their appropriate YAML representation.
// Arrays become FlowStyleArray, maps become JSON strings, primitives stay unchanged.
func convertValueToString(val any) any {
	switch v := val.(type) {
	case []any:
		return formatArrayInline(v)

	case map[string]any:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return v
		}
		return string(jsonBytes)

	case int, int64, int32, float64, float32, bool, string:
		return v

	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatArrayInline wraps an array in FlowStyleArray for inline YAML rendering.
func formatArrayInline(arr []any) any {
	return FlowStyleArray(arr)
}

// FlowStyleArray wraps arrays for flow-style YAML rendering (e.g., [1, 2, 3]).
type FlowStyleArray []any

// MarshalYAML implements yaml.Marshaler for flow-style array rendering.
func (f FlowStyleArray) MarshalYAML() (interface{}, error) {
	return []any(f), nil
}

// String returns the flow-style string representation for debugging.
func (f FlowStyleArray) String() string {
	result := "["
	for i, elem := range f {
		if i > 0 {
			result += ", "
		}
		switch v := elem.(type) {
		case string:
			escaped, _ := json.Marshal(v)
			result += string(escaped)
		default:
			result += fmt.Sprintf("%v", v)
		}
	}
	result += "]"
	return result
}

// convertFlowStyleArraysToSlices recursively converts FlowStyleArray to []any slices.
// Required because Kubernetes API client doesn't understand custom types.
func convertFlowStyleArraysToSlices(data any) any {
	switch v := data.(type) {
	case FlowStyleArray:
		result := make([]any, len(v))
		for i, elem := range v {
			result[i] = convertFlowStyleArraysToSlices(elem)
		}
		return result

	case map[string]any:
		result := make(map[string]any)
		for key, val := range v {
			result[key] = convertFlowStyleArraysToSlices(val)
		}
		return result

	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = convertFlowStyleArraysToSlices(val)
		}
		return result

	default:
		return v
	}
}
