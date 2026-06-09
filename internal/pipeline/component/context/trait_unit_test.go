// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// validTraitContextInput returns a minimal valid TraitContextInput for use as a base in tests.
func validTraitContextInput() *TraitContextInput {
	trait := &v1alpha1.Trait{}
	trait.Name = "my-trait"

	return &TraitContextInput{
		TraitContextBase: TraitContextBase{
			Metadata:  validMetadata(),
			DataPlane: &v1alpha1.DataPlane{},
			Environment: &v1alpha1.Environment{
				Spec: v1alpha1.EnvironmentSpec{
					DataPlaneRef: &v1alpha1.DataPlaneRef{
						Kind: v1alpha1.DataPlaneRefKindDataPlane,
						Name: "test-dp",
					},
				},
			},
			WorkloadData:   ExtractWorkloadData(nil),
			Configurations: ExtractConfigurationsFromWorkload(nil, nil),
		},
		Trait:        trait,
		InstanceName: "my-trait-instance",
	}
}

func TestBuildTraitContext_ValidationErrors(t *testing.T) {
	tests := []struct {
		name  string
		input *TraitContextInput
	}{
		{
			name: "nil Trait",
			input: func() *TraitContextInput {
				in := validTraitContextInput()
				in.Trait = nil
				return in
			}(),
		},
		{
			name: "nil DataPlane",
			input: func() *TraitContextInput {
				in := validTraitContextInput()
				in.DataPlane = nil
				return in
			}(),
		},
		{
			name: "nil Environment",
			input: func() *TraitContextInput {
				in := validTraitContextInput()
				in.Environment = nil
				return in
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := BuildTraitContext(tt.input)
			require.Error(t, err)
			assert.Nil(t, ctx)
			assert.Contains(t, err.Error(), "validation failed")
		})
	}
}

func TestBuildTraitContext_EmptyInstanceName(t *testing.T) {
	input := validTraitContextInput()
	input.InstanceName = ""

	ctx, err := BuildTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	// The required-field check is now enforced by the struct validator.
	assert.Contains(t, err.Error(), "validation failed")
}

func TestBuildTraitContext_NilMetadataMaps(t *testing.T) {
	// Nil maps fail validation because of validate:"required" tags on MetadataContext.
	// Verify the validator rejects nil maps.
	input := validTraitContextInput()
	input.Metadata.Labels = nil
	input.Metadata.Annotations = nil
	input.Metadata.PodSelectors = nil

	ctx, err := BuildTraitContext(input)
	require.Error(t, err, "nil metadata maps should be rejected by validator")
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "validation failed")

	// When empty maps are provided (passing validation), verify the init code
	// preserves them as initialized (not nil).
	input2 := validTraitContextInput()
	input2.Metadata.Labels = map[string]string{}
	input2.Metadata.Annotations = map[string]string{}
	input2.Metadata.PodSelectors = map[string]string{"app": "test"}

	ctx2, err := BuildTraitContext(input2)
	require.NoError(t, err)
	require.NotNil(t, ctx2)
	assert.NotNil(t, ctx2.Metadata.Labels)
	assert.NotNil(t, ctx2.Metadata.Annotations)
	assert.NotNil(t, ctx2.Metadata.PodSelectors)
}

func TestProcessTraitParameters_SchemaError(t *testing.T) {
	input := validTraitContextInput()
	// Provide an invalid OpenAPI v3 schema: properties is not an object.
	input.Trait.Spec.Parameters = &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{
			Raw: []byte(`{"type": "object", "properties": "invalid"}`),
		},
	}

	ctx, err := BuildTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "failed to build trait schemas")
}

func TestProcessTraitParameters_ValidationFailure(t *testing.T) {
	input := validTraitContextInput()
	// Schema expects an integer
	input.Trait.Spec.Parameters = openAPIV3Schema(objectSchema(map[string]any{
		"count": integerPropSchema(),
	}))
	// Provide a string where integer is expected
	input.ResolvedParameters = map[string]any{
		"count": "not-a-number",
	}

	ctx, err := BuildTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "parameters validation failed")
}

func TestProcessTraitParameters_NilEnvConfigsAppliesDefaults(t *testing.T) {
	input := validTraitContextInput()
	// nil resolved envConfigs (e.g., no override from the resolver)
	input.ResolvedEnvironmentConfigs = nil

	// Trait has environmentConfigs schema with a default
	input.Trait.Spec.EnvironmentConfigs = openAPIV3Schema(objectSchema(map[string]any{
		"replicas": integerPropSchema(1),
	}))

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	// Schema defaults should be applied even when resolved envConfigs is nil.
	// Schema defaults produce int64 values for integer types.
	assert.Equal(t, map[string]any{"replicas": int64(1)}, ctx.EnvironmentConfigs)
}

