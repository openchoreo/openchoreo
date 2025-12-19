// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/go-playground/validator/v10"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// BuildComponentContext builds a CEL evaluation context for rendering component resources.
//
// The context includes:
//   - parameters: From Component.Spec.Parameters (pruned to schema.parameters) - access via ${parameters.*}
//   - envOverrides: From ReleaseBinding.Spec.ComponentTypeEnvOverrides (pruned to schema.envOverrides) - access via ${envOverrides.*}
//   - workload: Workload specification (image, resources, etc.) - access via ${workload.*}
//   - metadata: Structured naming and labeling information - access via ${metadata.*}
//   - dataplane: Data plane configuration - access via ${dataplane.*}
//   - configurations: Extracted configuration items from workload - access via ${configurations.*}
//
// Schema defaults are applied to both parameters and envOverrides sections.
func BuildComponentContext(input *ComponentContextInput) (*ComponentContext, error) {
	if err := validate.Struct(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	ctx := &ComponentContext{}

	// Process parameters and envOverrides separately
	parameters, envOverrides, err := processComponentParameters(input)
	if err != nil {
		return nil, err
	}
	ctx.Parameters = parameters
	ctx.EnvOverrides = envOverrides

	workload := input.Workload
	if input.Workload != nil && input.ReleaseBinding != nil && input.ReleaseBinding.Spec.WorkloadOverrides != nil {
		workload = MergeWorkloadOverrides(input.Workload, input.ReleaseBinding.Spec.WorkloadOverrides)
	}

	ctx.Workload = extractWorkloadData(workload)
	ctx.Configurations = extractConfigurationsFromWorkload(input.SecretReferences, workload)

	// Ensure metadata maps are always initialized
	ctx.Metadata = input.Metadata
	if ctx.Metadata.Labels == nil {
		ctx.Metadata.Labels = make(map[string]string)
	}
	if ctx.Metadata.Annotations == nil {
		ctx.Metadata.Annotations = make(map[string]string)
	}
	if ctx.Metadata.PodSelectors == nil {
		ctx.Metadata.PodSelectors = make(map[string]string)
	}

	ctx.DataPlane = DataPlaneData{
		PublicVirtualHost: input.DataPlane.Spec.Gateway.PublicVirtualHost,
	}
	if input.DataPlane.Spec.SecretStoreRef != nil {
		ctx.DataPlane.SecretStore = input.DataPlane.Spec.SecretStoreRef.Name
	}

	return ctx, nil
}

// processComponentParameters processes component parameters and envOverrides separately,
// validates each against their respective schemas, and returns them as separate maps.
// Parameters come from Component.Spec.Parameters only.
// EnvOverrides come from ReleaseBinding.Spec.ComponentTypeEnvOverrides only.
func processComponentParameters(input *ComponentContextInput) (map[string]any, map[string]any, error) {
	// Build both structural schemas in one call to share types unmarshaling
	parametersSchema, envOverridesSchema, err := BuildStructuralSchemas(&SchemaInput{
		Types:              input.ComponentType.Spec.Schema.Types,
		ParametersSchema:   input.ComponentType.Spec.Schema.Parameters,
		EnvOverridesSchema: input.ComponentType.Spec.Schema.EnvOverrides,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build schemas: %w", err)
	}

	// Extract component parameters (for parameters section only)
	componentParams, err := extractParameters(input.Component.Spec.Parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract component parameters: %w", err)
	}

	// Process parameters: prune to parameters schema, apply defaults
	var parameters map[string]any
	if parametersSchema != nil {
		parameters = make(map[string]any, len(componentParams))
		maps.Copy(parameters, componentParams)
		pruning.Prune(parameters, parametersSchema, false)
		parameters = schema.ApplyDefaults(parameters, parametersSchema)
	} else {
		// No parameters schema defined - discard all parameters
		parameters = make(map[string]any)
	}

	// Process envOverrides: ONLY from ReleaseBinding (no merging with Component)
	var envOverrides map[string]any
	if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.ComponentTypeEnvOverrides != nil {
		envOverrides, err = extractParameters(input.ReleaseBinding.Spec.ComponentTypeEnvOverrides)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract environment overrides: %w", err)
		}
	} else {
		envOverrides = make(map[string]any)
	}

	// Prune against schema and apply defaults
	if envOverridesSchema != nil {
		pruning.Prune(envOverrides, envOverridesSchema, false)
		envOverrides = schema.ApplyDefaults(envOverrides, envOverridesSchema)
	} else {
		// No envOverrides schema defined - discard all envOverrides
		envOverrides = make(map[string]any)
	}

	return parameters, envOverrides, nil
}

// ToMap converts the ComponentContext to map[string]any for CEL evaluation.
// All fields including configurations are converted to nested maps via JSON marshaling.
// This allows consistent CEL access without requiring ext.NativeTypes() registration.
func (c *ComponentContext) ToMap() map[string]any {
	result, err := structToMap(c)
	if err != nil {
		// This should never happen with well-formed ComponentContext
		return make(map[string]any)
	}
	return result
}

// extractParameters converts a runtime.RawExtension to a map for CEL evaluation.
//
// runtime.RawExtension is Kubernetes' way of storing arbitrary JSON in a typed field.
// This function unmarshals the raw JSON bytes into a map that can be:
//   - Merged with other parameter sources
//   - Used as CEL evaluation context
//   - Validated against schemas
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
func extractWorkloadData(workload *v1alpha1.Workload) WorkloadData {
	data := WorkloadData{
		Containers: make(map[string]ContainerData),
	}

	if workload == nil {
		return data
	}

	for name, container := range workload.Spec.Containers {
		data.Containers[name] = ContainerData{
			Image:   container.Image,
			Command: container.Command,
			Args:    container.Args,
		}
	}

	return data
}

// structToMap converts typed Go structs to map[string]any for CEL evaluation.
//
// CEL expressions can only access maps and primitive types, not arbitrary Go structs.
// This function uses JSON marshaling as a conversion mechanism:
func structToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// BuildStructuralSchemas builds separate structural schemas for parameters and envOverrides
// while unmarshaling types only once.
//
// Returns (parametersSchema, envOverridesSchema, error). Either schema can be nil if not provided.
func BuildStructuralSchemas(input *SchemaInput) (*apiextschema.Structural, *apiextschema.Structural, error) {
	// Extract types from RawExtension once
	var types map[string]any
	if input.Types != nil {
		if err := yaml.Unmarshal(input.Types.Raw, &types); err != nil {
			return nil, nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	// Build parameters schema
	var parametersSchema *apiextschema.Structural
	if input.ParametersSchema != nil {
		params, err := extractParameters(input.ParametersSchema)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract parameters schema: %w", err)
		}

		def := schema.Definition{
			Types:   types,
			Schemas: []map[string]any{params},
		}

		parametersSchema, err = schema.ToStructural(def)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create parameters structural schema: %w", err)
		}
	}

	// Build envOverrides schema
	var envOverridesSchema *apiextschema.Structural
	if input.EnvOverridesSchema != nil {
		envOverrides, err := extractParameters(input.EnvOverridesSchema)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract envOverrides schema: %w", err)
		}

		def := schema.Definition{
			Types:   types,
			Schemas: []map[string]any{envOverrides},
		}

		envOverridesSchema, err = schema.ToStructural(def)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create envOverrides structural schema: %w", err)
		}
	}

	return parametersSchema, envOverridesSchema, nil
}
