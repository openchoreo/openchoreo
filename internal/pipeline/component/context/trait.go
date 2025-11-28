// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"
	"maps"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/internal/schema"
)

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
//   - ComponentDeployment.Spec.TraitOverrides[instanceName] (environment-specific)
//   - TraitInstance.Parameters (instance parameters)
//   - Schema defaults from Trait
//
// Note: TraitOverrides is keyed by instanceName (not traitName), as instanceNames
// must be unique across all traits in a component.
func BuildTraitContext(input *TraitContextInput) (map[string]any, error) {
	if input == nil {
		return nil, fmt.Errorf("trait context input is nil")
	}
	if input.Trait == nil {
		return nil, fmt.Errorf("trait is nil")
	}
	if input.Component == nil {
		return nil, fmt.Errorf("component is nil")
	}

	// Validate metadata is provided
	if input.Metadata.Name == "" {
		return nil, fmt.Errorf("metadata.name is required")
	}
	if input.Metadata.Namespace == "" {
		return nil, fmt.Errorf("metadata.namespace is required")
	}

	ctx := make(map[string]any)

	// Process parameters and envOverrides
	finalParameters, err := processTraitParameters(input)
	if err != nil {
		return nil, err
	}
	ctx["parameters"] = finalParameters

	// 2. Add trait metadata
	traitMeta := map[string]any{
		"name":         input.Trait.Name,
		"instanceName": input.Instance.InstanceName,
	}
	ctx["trait"] = traitMeta

	// 3. Add structured metadata for resource generation
	// This is what templates and patches use via ${metadata.name}, ${metadata.namespace}, etc.
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

// processTraitParameters processes trait parameters and envOverrides separately,
// validates each against their respective schemas, merges them, and returns the final map.
func processTraitParameters(input *TraitContextInput) (map[string]any, error) {
	traitName := input.Trait.Name

	// Build or retrieve separate structural schemas for parameters and envOverrides
	// Use cache keys with suffixes to distinguish between parameters and envOverrides schemas
	parametersSchema := getCachedSchema(input.SchemaCache, traitName+":parameters")
	if parametersSchema == nil {
		var err error
		parametersSchema, err = BuildStructuralSchema(&SchemaInput{
			Types:            input.Trait.Spec.Schema.Types,
			ParametersSchema: input.Trait.Spec.Schema.Parameters,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build trait parameters schema: %w", err)
		}
		setCachedSchema(input.SchemaCache, traitName+":parameters", parametersSchema)
	}

	envOverridesSchema := getCachedSchema(input.SchemaCache, traitName+":envOverrides")
	if envOverridesSchema == nil {
		var err error
		envOverridesSchema, err = BuildStructuralSchema(&SchemaInput{
			Types:              input.Trait.Spec.Schema.Types,
			EnvOverridesSchema: input.Trait.Spec.Schema.EnvOverrides,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build trait envOverrides schema: %w", err)
		}
		setCachedSchema(input.SchemaCache, traitName+":envOverrides", envOverridesSchema)
	}

	// Process parameters: extract from trait instance, prune to parameters schema, apply defaults
	// Note: extractParameters() unmarshals into a new map, so no deep copy needed before pruning
	parameters, err := extractParameters(input.Instance.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract trait instance parameters: %w", err)
	}
	if parametersSchema != nil {
		pruning.Prune(parameters, parametersSchema, false)
	}
	parameters = schema.ApplyDefaults(parameters, parametersSchema)

	// Process envOverrides: extract and merge based on DiscardComponentEnvOverrides flag
	var envOverrides map[string]any

	if input.DiscardComponentEnvOverrides {
		// Discard trait instance envOverride values, use only ReleaseBinding
		instanceName := input.Instance.InstanceName
		if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.TraitOverrides != nil {
			if instanceOverride, ok := input.ReleaseBinding.Spec.TraitOverrides[instanceName]; ok {
				envOverrides, err = extractParametersFromRawExtension(&instanceOverride)
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
		// Merge trait instance envOverride values with ReleaseBinding
		envOverrides, err = extractParameters(input.Instance.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to extract trait instance parameters for envOverrides: %w", err)
		}

		// Merge with ReleaseBinding envOverrides
		instanceName := input.Instance.InstanceName
		if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.TraitOverrides != nil {
			if instanceOverride, ok := input.ReleaseBinding.Spec.TraitOverrides[instanceName]; ok {
				rbEnvOverrides, err := extractParametersFromRawExtension(&instanceOverride)
				if err != nil {
					return nil, fmt.Errorf("failed to extract trait environment overrides: %w", err)
				}
				envOverrides = deepMerge(envOverrides, rbEnvOverrides)
			}
		}
	}

	// Prune merged result against schema
	if envOverridesSchema != nil {
		pruning.Prune(envOverrides, envOverridesSchema, false)
	}

	// Apply defaults
	envOverrides = schema.ApplyDefaults(envOverrides, envOverridesSchema)

	// Top-level merge: combine parameters and envOverrides
	// Safe because parameter and envOverride schemas don't overlap
	finalParameters := make(map[string]any)
	maps.Copy(finalParameters, parameters)
	maps.Copy(finalParameters, envOverrides)

	return finalParameters, nil
}

// extractParametersFromRawExtension converts a runtime.RawExtension to a map[string]any.
// This is similar to extractParameters but operates on a runtime.RawExtension directly.
func extractParametersFromRawExtension(raw *runtime.RawExtension) (map[string]any, error) {
	if raw == nil || raw.Raw == nil {
		return make(map[string]any), nil
	}

	var params map[string]any
	if err := json.Unmarshal(raw.Raw, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	return params, nil
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
