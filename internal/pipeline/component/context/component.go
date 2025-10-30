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
	"github.com/openchoreo/openchoreo/internal/clone"
	"github.com/openchoreo/openchoreo/internal/schema"
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
	structural, err := BuildStructuralSchema(schemaInput)
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

	// 6. Extract configurations (env and file from all containers)
	if input.Workload != nil {
		workloadData, err := extractWorkloadData(input.Workload)
		if err != nil {
			return nil, fmt.Errorf("failed to extract workload data: %w", err)
		}
		ctx["workload"] = workloadData
	}

	// 7. Extract configurations from workload
	if input.Workload != nil {
		configurations := extractConfigurationsFromWorkload(input.Workload)

		// 8. Apply configuration overrides from ComponentDeployment if present
		if input.ComponentDeployment != nil && input.ComponentDeployment.Spec.ConfigurationOverrides != nil {
			configurations = applyConfigurationOverrides(configurations, input.ComponentDeployment.Spec.ConfigurationOverrides)
		}

		if configurations != nil {
			ctx["configurations"] = configurations
		}
	}

	// 9. Add component metadata
	componentMeta := map[string]any{
		"name": input.Component.Name,
	}
	if input.Component.Namespace != "" {
		componentMeta["namespace"] = input.Component.Namespace
	}
	ctx["component"] = componentMeta

	// 10. Add environment
	if input.Environment != "" {
		ctx["environment"] = input.Environment
	}

	// 11. Add structured metadata for resource generation
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

// extractParameters converts a runtime.RawExtension to a map for CEL evaluation.
//
// runtime.RawExtension is Kubernetes' way of storing arbitrary JSON in a typed field.
// This function unmarshals the raw JSON bytes into a map that can be:
//  1. Merged with other parameter sources
//  2. Used as CEL evaluation context
//  3. Validated against schemas
//
// Returns an empty map if the extension is nil or empty, rather than an error,
// since absent parameters are valid (defaults will be applied by schema).
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

// structToMap converts typed Go structs to map[string]any for CEL evaluation.
//
// CEL expressions can only access maps and primitive types, not arbitrary Go structs.
// This function uses JSON marshaling as a universal conversion mechanism:
//  1. Marshal the struct to JSON (respects json tags)
//  2. Unmarshal to map[string]any/[]any
//  3. Result is accessible to CEL as ${workload.endpoints[0].path}
//
// This is used to expose Workload.Spec.Endpoints and Workload.Spec.Connections
// to component templates.
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

// extractConfigurationsFromWorkload extracts env and file configurations from workload containers
// and separates them into configs vs secrets based on valueFrom usage.
func extractConfigurationsFromWorkload(workload *v1alpha1.Workload) map[string]any {

	configs := map[string][]any{
		"envs":  make([]any, 0),
		"files": make([]any, 0),
	}
	secrets := map[string][]any{
		"envs":  make([]any, 0),
		"files": make([]any, 0),
	}

	// Process all containers (only if workload exists and has containers)
	if workload != nil && len(workload.Spec.Containers) > 0 {
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
					// TODO: Handle environment variables as secrets
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
					// TODO: Handle file as secrets
				}
			}
		}
	}

	result := make(map[string]any)

	configsResult := make(map[string]any)
	configsResult["envs"] = configs["envs"]
	configsResult["files"] = configs["files"]
	result["configs"] = configsResult

	secretsResult := make(map[string]any)
	secretsResult["envs"] = secrets["envs"]
	secretsResult["files"] = secrets["files"]
	result["secrets"] = secretsResult

	return result
}

