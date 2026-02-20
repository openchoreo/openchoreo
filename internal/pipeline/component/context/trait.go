// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"maps"

	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"

	"github.com/openchoreo/openchoreo/internal/schema"
)

// Note: validator is initialized in component.go

// BuildTraitContext builds a CEL evaluation context for rendering trait resources.
//
// The context includes:
//   - parameters: From TraitInstance.Parameters (pruned to Trait.Schema.Parameters) - access via ${parameters.*}
//   - envOverrides: From ReleaseBinding.Spec.TraitOverrides[instanceName] (pruned to Trait.Schema.EnvOverrides) - access via ${envOverrides.*}
//   - trait: Trait metadata (name, instanceName) - access via ${trait.*}
//   - metadata: Structured naming and labeling information - access via ${metadata.*}
//
// Schema defaults are applied to both parameters and envOverrides sections.
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

	// Process parameters and envOverrides separately
	parameters, envOverrides, err := processTraitParameters(input)
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
		Parameters:   parameters,
		EnvOverrides: envOverrides,
		Metadata:     metadata,
		Trait: TraitMetadata{
			Name:         input.Trait.Name,
			InstanceName: input.Instance.InstanceName,
		},
	}

	ctx.DataPlane = extractDataPlaneData(input.DataPlane)
	ctx.Environment = extractEnvironmentData(input.Environment, input.DataPlane, input.DefaultNotificationChannel)
	ctx.Workload = input.WorkloadData
	ctx.Configurations = input.Configurations

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
// validates each against their respective schemas, and returns them as separate maps.
// Parameters come from TraitInstance.Parameters only.
// EnvOverrides come from ReleaseBinding.Spec.TraitOverrides[instanceName] only.
func processTraitParameters(input *TraitContextInput) (map[string]any, map[string]any, error) {
	traitName := input.Trait.Name

	// Build or retrieve separate schema bundles for parameters and envOverrides
	// Use cache keys with suffixes to distinguish between parameters and envOverrides schemas
	parametersBundle := getCachedSchemaBundle(input.SchemaCache, traitName+":parameters")
	envOverridesBundle := getCachedSchemaBundle(input.SchemaCache, traitName+":envOverrides")

	// If either bundle is missing, build both in one call to share types unmarshaling
	if parametersBundle == nil || envOverridesBundle == nil {
		var err error
		parametersBundle, envOverridesBundle, err = BuildStructuralSchemas(&SchemaInput{
			Types:              input.Trait.Spec.Schema.Types,
			ParametersSchema:   input.Trait.Spec.Schema.Parameters,
			EnvOverridesSchema: input.Trait.Spec.Schema.EnvOverrides,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build trait schemas: %w", err)
		}
		setCachedSchemaBundle(input.SchemaCache, traitName+":parameters", parametersBundle)
		setCachedSchemaBundle(input.SchemaCache, traitName+":envOverrides", envOverridesBundle)
	}

	// Extract trait instance parameters (for parameters section only)
	instanceParams, err := extractParameters(input.Instance.Parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract trait instance parameters: %w", err)
	}

	// Process parameters: prune to parameters schema, apply defaults, validate
	var parameters map[string]any
	if parametersBundle != nil {
		parameters = make(map[string]any, len(instanceParams))
		maps.Copy(parameters, instanceParams)
		pruning.Prune(parameters, parametersBundle.Structural, false)
		parameters = schema.ApplyDefaults(parameters, parametersBundle.Structural)
		if err := schema.ValidateWithJSONSchema(parameters, parametersBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("parameters validation failed: %w", err)
		}
	} else {
		// No parameters schema defined - discard all parameters
		parameters = make(map[string]any)
	}

	// Process envOverrides: ONLY from ReleaseBinding (no merging with trait instance)
	var envOverrides map[string]any
	instanceName := input.Instance.InstanceName
	if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.TraitOverrides != nil {
		if instanceOverride, ok := input.ReleaseBinding.Spec.TraitOverrides[instanceName]; ok {
			envOverrides, err = extractParameters(&instanceOverride)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to extract trait environment overrides: %w", err)
			}
		} else {
			envOverrides = make(map[string]any)
		}
	} else {
		envOverrides = make(map[string]any)
	}

	// Prune against schema, apply defaults, and validate
	if envOverridesBundle != nil {
		pruning.Prune(envOverrides, envOverridesBundle.Structural, false)
		envOverrides = schema.ApplyDefaults(envOverrides, envOverridesBundle.Structural)
		if err := schema.ValidateWithJSONSchema(envOverrides, envOverridesBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("envOverrides validation failed: %w", err)
		}
	} else {
		// No envOverrides schema defined - discard all envOverrides
		envOverrides = make(map[string]any)
	}

	return parameters, envOverrides, nil
}

// getCachedSchemaBundle retrieves a schema bundle from the cache
func getCachedSchemaBundle(cache map[string]*SchemaBundle, key string) *SchemaBundle {
	if cache == nil {
		return nil
	}
	return cache[key]
}

// setCachedSchemaBundle stores a schema bundle in the cache
func setCachedSchemaBundle(cache map[string]*SchemaBundle, key string, bundle *SchemaBundle) {
	if cache != nil {
		cache[key] = bundle
	}
}