func TestBuildTraitContext_EmptyResolvedParamsAppliesDefaults(t *testing.T) {
	// Distinguish empty-map from nil: an explicit empty map should still trigger
	// schema default application, just like nil does.
	input := validTraitContextInput()
	input.Trait.Spec.Parameters = openAPIV3Schema(objectSchema(map[string]any{
		"timeout": stringPropSchema("30s"),
		"retries": integerPropSchema(3),
	}))
	input.ResolvedParameters = map[string]any{} // empty, not nil

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Equal(t, "30s", ctx.Parameters["timeout"])
	assert.Equal(t, int64(3), ctx.Parameters["retries"])
}

func TestBuildTraitContext_PrunesAndDefaults(t *testing.T) {
	input := validTraitContextInput()
	input.Trait.Spec.Parameters = openAPIV3Schema(objectSchema(map[string]any{
		"name":    stringPropSchema(),
		"timeout": stringPropSchema("30s"),
	}))
	input.Trait.Spec.EnvironmentConfigs = openAPIV3Schema(objectSchema(map[string]any{
		"replicas": integerPropSchema(1),
	}))

	// One valid key, one extra key to be pruned, timeout omitted so defaults apply
	input.ResolvedParameters = map[string]any{
		"name":     "my-app",
		"extraKey": "should-be-pruned",
	}
	// Empty so defaults are applied
	input.ResolvedEnvironmentConfigs = map[string]any{}

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	_, hasExtra := ctx.Parameters["extraKey"]
	assert.False(t, hasExtra, "extraKey should have been pruned")
	assert.Equal(t, "my-app", ctx.Parameters["name"])
	assert.Equal(t, "30s", ctx.Parameters["timeout"])
	assert.Equal(t, int64(1), ctx.EnvironmentConfigs["replicas"])
}

func TestBuildTraitContext_GatewaySet(t *testing.T) {
	input := validTraitContextInput()
	input.DataPlane.Spec.Gateway = v1alpha1.GatewaySpec{
		Ingress: &v1alpha1.GatewayNetworkSpec{
			External: &v1alpha1.GatewayEndpointSpec{
				Name:      "ext-gw",
				Namespace: "gw-ns",
				HTTPS: &v1alpha1.GatewayListenerSpec{
					ListenerName: "https",
					Port:         443,
					Host:         "api.example.com",
				},
			},
		},
	}

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Gateway should be derived from Environment (which merges with DataPlane)
	require.NotNil(t, ctx.Gateway, "ctx.Gateway should not be nil when DataPlane has gateway config")
	assert.Equal(t, ctx.Environment.Gateway, ctx.Gateway,
		"ctx.Gateway should equal ctx.Environment.Gateway")

	require.NotNil(t, ctx.Gateway.Ingress)
	require.NotNil(t, ctx.Gateway.Ingress.External)
	assert.Equal(t, "ext-gw", ctx.Gateway.Ingress.External.Name)
	require.NotNil(t, ctx.Gateway.Ingress.External.HTTPS)
	assert.Equal(t, "api.example.com", ctx.Gateway.Ingress.External.HTTPS.Host)
	assert.Equal(t, int32(443), ctx.Gateway.Ingress.External.HTTPS.Port)
}

func TestBuildTraitContext_NoEnvConfigsSchemaDiscardsOverride(t *testing.T) {
	// When the trait defines no EnvironmentConfigs schema, any provided envConfigs
	// should be discarded rather than propagated to the context.
	input := validTraitContextInput()
	input.ResolvedEnvironmentConfigs = map[string]any{
		"something": "value",
	}

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.Empty(t, ctx.EnvironmentConfigs, "envConfigs should be discarded when no schema is defined")
}

func TestBuildTraitContext_EnvConfigsValidationFailure(t *testing.T) {
	input := validTraitContextInput()
	input.Trait.Spec.EnvironmentConfigs = openAPIV3Schema(objectSchema(map[string]any{
		"count": integerPropSchema(),
	}))
	// Provide a string where integer is expected
	input.ResolvedEnvironmentConfigs = map[string]any{
		"count": "not-a-number",
	}

	ctx, err := BuildTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "environmentConfigs validation failed")
}

func TestExtractTraitInstanceBindings_Success(t *testing.T) {
	instance := v1alpha1.ComponentTrait{
		Name:         "storage",
		InstanceName: "app-storage",
		Parameters: &runtime.RawExtension{
			Raw: toJSON(t, map[string]any{
				"mountPath": "/var/data",
				"size":      "10Gi",
			}),
		},
	}
	rb := &v1alpha1.ReleaseBinding{
		Spec: v1alpha1.ReleaseBindingSpec{
			TraitEnvironmentConfigs: map[string]runtime.RawExtension{
				"app-storage": {
					Raw: toJSON(t, map[string]any{
						"replicas": float64(5),
					}),
				},
			},
		},
	}

	params, envConfigs, err := ExtractTraitInstanceBindings(instance, rb)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"mountPath": "/var/data",
		"size":      "10Gi",
	}, params)
	assert.Equal(t, map[string]any{
		"replicas": float64(5),
	}, envConfigs)
}

