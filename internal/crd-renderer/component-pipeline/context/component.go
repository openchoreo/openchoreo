// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"

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
		workloadData := extractWorkloadData(input.Workload)
		ctx["workload"] = workloadData
	}

	// 6. Extract configurations (env and file from all containers)
	if input.Workload != nil {
		configurations := extractConfigurationsFromWorkload(input.Workload)
		if configurations != nil {
			ctx["configurations"] = configurations
		}
	}

	// 7. Add component metadata
	componentMeta := map[string]any{
		"name": input.Component.Name,
	}
	if input.Component.Namespace != "" {
		componentMeta["namespace"] = input.Component.Namespace
	}
	ctx["component"] = componentMeta

	// 8. Add environment
	if input.Environment != "" {
		ctx["environment"] = input.Environment
	}

	// 9. Add structured metadata for resource generation
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
func extractWorkloadData(workload *v1alpha1.Workload) map[string]any {
	data := make(map[string]any)

	if workload == nil {
		return data
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
	if len(workload.Spec.Endpoints) > 0 {
		data["endpoints"] = workload.Spec.Endpoints
	}

	// Extract connections information
	if len(workload.Spec.Connections) > 0 {
		data["connections"] = workload.Spec.Connections
	}

	return data
}

// extractConfigurationsFromWorkload extracts env and file configurations from workload containers
// and separates them into configs vs secrets based on valueFrom usage.
func extractConfigurationsFromWorkload(workload *v1alpha1.Workload) map[string]any {
	if workload == nil || len(workload.Spec.Containers) == 0 {
		return nil
	}

	configs := map[string][]map[string]any{
		"envs":  []map[string]any{},
		"files": []map[string]any{},
	}
	secrets := map[string][]map[string]any{
		"envs":  []map[string]any{},
		"files": []map[string]any{},
	}

	// Process all containers
	for _, container := range workload.Spec.Containers {
		// Process environment variables
		for _, env := range container.Env {
			envMap := map[string]any{
				"name": env.Key,
			}

			if env.Value != "" {
				// Direct value - goes to configs
				envMap["value"] = env.Value
				configs["envs"] = append(configs["envs"], envMap)
			} else if env.ValueFrom != nil {
				// Reference to external source - goes to secrets
				if env.ValueFrom.SecretRef != nil {
					envMap["remoteRef"] = map[string]any{
						"key":      fmt.Sprintf("secret/data/%s", env.ValueFrom.SecretRef.Name),
						"property": env.ValueFrom.SecretRef.Key,
					}
					secrets["envs"] = append(secrets["envs"], envMap)
				} else if env.ValueFrom.ConfigurationGroupRef != nil {
					// ConfigurationGroup references also go to configs
					envMap["remoteRef"] = map[string]any{
						"key":      fmt.Sprintf("configmap/data/%s", env.ValueFrom.ConfigurationGroupRef.Name),
						"property": env.ValueFrom.ConfigurationGroupRef.Key,
					}
					configs["envs"] = append(configs["envs"], envMap)
				}
			}
		}

		// Process file configurations
		for _, file := range container.File {
			fileMap := map[string]any{
				"name":      file.Key,
				"mountPath": file.MountPath,
			}

			if file.Value != "" {
				// Direct content - goes to configs
				fileMap["value"] = file.Value
				configs["files"] = append(configs["files"], fileMap)
			} else if file.ValueFrom != nil {
				// Reference to external source - goes to secrets
				if file.ValueFrom.SecretRef != nil {
					fileMap["remoteRef"] = map[string]any{
						"key":      fmt.Sprintf("secret/data/%s", file.ValueFrom.SecretRef.Name),
						"property": file.ValueFrom.SecretRef.Key,
					}
					secrets["files"] = append(secrets["files"], fileMap)
				} else if file.ValueFrom.ConfigurationGroupRef != nil {
					// ConfigurationGroup references also go to configs
					fileMap["remoteRef"] = map[string]any{
						"key":      fmt.Sprintf("configmap/data/%s", file.ValueFrom.ConfigurationGroupRef.Name),
						"property": file.ValueFrom.ConfigurationGroupRef.Key,
					}
					configs["files"] = append(configs["files"], fileMap)
				}
			}
		}
	}

	// Always include sections with empty arrays if no data
	result := make(map[string]any)

	configsResult := make(map[string]any)
	configsResult["envs"] = configs["envs"]   // Always include, even if empty
	configsResult["files"] = configs["files"] // Always include, even if empty
	result["configs"] = configsResult

	secretsResult := make(map[string]any)
	secretsResult["envs"] = secrets["envs"]   // Always include, even if empty
	secretsResult["files"] = secrets["files"] // Always include, even if empty
	result["secrets"] = secretsResult

	return result
}

// buildStructuralSchema creates a structural schema from schema input.
func buildStructuralSchema(input *SchemaInput) (*apiextschema.Structural, error) {
	if input.Structural != nil {
		return input.Structural, nil
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
