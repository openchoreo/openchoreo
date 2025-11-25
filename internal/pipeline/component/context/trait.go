// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"

	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
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
//  1. ComponentDeployment.Spec.TraitOverrides[instanceName] (environment-specific)
//  2. TraitInstance.Parameters (instance parameters)
//  3. Schema defaults from Trait
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

	// 1. Get or build structural schema for defaulting
	var structural *apiextschema.Structural
	traitName := input.Trait.Name

	// Check cache first
	if input.SchemaCache != nil {
		if cached, ok := input.SchemaCache[traitName]; ok {
			structural = cached
		}
	}

	// Build schema if not cached
	if structural == nil {
		schemaInput := &SchemaInput{
			Types:              input.Trait.Spec.Schema.Types,
			ParametersSchema:   input.Trait.Spec.Schema.Parameters,
			EnvOverridesSchema: input.Trait.Spec.Schema.EnvOverrides,
		}
		var err error
		structural, err = BuildStructuralSchema(schemaInput)
		if err != nil {
			return nil, fmt.Errorf("failed to build trait schema: %w", err)
		}

		// Store in cache for next time
		if input.SchemaCache != nil {
			input.SchemaCache[traitName] = structural
		}
	}

	// 2. Start with instance parameters (using Parameters field from ComponentTrait)
	parameters, err := extractParameters(input.Instance.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to extract trait instance parameters: %w", err)
	}

	// 3. Merge environment overrides if present
	if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.TraitOverrides != nil {
		// TraitOverrides structure: map[instanceName]overrides (flattened)
		instanceName := input.Instance.InstanceName

		if instanceOverride, ok := input.ReleaseBinding.Spec.TraitOverrides[instanceName]; ok {
			envOverrides, err := extractParametersFromRawExtension(&instanceOverride)
			if err != nil {
				return nil, fmt.Errorf("failed to extract trait environment overrides: %w", err)
			}
			parameters = deepMerge(parameters, envOverrides)
		}
	}

	// 4. Apply schema defaults
	parameters = schema.ApplyDefaults(parameters, structural)
	ctx["parameters"] = parameters

	// 5. Add trait metadata
	traitMeta := map[string]any{
		"name":         input.Trait.Name,
		"instanceName": input.Instance.InstanceName,
	}
	ctx["trait"] = traitMeta

	// 6. Add structured metadata for resource generation
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
