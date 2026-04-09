// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Note: validator is initialized in component.go

// BuildTraitContext builds a CEL evaluation context for rendering trait resources.
//
// The context includes:
//   - parameters: ResolvedParameters pruned to Trait.Spec.Parameters schema — access via ${parameters.*}
//   - environmentConfigs: ResolvedEnvironmentConfigs pruned to Trait.Spec.EnvironmentConfigs schema — access via ${environmentConfigs.*}
//   - trait: Trait metadata (name, instanceName) — access via ${trait.*}
//   - metadata: Structured naming and labeling information — access via ${metadata.*}
//
// Schema defaults are applied to both parameters and environmentConfigs sections.
//
// Both component-level and embedded traits use this builder. Callers obtain the resolved
// parameter/environmentConfigs maps separately:
//   - Component-level traits call ExtractTraitInstanceBindings.
//   - Embedded traits call ResolveEmbeddedTraitBindings.
//
// By the time the input reaches BuildTraitContext both flows look identical.
func BuildTraitContext(input *TraitContextInput) (*TraitContext, error) {
	if err := validate.Struct(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	parametersBundle, envConfigsBundle, err := getOrBuildTraitSchemaBundles(input.Trait, input.SchemaCache)
	if err != nil {
		return nil, err
	}

	parameters, err := applySchemaSection(input.ResolvedParameters, parametersBundle, "parameters")
	if err != nil {
		return nil, err
	}

	envConfigs, err := applySchemaSection(input.ResolvedEnvironmentConfigs, envConfigsBundle, "environmentConfigs")
	if err != nil {
		return nil, err
	}

	return makeTraitContext(input, parameters, envConfigs), nil
}

// ExtractTraitInstanceBindings produces concrete parameter and environmentConfigs maps for a
// component-level trait instance. Unlike ResolveEmbeddedTraitBindings, no CEL evaluation
// happens here — values are already concrete and we just JSON-deserialize them.
//
// The instance parameters come from instance.Parameters (a runtime.RawExtension); the
// environmentConfigs come from rb.Spec.TraitEnvironmentConfigs[instance.InstanceName], if
// present. A nil ReleaseBinding or a missing instance entry produces an empty envConfigs map.
//
// Note: TraitEnvironmentConfigs is keyed by instanceName (not traitName), as instanceNames
// must be unique across all traits in a component.
func ExtractTraitInstanceBindings(
	instance v1alpha1.ComponentTrait,
	rb *v1alpha1.ReleaseBinding,
) (parameters map[string]any, envConfigs map[string]any, err error) {
	parameters, err = extractParameters(instance.Parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract trait instance parameters: %w", err)
	}

	envConfigs, err = extractTraitEnvConfigs(rb, instance.InstanceName)
	if err != nil {
		return nil, nil, err
	}

	return parameters, envConfigs, nil
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

// makeTraitContext builds the final TraitContext struct from an input and the already-
// processed parameters and environmentConfigs maps. Metadata maps (Labels, Annotations,
// PodSelectors) are normalized to non-nil, and dataplane/environment/gateway data are
// extracted from the base. Called once by BuildTraitContext after schema processing.
func makeTraitContext(input *TraitContextInput, parameters, envConfigs map[string]any) *TraitContext {
	base := input.TraitContextBase

	metadata := base.Metadata
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

	ctx.DataPlane = extractDataPlaneData(base.DataPlane)
	ctx.Environment = extractEnvironmentData(base.Environment, base.DataPlane, base.DefaultNotificationChannel)
	ctx.Gateway = ctx.Environment.Gateway
	ctx.Workload = base.WorkloadData
	ctx.Configurations = base.Configurations
	ctx.Dependencies = newDependenciesContextData(base.Dependencies)

	return ctx
}

// getOrBuildTraitSchemaBundles retrieves the parameters and environmentConfigs schema bundles
// for a trait from the cache, building and caching them if either is missing. If either bundle
// is missing both are rebuilt in a single call so types unmarshaling is shared, matching the
// prior cache contract.
func getOrBuildTraitSchemaBundles(trait *v1alpha1.Trait, cache map[string]*SchemaBundle) (*SchemaBundle, *SchemaBundle, error) {
	cacheKey := traitSchemaCacheKey(trait)
	parametersBundle := getCachedSchemaBundle(cache, cacheKey+":parameters")
	envConfigsBundle := getCachedSchemaBundle(cache, cacheKey+":environmentConfigs")

	if parametersBundle == nil || envConfigsBundle == nil {
		var err error
		parametersBundle, envConfigsBundle, err = BuildStructuralSchemas(&SchemaInput{
			ParametersSchema:         trait.Spec.Parameters,
			EnvironmentConfigsSchema: trait.Spec.EnvironmentConfigs,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build trait schemas: %w", err)
		}
		setCachedSchemaBundle(cache, cacheKey+":parameters", parametersBundle)
		setCachedSchemaBundle(cache, cacheKey+":environmentConfigs", envConfigsBundle)
	}

	return parametersBundle, envConfigsBundle, nil
}

// extractTraitEnvConfigs pulls the environmentConfigs override for a specific trait instance
// from the ReleaseBinding, returning an empty map if the ReleaseBinding is nil or the instance
// has no override.
func extractTraitEnvConfigs(rb *v1alpha1.ReleaseBinding, instanceName string) (map[string]any, error) {
	if rb == nil || rb.Spec.TraitEnvironmentConfigs == nil {
		return make(map[string]any), nil
	}
	instanceOverride, ok := rb.Spec.TraitEnvironmentConfigs[instanceName]
	if !ok {
		return make(map[string]any), nil
	}
	envConfigs, err := extractParameters(&instanceOverride)
	if err != nil {
		return nil, fmt.Errorf("failed to extract trait environment configs: %w", err)
	}
	return envConfigs, nil
}

// traitSchemaCacheKey returns a cache key that includes both kind and name,
// so that a Trait and ClusterTrait with the same name get separate cache entries.
func traitSchemaCacheKey(trait *v1alpha1.Trait) string {
	return trait.Kind + ":" + trait.Name
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
