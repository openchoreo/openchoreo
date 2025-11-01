// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/crd-renderer/schema"
	"github.com/openchoreo/openchoreo/internal/crd-renderer/util"
)

// BuildComponentContext builds a CEL evaluation context for rendering component resources.
//
// The context includes:
//   - parameters: Component parameters with environment overrides and schema defaults applied
//   - workload: Workload specification (image, resources, etc.)
//   - component: Component metadata (name, etc.)
//   - environment: Environment name
//   - metadata: Additional metadata
//
// Parameter precedence (highest to lowest):
//  1. ComponentDeployment.Spec.Overrides (environment-specific)
//  2. Component.Spec.Parameters (component defaults)
//  3. Schema defaults from ComponentTypeDefinition
func BuildComponentContext(input *ComponentContextInput) (map[string]any, error) {
	if input == nil {
		return nil, fmt.Errorf("component context input is nil")
	}
	if input.Component == nil {
		return nil, fmt.Errorf("component is nil")
	}
	if input.ComponentTypeDefinition == nil {
		return nil, fmt.Errorf("component type definition is nil")
	}

	// Validate metadata is provided
	if input.Metadata.Name == "" {
		return nil, fmt.Errorf("metadata.name is required")
	}
	if input.Metadata.Namespace == "" {
		return nil, fmt.Errorf("metadata.namespace is required")
	}

	ctx := make(map[string]any)

	// 1. Build and apply schema for defaulting
	schemaInput := &SchemaInput{
		Types:              input.ComponentTypeDefinition.Spec.Schema.Types,
		ParametersSchema:   input.ComponentTypeDefinition.Spec.Schema.Parameters,
		EnvOverridesSchema: input.ComponentTypeDefinition.Spec.Schema.EnvOverrides,
	}
	structural, err := buildStructuralSchema(schemaInput)
	if err != nil {
		return nil, fmt.Errorf("failed to build component schema: %w", err)
	}

	// 2. Start with component parameters
	parameters, err := extractParameters(input.Component.Spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract component parameters: %w", err)
	}

	// 3. Merge environment overrides if present
	if input.ComponentDeployment != nil && input.ComponentDeployment.Spec.Overrides != nil {
		envOverrides, err := extractParameters(input.ComponentDeployment.Spec.Overrides)
		if err != nil {
			return nil, fmt.Errorf("failed to extract environment overrides: %w", err)
		}
		parameters = deepMerge(parameters, envOverrides)
	}

	// 4. Apply schema defaults
	parameters = schema.ApplyDefaults(parameters, structural)
	ctx["parameters"] = parameters

	// 5. Add workload information
	if input.Workload != nil {
		workloadData, err := extractWorkloadData(input.Workload)
		if err != nil {
			return nil, fmt.Errorf("failed to extract workload data: %w", err)
		}
		ctx["workload"] = workloadData
	}

	// 6. Add component metadata
	componentMeta := map[string]any{
		"name": input.Component.Name,
	}
	if input.Component.Namespace != "" {
		componentMeta["namespace"] = input.Component.Namespace
	}
	ctx["component"] = componentMeta

	// 7. Add environment
	if input.Environment != "" {
		ctx["environment"] = input.Environment
	}

	// 8. Add structured metadata for resource generation
	// This is what templates use via ${metadata.name}, ${metadata.namespace}, etc.
	metadataMap := map[string]any{
		"name":      input.Metadata.Name,
		"namespace": input.Metadata.Namespace,
	}
	if len(input.Metadata.Labels) > 0 {
		metadataMap["labels"] = input.Metadata.Labels
	}
	if len(input.Metadata.Annotations) > 0 {
		metadataMap["annotations"] = input.Metadata.Annotations
	}
	if len(input.Metadata.PodSelectors) > 0 {
		metadataMap["podSelectors"] = input.Metadata.PodSelectors
	}
	ctx["metadata"] = metadataMap

	return ctx, nil
}

// extractParameters converts a runtime.RawExtension to a map[string]any.
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

// extractWorkloadData extracts relevant workload information for the rendering context.
func extractWorkloadData(workload *v1alpha1.Workload) (map[string]any, error) {
	data := make(map[string]any)

	if workload == nil {
		return data, nil
	}

	// Add workload name
	if workload.Name != "" {
		data["name"] = workload.Name
	}

	// Extract containers information
	if len(workload.Spec.Containers) > 0 {
		containers := make(map[string]any)
		for name, container := range workload.Spec.Containers {
			containerData := map[string]any{
				"image": container.Image,
			}
			if len(container.Command) > 0 {
				containerData["command"] = container.Command
			}
			if len(container.Args) > 0 {
				containerData["args"] = container.Args
			}
			containers[name] = containerData
		}
		data["containers"] = containers
	}

	// Extract endpoints information
	// Convert struct slices to map[string]any for CEL access
	if len(workload.Spec.Endpoints) > 0 {
		endpoints, err := structToMap(workload.Spec.Endpoints)
		if err != nil {
			return nil, fmt.Errorf("failed to convert endpoints to map: %w", err)
		}
		data["endpoints"] = endpoints
	}

	// Extract connections information
	// Convert struct slices to map[string]any for CEL access
	if len(workload.Spec.Connections) > 0 {
		connections, err := structToMap(workload.Spec.Connections)
		if err != nil {
			return nil, fmt.Errorf("failed to convert connections to map: %w", err)
		}
		data["connections"] = connections
	}

	return data, nil
}

// structToMap converts any struct or slice of structs to map[string]any
// using JSON marshaling for CEL compatibility
func structToMap(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// buildStructuralSchema creates a structural schema from schema input.
func buildStructuralSchema(input *SchemaInput) (*apiextschema.Structural, error) {
	if input.Structural != nil {
		return input.Structural, nil
	}

	// Extract types from RawExtension
	var types map[string]any
	if input.Types != nil {
		if err := yaml.Unmarshal(input.Types.Raw, &types); err != nil {
			return nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	// Extract schemas from RawExtensions
	var schemas []map[string]any

	if input.ParametersSchema != nil {
		params, err := extractParameters(input.ParametersSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to extract parameters schema: %w", err)
		}
		schemas = append(schemas, params)
	}

	if input.EnvOverridesSchema != nil {
		envOverrides, err := extractParameters(input.EnvOverridesSchema)
		if err != nil {
			return nil, fmt.Errorf("failed to extract envOverrides schema: %w", err)
		}
		schemas = append(schemas, envOverrides)
	}

	def := schema.Definition{
		Types:   types,
		Schemas: schemas,
	}

	structural, err := schema.ToStructural(def)
	if err != nil {
		return nil, fmt.Errorf("failed to create structural schema: %w", err)
	}

	return structural, nil
}

// deepMerge merges two maps recursively.
// Values from 'override' take precedence over 'base'.
func deepMerge(base, override map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	if override == nil {
		return base
	}

	result := make(map[string]any)

	// Copy all base values
	for k, v := range base {
		result[k] = util.DeepCopy(v)
	}

	// Merge override values
	for k, v := range override {
		if existing, ok := result[k]; ok {
			// Both exist - try to merge if both are maps
			existingMap, existingIsMap := existing.(map[string]any)
			overrideMap, overrideIsMap := v.(map[string]any)

			if existingIsMap && overrideIsMap {
				result[k] = deepMerge(existingMap, overrideMap)
				continue
			}
		}

		// Override takes precedence
		result[k] = util.DeepCopy(v)
	}

	return result
}