// applyConfigurationOverrides merges configuration overrides from ComponentDeployment into existing configurations.
// If a configuration with the same name exists, it updates the value. If it's new, it adds it.
func applyConfigurationOverrides(baseConfigurations map[string]any, overrides *v1alpha1.EnvConfigurationOverrides) map[string]any {
	// Create maps for easy lookup by name
	configEnvMap := make(map[string]map[string]any)
	configFileMap := make(map[string]map[string]any)
	secretEnvMap := make(map[string]map[string]any)
	secretFileMap := make(map[string]map[string]any)

	// Populate maps from base configurations
	configs := baseConfigurations["configs"].(map[string]any)
	secrets := baseConfigurations["secrets"].(map[string]any)

	for _, envItem := range configs["envs"].([]any) {
		if envMap, ok := envItem.(map[string]any); ok {
			if name, ok := envMap["name"].(string); ok {
				configEnvMap[name] = envMap
			}
		}
	}

	for _, fileItem := range configs["files"].([]any) {
		if fileMap, ok := fileItem.(map[string]any); ok {
			if name, ok := fileMap["name"].(string); ok {
				configFileMap[name] = fileMap
			}
		}
	}

	for _, envItem := range secrets["envs"].([]any) {
		if envMap, ok := envItem.(map[string]any); ok {
			if name, ok := envMap["name"].(string); ok {
				secretEnvMap[name] = envMap
			}
		}
	}

	for _, fileItem := range secrets["files"].([]any) {
		if fileMap, ok := fileItem.(map[string]any); ok {
			if name, ok := fileMap["name"].(string); ok {
				secretFileMap[name] = fileMap
			}
		}
	}

	// Process environment variable overrides
	for _, envOverride := range overrides.Env {
		envMap := map[string]any{
			"name": envOverride.Key,
		}

		if envOverride.Value != "" {
			// Direct value - goes to configs
			envMap["value"] = envOverride.Value
		} else if envOverride.ValueFrom != nil {
			// TODO: Handle environment variables as secrets
		}
		configEnvMap[envOverride.Key] = envMap
	}

	// Process file overrides
	for _, fileOverride := range overrides.Files {
		fileMap := map[string]any{
			"name":      fileOverride.Key,
			"mountPath": fileOverride.MountPath,
		}

		if fileOverride.Value != "" {
			fileMap["value"] = fileOverride.Value
		} else if fileOverride.ValueFrom != nil {
			// TODO: Handle file as secrets
		}
		configFileMap[fileOverride.Key] = fileMap
	}

	// Convert maps back to arrays
	configEnvs := make([]any, 0, len(configEnvMap))
	for _, envMap := range configEnvMap {
		configEnvs = append(configEnvs, envMap)
	}

	configFiles := make([]any, 0, len(configFileMap))
	for _, fileMap := range configFileMap {
		configFiles = append(configFiles, fileMap)
	}

	secretEnvs := make([]any, 0, len(secretEnvMap))
	for _, envMap := range secretEnvMap {
		secretEnvs = append(secretEnvs, envMap)
	}

	secretFiles := make([]any, 0, len(secretFileMap))
	for _, fileMap := range secretFileMap {
		secretFiles = append(secretFiles, fileMap)
	}

	// Update base configurations
	configs["envs"] = configEnvs
	configs["files"] = configFiles
	secrets["envs"] = secretEnvs
	secrets["files"] = secretFiles

	baseConfigurations["configs"] = configs
	baseConfigurations["secrets"] = secrets

	return baseConfigurations
}

// BuildStructuralSchema converts schema input into a Kubernetes structural schema.
// This function is exported to allow per-render caching of schemas for reused addons.
func BuildStructuralSchema(input *SchemaInput) (*apiextschema.Structural, error) {
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

// deepMerge recursively merges two parameter maps with override precedence.
//
// This function implements the parameter precedence model for ComponentDeployments:
// values from 'override' take precedence over 'base'.
//
// Merge behavior:
//   - Nested objects: recursively merged to preserve partial overrides
//   - Other values: override completely replaces base
//   - All values are deep copied to prevent shared references
//
// This allows environment-specific ComponentDeployment.Spec.Overrides to selectively
// override parts of Component.Spec.Parameters without replacing the entire structure.
//
// Example:
//
//	base:     {db: {host: "localhost", port: 5432}, replicas: 1}
//	override: {db: {host: "prod.db.example.com"}, replicas: 3}
//	result:   {db: {host: "prod.db.example.com", port: 5432}, replicas: 3}
func deepMerge(base, override map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	if override == nil {
		return base
	}

	// Copy all base values
	result := clone.DeepCopyMap(base)

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
		result[k] = clone.DeepCopy(v)
	}

	return result
}
