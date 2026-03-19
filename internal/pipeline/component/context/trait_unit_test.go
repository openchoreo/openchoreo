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
		Trait: trait,
		Instance: v1alpha1.ComponentTrait{
			Name:         "my-trait",
			InstanceName: "my-trait-instance",
		},
		Component:      &v1alpha1.Component{},
		WorkloadData:   ExtractWorkloadData(nil),
		Configurations: ExtractConfigurationsFromWorkload(nil, nil),
		Metadata:       validMetadata(),
		DataPlane:      &v1alpha1.DataPlane{},
		Environment: &v1alpha1.Environment{
			Spec: v1alpha1.EnvironmentSpec{
				DataPlaneRef: &v1alpha1.DataPlaneRef{
					Kind: v1alpha1.DataPlaneRefKindDataPlane,
					Name: "test-dp",
				},
			},
		},
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
			name: "nil Component",
			input: func() *TraitContextInput {
				in := validTraitContextInput()
				in.Component = nil
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
	input.Instance.InstanceName = ""

	ctx, err := BuildTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "trait instance name is required")
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
	input.Instance.Parameters = &runtime.RawExtension{
		Raw: toJSON(t, map[string]any{
			"count": "not-a-number",
		}),
	}

	ctx, err := BuildTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "parameters validation failed")
}

func TestProcessTraitParameters_NoReleaseBinding(t *testing.T) {
	input := validTraitContextInput()
	input.ReleaseBinding = nil

	// Set up a trait with environmentConfigs schema so we can verify envConfigs is empty
	input.Trait.Spec.EnvironmentConfigs = openAPIV3Schema(objectSchema(map[string]any{
		"replicas": integerPropSchema(1),
	}))

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	// With nil ReleaseBinding, envConfigs should get schema defaults applied.
	// Schema defaults produce int64 values for integer types.
	assert.Equal(t, map[string]any{"replicas": int64(1)}, ctx.EnvironmentConfigs)
}

func TestProcessTraitParameters_EnvConfigsMissing(t *testing.T) {
	input := validTraitContextInput()
	input.ReleaseBinding = &v1alpha1.ReleaseBinding{
		Spec: v1alpha1.ReleaseBindingSpec{
			TraitEnvironmentConfigs: map[string]runtime.RawExtension{
				"other-instance": {
					Raw: toJSON(t, map[string]any{"size": "large"}),
				},
			},
		},
	}

	// Trait has environmentConfigs schema
	input.Trait.Spec.EnvironmentConfigs = openAPIV3Schema(objectSchema(map[string]any{
		"size": stringPropSchema("small"),
	}))

	ctx, err := BuildTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	// The instanceName is "my-trait-instance" but ReleaseBinding only has "other-instance",
	// so envConfigs should be empty map with defaults applied from schema
	assert.Equal(t, map[string]any{"size": "small"}, ctx.EnvironmentConfigs)
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
