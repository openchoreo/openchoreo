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

	// ResolvedEnvironmentConfigs contains the CEL-resolved environmentConfig defaults from the embedded binding.
	ResolvedEnvironmentConfigs map[string]any

	// Component is the component this trait is being applied to.
	Component *v1alpha1.Component `validate:"required"`

	// WorkloadData is the pre-computed workload data.
	WorkloadData WorkloadData

	// Configurations is the pre-computed configurations from workload.
	Configurations ContainerConfigurations

	// Connections contains pre-computed connection environment variables.
	Connections ConnectionsData

	// Metadata provides structured naming and labeling information.
	Metadata MetadataContext `validate:"required"`

	// SchemaCache is an optional cache for schema bundles.
	SchemaCache map[string]*SchemaBundle

	// DataPlane contains the data plane configuration.
	DataPlane *v1alpha1.DataPlane `validate:"required"`

	// Environment contains the environment configuration.
	Environment *v1alpha1.Environment `validate:"required"`

	// DefaultNotificationChannel is the default notification channel name for the environment.
	// Optional - if not provided, the defaultNotificationChannel field in EnvironmentData will be empty.
	DefaultNotificationChannel string
}

// ResolveEmbeddedTraitBindings resolves CEL expressions in an embedded trait's parameter
// and environmentConfigs bindings against the component context.
//
// Values in the embedded trait bindings can be:
//   - Concrete values (locked by PE): passed through as-is
//   - CEL expressions like "${parameters.storage.mountPath}": evaluated against the component context
//
// Returns the resolved parameters and environmentConfigs as maps.
func ResolveEmbeddedTraitBindings(
	engine *template.Engine,
	embeddedTrait v1alpha1.ComponentTypeTrait,
	componentContextMap map[string]any,
) (resolvedParams map[string]any, resolvedEnvConfigs map[string]any, err error) {
	resolvedParams, err = resolveBindings(engine, embeddedTrait.Parameters, componentContextMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve embedded trait %s/%s parameters: %w",
			embeddedTrait.Name, embeddedTrait.InstanceName, err)
	}

	resolvedEnvConfigs, err = resolveBindings(engine, embeddedTrait.EnvironmentConfigs, componentContextMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve embedded trait %s/%s environmentConfigs: %w",
			embeddedTrait.Name, embeddedTrait.InstanceName, err)
	}

	return resolvedParams, resolvedEnvConfigs, nil
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

	parameters, envConfigs, err := processEmbeddedTraitParameters(input)
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
		Parameters:         parameters,
		EnvironmentConfigs: envConfigs,
		Metadata:           metadata,
		Trait: TraitMetadata{
			Name:         input.Trait.Name,
			InstanceName: input.InstanceName,
		},
	}

	ctx.DataPlane = extractDataPlaneData(input.DataPlane)
	ctx.Environment = extractEnvironmentData(input.Environment, input.DataPlane, input.DefaultNotificationChannel)
	ctx.Workload = input.WorkloadData
	ctx.Configurations = input.Configurations
	ctx.Connections = newConnectionsContextData(input.Connections)

	return ctx, nil
}

// processEmbeddedTraitParameters processes parameters and environmentConfigs for an embedded trait.
//
// Parameters: Come from the resolved bindings (already concrete values from CEL evaluation).
// EnvironmentConfigs: Come from the resolved bindings (already concrete values from CEL evaluation).
func processEmbeddedTraitParameters(input *EmbeddedTraitContextInput) (map[string]any, map[string]any, error) {
	traitName := input.Trait.Name

	// Build or retrieve schema bundles
	parametersBundle := getCachedSchemaBundle(input.SchemaCache, traitName+":parameters")
	envConfigsBundle := getCachedSchemaBundle(input.SchemaCache, traitName+":environmentConfigs")

	if parametersBundle == nil || envConfigsBundle == nil {
		var err error
		parametersBundle, envConfigsBundle, err = BuildStructuralSchemas(&SchemaInput{
			ParametersSchema:         input.Trait.Spec.Parameters,
			EnvironmentConfigsSchema: input.Trait.Spec.EnvironmentConfigs,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build trait schemas: %w", err)
		}
		setCachedSchemaBundle(input.SchemaCache, traitName+":parameters", parametersBundle)
		setCachedSchemaBundle(input.SchemaCache, traitName+":environmentConfigs", envConfigsBundle)
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

	// Process environmentConfigs: prune, apply defaults, validate
	var envConfigs map[string]any
	if envConfigsBundle != nil {
		envConfigs = make(map[string]any, len(input.ResolvedEnvironmentConfigs))
		maps.Copy(envConfigs, input.ResolvedEnvironmentConfigs)
		pruning.Prune(envConfigs, envConfigsBundle.Structural, false)
		envConfigs = schema.ApplyDefaults(envConfigs, envConfigsBundle.Structural)
		if err := schema.ValidateWithJSONSchema(envConfigs, envConfigsBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("environmentConfigs validation failed: %w", err)
		}
	} else {
		envConfigs = make(map[string]any)
	}

	return parameters, envConfigs, nil
}
