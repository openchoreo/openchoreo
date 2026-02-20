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

	// InstanceName is the unique instance name for this embedded trait.
	InstanceName string `validate:"required"`

	// ResolvedParameters contains the CEL-resolved parameters from the embedded binding.
	// These are already concrete values (CEL expressions have been evaluated).
	ResolvedParameters map[string]any

	// ResolvedEnvOverrides contains the CEL-resolved envOverride defaults from the embedded binding.
	ResolvedEnvOverrides map[string]any

	// Component is the component this trait is being applied to.
	Component *v1alpha1.Component `validate:"required"`

	// WorkloadData is the pre-computed workload data.
	WorkloadData WorkloadData

	// Configurations is the pre-computed configurations from workload.
	Configurations ContainerConfigurations

	// Metadata provides structured naming and labeling information.
	Metadata MetadataContext `validate:"required"`

	// SchemaCache is an optional cache for schema bundles.
	SchemaCache map[string]*SchemaBundle

	// DataPlane contains the data plane configuration.
	DataPlane *v1alpha1.DataPlane `validate:"required"`

	// Environment contains the environment configuration.
	Environment *v1alpha1.Environment `validate:"required"`
}

// ResolveEmbeddedTraitBindings resolves CEL expressions in an embedded trait's parameter
// and envOverride bindings against the component context.
//
// Values in the embedded trait bindings can be:
//   - Concrete values (locked by PE): passed through as-is
//   - CEL expressions like "${parameters.storage.mountPath}": evaluated against the component context
//
// Returns the resolved parameters and envOverrides as maps.
func ResolveEmbeddedTraitBindings(
	engine *template.Engine,
	embeddedTrait v1alpha1.ComponentTypeTrait,
	componentContextMap map[string]any,
) (resolvedParams map[string]any, resolvedEnvOverrides map[string]any, err error) {
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
// evaluates all CEL expressions against the given context, and returns a map with all values
// resolved to concrete types.
func resolveBindings(
	engine *template.Engine,
	raw *runtime.RawExtension,
	contextMap map[string]any,
) (map[string]any, error) {
	if raw == nil || raw.Raw == nil {
		return nil, nil
	}

	var data map[string]any
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bindings: %w", err)
	}

	resolved, err := engine.Render(data, contextMap)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate CEL bindings: %w", err)
	}

	resolvedMap, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resolved bindings is not a map, got type %T", resolved)
	}

	return resolvedMap, nil
}

// BuildEmbeddedTraitContext builds a CEL evaluation context for rendering an embedded trait's resources.
//
// Unlike BuildTraitContext, this function:
//   - Uses pre-resolved parameters (already evaluated from CEL bindings)
func BuildEmbeddedTraitContext(input *EmbeddedTraitContextInput) (*TraitContext, error) {
	if err := validate.Struct(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
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
			InstanceName: input.InstanceName,
		},
	}

	ctx.DataPlane = extractDataPlaneData(input.DataPlane)
	ctx.Environment = extractEnvironmentData(input.Environment, input.DataPlane)
	ctx.Workload = input.WorkloadData
	ctx.Configurations = input.Configurations

	return ctx, nil
}

// processEmbeddedTraitParameters processes parameters and envOverrides for an embedded trait.
//
// Parameters: Come from the resolved bindings (already concrete values from CEL evaluation).
// EnvOverrides: Come from the resolved bindings (already concrete values from CEL evaluation).
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

	// Process parameters: prune, apply defaults, validate
	var parameters map[string]any
	if parametersBundle != nil {
		parameters = make(map[string]any, len(input.ResolvedParameters))
		maps.Copy(parameters, input.ResolvedParameters)
		pruning.Prune(parameters, parametersBundle.Structural, false)
		parameters = schema.ApplyDefaults(parameters, parametersBundle.Structural)
		if err := schema.ValidateWithJSONSchema(parameters, parametersBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("parameters validation failed: %w", err)
		}
	} else {
		parameters = make(map[string]any)
	}

	// Process envOverrides: prune, apply defaults, validate
	var envOverrides map[string]any
	if envOverridesBundle != nil {
		envOverrides = make(map[string]any, len(input.ResolvedEnvOverrides))
		maps.Copy(envOverrides, input.ResolvedEnvOverrides)
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
