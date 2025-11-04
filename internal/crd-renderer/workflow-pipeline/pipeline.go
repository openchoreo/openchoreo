// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package workflowpipeline provides the main rendering pipeline for Workflow resources.
//
// The pipeline combines Workflow, WorkflowDefinition, and ComponentTypeDefinition
// to generate fully resolved workflow resources (e.g., Argo Workflow) by:
//  1. Building CEL evaluation contexts with parameters, overrides, and context variables
//  2. Rendering the workflow resource from WorkflowDefinition template
//  3. Evaluating all CEL expressions in the template
//  4. Post-processing (validation)
package workflowpipeline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/internal/crd-renderer/template"
)

// NewPipeline creates a new workflow rendering pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{
		templateEngine: template.NewEngine(),
	}
}

// Render orchestrates the complete rendering workflow for a Workflow.
//
// Workflow:
//  1. Validate input
//  2. Build workflow context (parameters + fixed parameters + context variables)
//  3. Render workflow resource from WorkflowDefinition template
//  4. Post-process (validate)
//  5. Return output
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

	// 2. Build workflow context
	input.Context.Timestamp = time.Now().Unix()

	uuid, err := generateShortUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", err)
	}
	input.Context.UUID = uuid

	// Build CEL evaluation context
	celContext, err := p.buildCELContext(input)
	if err != nil {
		return nil, fmt.Errorf("failed to build CEL context: %w", err)
	}

	// 3. Render workflow resource from template
	templateData, err := rawExtensionToMap(input.WorkflowDefinition.Spec.Resource.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource template: %w", err)
	}

	rendered, err := p.templateEngine.Render(templateData, celContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render workflow resource: %w", err)
	}

	// Remove any omitted fields
	rendered = template.RemoveOmittedFields(rendered)

	// Convert arrays and objects to JSON strings
	rendered = convertComplexValuesToJSONStrings(rendered)

	// Convert FlowStyleArray back to regular arrays for Kubernetes API
	rendered = convertFlowStyleArraysToSlices(rendered)

	// Convert to map[string]any
	resource, ok := rendered.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("rendered resource is not a map, got %T", rendered)
	}

	// 4. Validate rendered resource
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

	// Validate context
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

// buildCELContext builds the CEL evaluation context with all variables.
//
// The context includes:
//   - ctx.* - Context variables (orgName, projectName, componentName, workflowName, timestamp, uuid)
//   - schema.* - Developer-provided parameters from Workflow.spec.parameters
//   - fixedParameters.* - PE-controlled parameters merged from WorkflowDefinition and ComponentTypeDefinition
func (p *Pipeline) buildCELContext(input *RenderInput) (map[string]any, error) {
	// Build context variables
	ctx := map[string]any{
		"orgName":       input.Context.OrgName,
		"projectName":   input.Context.ProjectName,
		"componentName": input.Context.ComponentName,
		"workflowName":  input.Context.WorkflowName,
		"timestamp":     input.Context.Timestamp,
		"uuid":          input.Context.UUID,
	}

	// Extract schema defaults from WorkflowDefinition
	schemaDefaults := make(map[string]any)
	if input.WorkflowDefinition.Spec.Schema != nil {
		var err error
		schemaDefaults, err = extractSchemaDefaults(input.WorkflowDefinition.Spec.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to extract schema defaults: %w", err)
		}
	}

	// Extract developer-provided parameters from Workflow
	var developerParams map[string]any
	if input.Workflow.Spec.Schema != nil {
		var err error
		developerParams, err = rawExtensionToMap(input.Workflow.Spec.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow Schema: %w", err)
		}
	}
	if developerParams == nil {
		developerParams = make(map[string]any)
	}

	// Merge schema defaults with developer-provided parameters
	// Developer parameters override defaults
	schema := deepMergeMaps(schemaDefaults, developerParams)

	// Build fixed parameters map
	// Start with WorkflowDefinition fixed parameters
	fixedParams := make(map[string]any)
	for _, param := range input.WorkflowDefinition.Spec.FixedParameters {
		fixedParams[param.Name] = param.Value
	}

	// Override with ComponentTypeDefinition fixed parameters if present
	if input.ComponentTypeDefinition != nil && input.ComponentTypeDefinition.Spec.Build != nil {
		// Find the matching allowed template
		for _, allowedTemplate := range input.ComponentTypeDefinition.Spec.Build.AllowedTemplates {
			if allowedTemplate.Name == input.WorkflowDefinition.Name {
				// Apply overrides
				for _, param := range allowedTemplate.FixedParameters {
					fixedParams[param.Name] = param.Value
				}
				break
			}
		}
	}

	// Return CEL context with all variables
	return map[string]any{
		"ctx":             ctx,
		"schema":          schema,
		"fixedParameters": fixedParams,
	}, nil
}

