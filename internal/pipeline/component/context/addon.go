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

// BuildAddonContext builds a CEL evaluation context for rendering addon resources.
//
// The context includes:
//   - parameters: Addon instance parameters with environment overrides and schema defaults applied
//   - addon: Addon metadata (name, instanceName)
//   - component: Component reference (name, etc.)
//   - environment: Environment name
//   - metadata: Additional metadata
//
// Parameter precedence (highest to lowest):
//  1. ComponentDeployment.Spec.AddonOverrides[instanceName] (environment-specific)
//  2. AddonInstance.Parameters (instance parameters)
//  3. Schema defaults from Addon
//
// Note: AddonOverrides is keyed by instanceName (not addonName), as instanceNames
// must be unique across all addons in a component.
func BuildAddonContext(input *AddonContextInput) (map[string]any, error) {
	if input == nil {
		return nil, fmt.Errorf("addon context input is nil")
	}
	if input.Addon == nil {
		return nil, fmt.Errorf("addon is nil")
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
	addonName := input.Addon.Name

	// Check cache first
	if input.SchemaCache != nil {
		if cached, ok := input.SchemaCache[addonName]; ok {
			structural = cached
		}
	}

	// Build schema if not cached
	if structural == nil {
		schemaInput := &SchemaInput{
			Types:              input.Addon.Spec.Schema.Types,
			ParametersSchema:   input.Addon.Spec.Schema.Parameters,
			EnvOverridesSchema: input.Addon.Spec.Schema.EnvOverrides,
		}
		var err error
		structural, err = BuildStructuralSchema(schemaInput)
		if err != nil {
			return nil, fmt.Errorf("failed to build addon schema: %w", err)
		}

		// Store in cache for next time
		if input.SchemaCache != nil {
			input.SchemaCache[addonName] = structural
		}
	}

	// 2. Start with instance parameters (using Config field from ComponentAddon)
	parameters, err := extractParameters(input.Instance.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to extract addon instance parameters: %w", err)
	}

	// 3. Merge environment overrides if present
	if input.ComponentDeployment != nil && input.ComponentDeployment.Spec.AddonOverrides != nil {
		// AddonOverrides structure: map[instanceName]overrides (flattened)
		instanceName := input.Instance.InstanceName

		if instanceOverride, ok := input.ComponentDeployment.Spec.AddonOverrides[instanceName]; ok {
			envOverrides, err := extractParametersFromRawExtension(&instanceOverride)
			if err != nil {
				return nil, fmt.Errorf("failed to extract addon environment overrides: %w", err)
			}
			parameters = deepMerge(parameters, envOverrides)
		}
	}

	// 4. Apply schema defaults
	parameters = schema.ApplyDefaults(parameters, structural)
	ctx["parameters"] = parameters

	// 5. Add addon metadata
	addonMeta := map[string]any{
		"name":         input.Addon.Name,
		"instanceName": input.Instance.InstanceName,
	}
	ctx["addon"] = addonMeta

	// 6. Add component reference
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
