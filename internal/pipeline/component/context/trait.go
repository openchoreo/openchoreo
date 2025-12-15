// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"maps"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"

	"github.com/openchoreo/openchoreo/internal/schema"
)

// Note: validator is initialized in component.go

// BuildTraitContext builds a CEL evaluation context for rendering trait resources.
//
// The context includes:
//   - parameters: Trait instance parameters with environment overrides and schema defaults applied
//   - trait: Trait metadata (name, instanceName)
//   - component: Component reference (name, etc.)
//   - environment: Environment name
//   - metadata: Additional metadata
//
// Parameter precedence (highest to lowest):
//   - ReleaseBinding.Spec.TraitOverrides[instanceName] (environment-specific)
//   - TraitInstance.Parameters (instance parameters)
//   - Schema defaults from Trait
//
// Note: TraitOverrides is keyed by instanceName (not traitName), as instanceNames
// must be unique across all traits in a component.
func BuildTraitContext(input *TraitContextInput) (*TraitContext, error) {
	if err := validate.Struct(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Additional validation for InstanceName (can't use struct tags on API types)
	if input.Instance.InstanceName == "" {
		return nil, fmt.Errorf("trait instance name is required")
	}

	// Process parameters and envOverrides
	finalParameters, err := processTraitParameters(input)
	if err != nil {
		return nil, err
	}

	// Ensure metadata maps are always initialized
	metadata := input.Metadata
	if metadata.Labels == nil {
		metadata.Labels = make(map[string]string)
	}
	if metadata.Annotations == nil {
		metadata.Annotations = make(map[string]string)
	}
	if metadata.PodSelectors == nil {
		metadata.PodSelectors = make(map[string]string)
	}

	ctx := &TraitContext{
		Parameters: finalParameters,
		Metadata:   metadata,
		Trait: TraitMetadata{
			Name:         input.Trait.Name,
			InstanceName: input.Instance.InstanceName,
		},
	}
	return ctx, nil
}

// ToMap converts the TraitContext to map[string]any for CEL evaluation.
func (t *TraitContext) ToMap() map[string]any {
	result, err := structToMap(t)
	if err != nil {
		// This should never happen with well-formed TraitContext
		return make(map[string]any)
	}
	return result
}

// processTraitParameters processes trait parameters and envOverrides separately,
// validates each against their respective schemas, merges them, and returns the final map.
func processTraitParameters(input *TraitContextInput) (map[string]any, error) {
	traitName := input.Trait.Name

	// Build or retrieve separate structural schemas for parameters and envOverrides
	// Use cache keys with suffixes to distinguish between parameters and envOverrides schemas
	parametersSchema := getCachedSchema(input.SchemaCache, traitName+":parameters")
	envOverridesSchema := getCachedSchema(input.SchemaCache, traitName+":envOverrides")

	// If either schema is missing, build both in one call to share types unmarshaling
	if parametersSchema == nil || envOverridesSchema == nil {
		var err error
		parametersSchema, envOverridesSchema, err = BuildStructuralSchemas(&SchemaInput{
			Types:              input.Trait.Spec.Schema.Types,
			ParametersSchema:   input.Trait.Spec.Schema.Parameters,
			EnvOverridesSchema: input.Trait.Spec.Schema.EnvOverrides,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build trait schemas: %w", err)
		}
		setCachedSchema(input.SchemaCache, traitName+":parameters", parametersSchema)
		setCachedSchema(input.SchemaCache, traitName+":envOverrides", envOverridesSchema)
	}

	// Extract trait instance parameters once (used for both parameters and envOverrides sections)
	instanceParams, err := extractParameters(input.Instance.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract trait instance parameters: %w", err)
	}

	// Process parameters: prune to parameters schema, apply defaults
	var parameters map[string]any
	if parametersSchema != nil {
		// Clone the map to avoid modifying the original (needed for envOverrides processing)
		parameters = make(map[string]any, len(instanceParams))
		maps.Copy(parameters, instanceParams)
		pruning.Prune(parameters, parametersSchema, false)
		parameters = schema.ApplyDefaults(parameters, parametersSchema)
	} else {
		// No parameters schema defined - discard all parameters
		parameters = make(map[string]any)
	}

	// Process envOverrides: extract and merge based on DiscardComponentEnvOverrides flag
	var envOverrides map[string]any

	if input.DiscardComponentEnvOverrides {
		// Discard trait instance envOverride values, use only ReleaseBinding
		instanceName := input.Instance.InstanceName
		if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.TraitOverrides != nil {
			if instanceOverride, ok := input.ReleaseBinding.Spec.TraitOverrides[instanceName]; ok {
				envOverrides, err = extractParameters(&instanceOverride)
				if err != nil {
					return nil, fmt.Errorf("failed to extract trait environment overrides: %w", err)
				}
			} else {
				envOverrides = make(map[string]any)
			}
		} else {
			envOverrides = make(map[string]any)
		}
	} else {
		// Use extracted instance parameters as starting point for envOverrides
		envOverrides = instanceParams

		// Merge with ReleaseBinding envOverrides
		instanceName := input.Instance.InstanceName
		if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.TraitOverrides != nil {
			if instanceOverride, ok := input.ReleaseBinding.Spec.TraitOverrides[instanceName]; ok {
				rbEnvOverrides, err := extractParameters(&instanceOverride)
				if err != nil {
					return nil, fmt.Errorf("failed to extract trait environment overrides: %w", err)
				}
				envOverrides = deepMerge(envOverrides, rbEnvOverrides)
			}
		}
	}

	// Prune merged result against schema and apply defaults
	if envOverridesSchema != nil {
		pruning.Prune(envOverrides, envOverridesSchema, false)
		envOverrides = schema.ApplyDefaults(envOverrides, envOverridesSchema)
	} else {
		// No envOverrides schema defined - discard all envOverrides
		envOverrides = make(map[string]any)
	}

	// Top-level merge: combine parameters and envOverrides
	// Safe because parameter and envOverride schemas don't overlap
	finalParameters := make(map[string]any)
	maps.Copy(finalParameters, parameters)
	maps.Copy(finalParameters, envOverrides)

	return finalParameters, nil
}

// getCachedSchema retrieves a structural schema from the cache
func getCachedSchema(cache map[string]*apiextschema.Structural, key string) *apiextschema.Structural {
	if cache == nil {
		return nil
	}
	return cache[key]
}

// setCachedSchema stores a structural schema in the cache
func setCachedSchema(cache map[string]*apiextschema.Structural, key string, schema *apiextschema.Structural) {
	if cache != nil {
		cache[key] = schema
	}
}