// validateRenderedResource performs basic validation on the rendered resource.
func (p *Pipeline) validateRenderedResource(resource map[string]any) error {
	// Check required fields
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

// extractSchemaDefaults extracts default values from a schema definition.
// Schema format uses shorthand syntax: "type | default=value | required=true | enum=val1,val2"
// Returns a nested map with default values extracted from the schema.
func extractSchemaDefaults(schemaRaw *runtime.RawExtension) (map[string]any, error) {
	if schemaRaw == nil {
		return make(map[string]any), nil
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(schemaRaw.Raw, &schemaMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
	}

	return extractDefaultsFromMap(schemaMap), nil
}

// extractDefaultsFromMap recursively extracts default values from a schema map.
func extractDefaultsFromMap(schemaMap map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range schemaMap {
		switch v := value.(type) {
		case string:
			// String value is a type definition like "string | default=main"
			if defaultVal := parseDefault(v); defaultVal != nil {
				result[key] = defaultVal
			}
		case map[string]any:
			// Nested object - recurse
			nested := extractDefaultsFromMap(v)
			if len(nested) > 0 {
				result[key] = nested
			}
		}
	}

	return result
}

// parseDefault extracts the default value from a type definition string.
// Format: "type | default=value | required=true | enum=[val1,val2]"
// Supports: strings, numbers, booleans, arrays (e.g., default=[], default=[1,2,3], default=["a","b"])
// Returns nil if no default is specified.
func parseDefault(typeDef string) any {
	// Split by | to get parts
	parts := splitAndTrim(typeDef, "|")

	for _, part := range parts {
		// Check if this part is a default declaration
		if len(part) > 8 && part[:8] == "default=" {
			defaultValue := part[8:]

			// Handle array defaults
			if len(defaultValue) >= 2 && defaultValue[0] == '[' && defaultValue[len(defaultValue)-1] == ']' {
				return parseArrayDefault(defaultValue)
			}

			// Try to parse as number
			if num, ok := parseNumber(defaultValue); ok {
				return num
			}

			// Try to parse as boolean
			if defaultValue == "true" {
				return true
			}
			if defaultValue == "false" {
				return false
			}

			// Return as string
			return defaultValue
		}
	}

	return nil
}

// parseArrayDefault parses an array default value like [], [1,2,3], or ["a","b","c"].
func parseArrayDefault(arrayStr string) any {
	// Remove [ and ]
	if len(arrayStr) < 2 {
		return []any{}
	}

	inner := arrayStr[1 : len(arrayStr)-1]
	inner = trimSpace(inner)

	// Empty array
	if inner == "" {
		return []any{}
	}

	// Split by comma
	elements := splitArrayElements(inner)
	result := make([]any, 0, len(elements))

	for _, elem := range elements {
		elem = trimSpace(elem)
		if elem == "" {
			continue
		}

		// Remove quotes if string
		if len(elem) >= 2 && elem[0] == '"' && elem[len(elem)-1] == '"' {
			result = append(result, elem[1:len(elem)-1])
			continue
		}
		if len(elem) >= 2 && elem[0] == '\'' && elem[len(elem)-1] == '\'' {
			result = append(result, elem[1:len(elem)-1])
			continue
		}

		// Try parsing as number
		if num, ok := parseNumber(elem); ok {
			result = append(result, num)
			continue
		}

		// Try parsing as boolean
		if elem == "true" {
			result = append(result, true)
			continue
		}
		if elem == "false" {
			result = append(result, false)
			continue
		}

		// Default to string
		result = append(result, elem)
	}

	return result
}

// splitArrayElements splits array elements by comma, respecting quoted strings.
func splitArrayElements(s string) []string {
	var result []string
	var current string
	inQuote := false
	quoteChar := rune(0)

	for i := 0; i < len(s); i++ {
		ch := rune(s[i])

		if ch == '"' || ch == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = ch
				current += string(ch)
			} else if ch == quoteChar {
				inQuote = false
				quoteChar = 0
				current += string(ch)
			} else {
				current += string(ch)
			}
		} else if ch == ',' && !inQuote {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

// splitAndTrim splits a string by delimiter and trims whitespace from each part.
func splitAndTrim(s string, delimiter string) []string {
	parts := []string{}
	for _, part := range splitString(s, delimiter) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// splitString splits a string by delimiter (simple implementation).
func splitString(s string, delimiter string) []string {
	if delimiter == "" {
		return []string{s}
	}

	var result []string
	current := ""
	delimLen := len(delimiter)

	for i := 0; i < len(s); i++ {
		if i+delimLen <= len(s) && s[i:i+delimLen] == delimiter {
			result = append(result, current)
			current = ""
			i += delimLen - 1
		} else {
			current += string(s[i])
		}
	}
	result = append(result, current)
	return result
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)

	// Trim leading whitespace
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	// Trim trailing whitespace
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

// parseNumber attempts to parse a string as a number (int or float).
func parseNumber(s string) (any, bool) {
	// Try parsing as int
	var intVal int
	if _, err := fmt.Sscanf(s, "%d", &intVal); err == nil {
		return intVal, true
	}

	// Try parsing as float
	var floatVal float64
	if _, err := fmt.Sscanf(s, "%f", &floatVal); err == nil {
		return floatVal, true
	}

	return nil, false
}

// deepMergeMaps recursively merges two maps, with values from 'override' taking precedence.
func deepMergeMaps(base, override map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy all base values
	for k, v := range base {
		result[k] = v
	}

	// Override with values from override map
	for k, v := range override {
		if baseVal, exists := result[k]; exists {
			// If both are maps, merge recursively
			if baseMap, baseIsMap := baseVal.(map[string]any); baseIsMap {
				if overrideMap, overrideIsMap := v.(map[string]any); overrideIsMap {
					result[k] = deepMergeMaps(baseMap, overrideMap)
					continue
				}
			}
		}
		// Otherwise, override takes precedence
		result[k] = v
	}

	return result
}

// convertComplexValuesToJSONStrings recursively traverses the rendered structure
// and converts arrays and objects that appear as parameter values to JSON strings.
// This is necessary because Argo Workflow parameters expect scalar string values,
// but arrays/objects need to be passed as JSON-encoded strings.
func convertComplexValuesToJSONStrings(data any) any {
	switch v := data.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, val := range v {
			// Special handling for "value" fields in parameter lists
			// These should have arrays/objects converted to JSON strings
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

// convertValueToString converts a value to its appropriate representation for YAML.
// - Arrays are formatted as JSON-style strings (without outer quotes)
// - Maps are formatted as JSON strings
// - Numbers are kept as numbers (not converted to strings)
// - Booleans are kept as booleans (not converted to strings)
// - Strings are returned as-is
func convertValueToString(val any) any {
	switch v := val.(type) {
	case []any:
		// Format array as flow-style inline representation
		// This will be rendered as: [1, 2, 3] or ["npm", "run", "build"]
		return formatArrayInline(v)

	case map[string]any:
		// Format map as JSON
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return v
		}
		return string(jsonBytes)

	case int, int64, int32, float64, float32:
		// Keep numbers as numbers
		return v

	case bool:
		// Keep booleans as booleans
		return v

	case string:
		// Already a string
		return v

	default:
		// For any other type, convert to string
		return fmt.Sprintf("%v", v)
	}
}

// formatArrayInline formats an array as an inline YAML/JSON representation.
// Examples: [1, 2, 3] or ["npm", "run", "build"]
// Returns a special wrapper type that tells YAML to render it inline without quotes.
func formatArrayInline(arr []any) any {
	// Return as FlowStyleArray which will be handled specially
	return FlowStyleArray(arr)
}

// FlowStyleArray is a wrapper type for arrays that should be rendered in flow style.
type FlowStyleArray []any

// MarshalYAML implements yaml.Marshaler to render arrays in flow style.
func (f FlowStyleArray) MarshalYAML() (interface{}, error) {
	// Return the array as-is, YAML marshaler will use flow style
	return []any(f), nil
}

// String returns the string representation for debugging.
func (f FlowStyleArray) String() string {
	result := "["
	for i, elem := range f {
		if i > 0 {
			result += ", "
		}
		switch v := elem.(type) {
		case string:
			// Quote and escape strings
			escaped, _ := json.Marshal(v)
			result += string(escaped)
		default:
			result += fmt.Sprintf("%v", v)
		}
	}
	result += "]"
	return result
}

// convertFlowStyleArraysToSlices converts FlowStyleArray back to regular []any slices.
// This is needed because the Kubernetes API client doesn't understand custom types.
func convertFlowStyleArraysToSlices(data any) any {
	switch v := data.(type) {
	case FlowStyleArray:
		// Convert FlowStyleArray to regular slice
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