func TestExtractTraitInstanceBindings_InstanceParamsInvalidJSON(t *testing.T) {
	instance := v1alpha1.ComponentTrait{
		Name:         "my-trait",
		InstanceName: "my-trait-instance",
		Parameters: &runtime.RawExtension{
			Raw: []byte(`{invalid json}`),
		},
	}

	_, _, err := ExtractTraitInstanceBindings(instance, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to extract trait instance parameters")
}

func TestExtractTraitInstanceBindings_ReleaseBindingEnvConfigsInvalidJSON(t *testing.T) {
	instance := v1alpha1.ComponentTrait{
		Name:         "my-trait",
		InstanceName: "my-trait-instance",
	}
	rb := &v1alpha1.ReleaseBinding{
		Spec: v1alpha1.ReleaseBindingSpec{
			TraitEnvironmentConfigs: map[string]runtime.RawExtension{
				instance.InstanceName: {
					Raw: []byte(`{invalid`),
				},
			},
		},
	}

	_, _, err := ExtractTraitInstanceBindings(instance, rb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to extract trait environment configs")
}

func TestExtractTraitInstanceBindings_EnvConfigsMissingForInstance(t *testing.T) {
	instance := v1alpha1.ComponentTrait{
		Name:         "my-trait",
		InstanceName: "my-trait-instance",
	}
	rb := &v1alpha1.ReleaseBinding{
		Spec: v1alpha1.ReleaseBindingSpec{
			TraitEnvironmentConfigs: map[string]runtime.RawExtension{
				"other-instance": {
					Raw: toJSON(t, map[string]any{"size": "large"}),
				},
			},
		},
	}

	_, envConfigs, err := ExtractTraitInstanceBindings(instance, rb)
	require.NoError(t, err)
	// Instance is not in the override map, so envConfigs should be empty.
	assert.Empty(t, envConfigs)
}

func TestExtractTraitInstanceBindings_NilReleaseBinding(t *testing.T) {
	instance := v1alpha1.ComponentTrait{
		Name:         "my-trait",
		InstanceName: "my-trait-instance",
	}

	params, envConfigs, err := ExtractTraitInstanceBindings(instance, nil)
	require.NoError(t, err)
	assert.Empty(t, params)
	assert.Empty(t, envConfigs)
}

func TestProcessTraitParameters_SchemaCache(t *testing.T) {
	input := validTraitContextInput()

	// Build valid schemas for both parameters and environmentConfigs and cache them.
	// Both must be non-nil in the cache — the code rebuilds if either is nil.
	paramSchema := openAPIV3Schema(objectSchema(map[string]any{
		"name": stringPropSchema("hello"),
	}))
	envConfigSchema := openAPIV3Schema(objectSchema(map[string]any{
		"level": stringPropSchema("info"),
	}))
	cache := make(map[string]*SchemaBundle)
	paramBundle, _, err := BuildStructuralSchemas(&SchemaInput{
		ParametersSchema: paramSchema,
	})
	require.NoError(t, err)
	envConfigBundle, _, err := BuildStructuralSchemas(&SchemaInput{
		ParametersSchema: envConfigSchema,
	})
	require.NoError(t, err)
	traitCacheKey := traitSchemaCacheKey(input.Trait)
	setCachedSchemaBundle(cache, traitCacheKey+":parameters", paramBundle)
	setCachedSchemaBundle(cache, traitCacheKey+":environmentConfigs", envConfigBundle)
	input.SchemaCache = cache

	// Poison the live trait schema so that rebuilding from scratch would fail.
	// If the cache is used, this broken schema is never parsed.
	input.Trait.Spec.Parameters = &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{
			Raw: []byte(`{"type": "object", "properties": "invalid"}`),
		},
	}

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Default from the cached schema should be applied
	assert.Equal(t, "hello", ctx.Parameters["name"])
}

func TestTraitSchemaCacheKey_DistinguishesKinds(t *testing.T) {
	trait := &v1alpha1.Trait{}
	trait.Kind = "Trait"
	trait.Name = "storage"

	clusterTrait := &v1alpha1.Trait{}
	clusterTrait.Kind = "ClusterTrait"
	clusterTrait.Name = "storage"

	assert.Equal(t, "Trait:storage", traitSchemaCacheKey(trait))
	assert.Equal(t, "ClusterTrait:storage", traitSchemaCacheKey(clusterTrait))
	assert.NotEqual(t, traitSchemaCacheKey(trait), traitSchemaCacheKey(clusterTrait))
}

func TestGetCachedSchemaBundle_NilCache(t *testing.T) {
	result := getCachedSchemaBundle(nil, "any-key")
	assert.Nil(t, result)
}

func TestSetCachedSchemaBundle_NilCache(t *testing.T) {
	// Should not panic
	bundle := &SchemaBundle{}
	assert.NotPanics(t, func() {
		setCachedSchemaBundle(nil, "any-key", bundle)
	})
}

// toJSON marshals v to JSON, failing the test on error.
func toJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
