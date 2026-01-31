// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"
	"maps"

	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/template"
)

// EmbeddedTraitContextInput contains all inputs needed to build a context for an embedded trait.
type EmbeddedTraitContextInput struct {
	// Trait is the trait definition.
	Trait *v1alpha1.Trait `validate:"required"`

	// Instance contains the synthetic trait instance with resolved parameters.
	Instance v1alpha1.ComponentTrait `validate:"required"`

	// ResolvedEnvOverrides contains the CEL-resolved envOverride defaults from the embedded binding.
	// These serve as defaults that can be overridden by ReleaseBinding.traitOverrides[instanceName].
	ResolvedEnvOverrides *runtime.RawExtension

	// Component is the component this trait is being applied to.
	Component *v1alpha1.Component `validate:"required"`

	// ReleaseBinding contains release reference and environment-specific overrides.
	ReleaseBinding *v1alpha1.ReleaseBinding

	// WorkloadData is the pre-computed workload data.
	WorkloadData WorkloadData

	// Configurations is the pre-computed configurations map from workload.
	Configurations ContainerConfigurationsMap

	// Metadata provides structured naming and labeling information.
	Metadata MetadataContext `validate:"required"`

	// SchemaCache is an optional cache for schema bundles.
	SchemaCache map[string]*SchemaBundle

	// DataPlane contains the data plane configuration.
	DataPlane *v1alpha1.DataPlane `validate:"required"`
}

// ResolveEmbeddedTraitBindings resolves CEL expressions in an embedded trait's parameter
// and envOverride bindings against the component context.
//
// Values in the embedded trait bindings can be:
//   - Concrete values (locked by PE): passed through as-is
//   - CEL expressions like "${parameters.storage.mountPath}": evaluated against the component context
//
// Returns the resolved parameters and envOverrides as RawExtension.
func ResolveEmbeddedTraitBindings(
	engine *template.Engine,
	embeddedTrait v1alpha1.ComponentTypeTrait,
	componentContextMap map[string]any,
) (resolvedParams *runtime.RawExtension, resolvedEnvOverrides *runtime.RawExtension, err error) {
	resolvedParams, err = resolveBindings(engine, embeddedTrait.Parameters, componentContextMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve embedded trait %s/%s parameters: %w",
			embeddedTrait.Name, embeddedTrait.InstanceName, err)
	}

	resolvedEnvOverrides, err = resolveBindings(engine, embeddedTrait.EnvOverrides, componentContextMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve embedded trait %s/%s envOverrides: %w",
			embeddedTrait.Name, embeddedTrait.InstanceName, err)
	}

	return resolvedParams, resolvedEnvOverrides, nil
}

// resolveBindings takes a RawExtension containing mixed concrete values and CEL expressions,
// evaluates all CEL expressions against the given context, and returns a new RawExtension
// with all values resolved to concrete types.
func resolveBindings(
	engine *template.Engine,
	raw *runtime.RawExtension,
	contextMap map[string]any,
) (*runtime.RawExtension, error) {
	if raw == nil || raw.Raw == nil {
		return nil, nil
	}

	var data any
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bindings: %w", err)
	}

	resolved, err := engine.Render(data, contextMap)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate CEL bindings: %w", err)
	}

	resolvedBytes, err := json.Marshal(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resolved bindings: %w", err)
	}

	return &runtime.RawExtension{Raw: resolvedBytes}, nil
}

// BuildEmbeddedTraitContext builds a CEL evaluation context for rendering an embedded trait's resources.
//
// Unlike BuildTraitContext, this function:
//   - Uses pre-resolved parameters (already evaluated from CEL bindings)
//   - Merges resolved envOverride defaults with ReleaseBinding traitOverrides (ReleaseBinding wins)
func BuildEmbeddedTraitContext(input *EmbeddedTraitContextInput) (*TraitContext, error) {
	if err := validate.Struct(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if input.Instance.InstanceName == "" {
		return nil, fmt.Errorf("trait instance name is required")
	}

	parameters, envOverrides, err := processEmbeddedTraitParameters(input)
	if err != nil {
		return nil, err
	}

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
	ctx.Workload = input.WorkloadData
	ctx.Configurations = input.Configurations

	return ctx, nil
}

// processEmbeddedTraitParameters processes parameters and envOverrides for an embedded trait.
//
// Parameters: Come from the resolved bindings (already concrete values from CEL evaluation).
// EnvOverrides: Start with resolved binding defaults, then merge with ReleaseBinding.traitOverrides
// (ReleaseBinding values take precedence).
func processEmbeddedTraitParameters(input *EmbeddedTraitContextInput) (map[string]any, map[string]any, error) {
	traitName := input.Trait.Name

	// Build or retrieve schema bundles
	parametersBundle := getCachedSchemaBundle(input.SchemaCache, traitName+":parameters")
	envOverridesBundle := getCachedSchemaBundle(input.SchemaCache, traitName+":envOverrides")

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

	// Extract resolved parameters (already concrete from CEL evaluation)
	instanceParams, err := extractParameters(input.Instance.Parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract resolved trait parameters: %w", err)
	}

	// Process parameters: prune, apply defaults, validate
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
		parameters = make(map[string]any)
	}

	// Process envOverrides: merge resolved defaults with ReleaseBinding overrides
	resolvedDefaults, err := extractParameters(input.ResolvedEnvOverrides)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract resolved trait envOverrides: %w", err)
	}

	// Start with resolved defaults from embedded binding
	envOverrides := make(map[string]any, len(resolvedDefaults))
	maps.Copy(envOverrides, resolvedDefaults)

	// Merge ReleaseBinding traitOverrides on top (ReleaseBinding wins)
	instanceName := input.Instance.InstanceName
	if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.TraitOverrides != nil {
		if instanceOverride, ok := input.ReleaseBinding.Spec.TraitOverrides[instanceName]; ok {
			releaseOverrides, err := extractParameters(&instanceOverride)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to extract trait environment overrides: %w", err)
			}
			maps.Copy(envOverrides, releaseOverrides)
		}
	}

	// Prune against schema, apply defaults, and validate
	if envOverridesBundle != nil {
		pruning.Prune(envOverrides, envOverridesBundle.Structural, false)
		envOverrides = schema.ApplyDefaults(envOverrides, envOverridesBundle.Structural)
		if err := schema.ValidateWithJSONSchema(envOverrides, envOverridesBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("envOverrides validation failed: %w", err)
		}
	} else {
		envOverrides = make(map[string]any)
	}

	return parameters, envOverrides, nil
}
