// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

const testTraitNameStorage = "storage"

func TestResolveEmbeddedTraitBindings(t *testing.T) {
	engine := template.NewEngineWithOptions(
		template.WithCELExtensions(CELExtensions()...),
	)

	tests := []struct {
		name                      string
		embeddedTrait             v1alpha1.ComponentTypeTrait
		componentContext          map[string]any
		wantParams                map[string]any
		wantEnvironmentConfigs    map[string]any
		wantParamsNil             bool
		wantEnvironmentConfigsNil bool
		wantErr                   bool
	}{
		{
			name: "concrete-only values (locked by PE)",
			embeddedTrait: v1alpha1.ComponentTypeTrait{
				Name:         testTraitNameStorage,
				InstanceName: "app-storage",
				Parameters: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"volumeName": "app-data",
						"size":       "10Gi",
					}),
				},
			},
			componentContext: map[string]any{
				"parameters": map[string]any{
					"replicas": float64(3),
				},
			},
			wantParams: map[string]any{
				"volumeName": "app-data",
				"size":       "10Gi",
			},
			wantEnvironmentConfigsNil: true,
		},
		{
			name: "CEL expressions referencing parameters",
			embeddedTrait: v1alpha1.ComponentTypeTrait{
				Name:         testTraitNameStorage,
				InstanceName: "app-storage",
				Parameters: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"mountPath":  "${parameters.storage.mountPath}",
						"volumeName": "app-data",
					}),
				},
			},
			componentContext: map[string]any{
				"parameters": map[string]any{
					"storage": map[string]any{
						"mountPath": "/var/data",
					},
				},
			},
			wantParams: map[string]any{
				"mountPath":  "/var/data",
				"volumeName": "app-data",
			},
			wantEnvironmentConfigsNil: true,
		},
		{
			name: "mixed concrete and CEL values in both params and environmentConfigs",
			embeddedTrait: v1alpha1.ComponentTypeTrait{
				Name:         "logging",
				InstanceName: "app-logging",
				Parameters: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"format":  "json",
						"appName": "${parameters.appName}",
					}),
				},
				EnvironmentConfigs: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"logLevel": "${environmentConfigs.logLevel}",
						"output":   "stdout",
					}),
				},
			},
			componentContext: map[string]any{
				"parameters": map[string]any{
					"appName": "my-service",
				},
				"environmentConfigs": map[string]any{
					"logLevel": "debug",
				},
			},
			wantParams: map[string]any{
				"format":  "json",
				"appName": "my-service",
			},
			wantEnvironmentConfigs: map[string]any{
				"logLevel": "debug",
				"output":   "stdout",
			},
		},
		{
			name: "nil parameters and environmentConfigs",
			embeddedTrait: v1alpha1.ComponentTypeTrait{
				Name:         "simple",
				InstanceName: "simple-1",
			},
			componentContext:          map[string]any{},
			wantParamsNil:             true,
			wantEnvironmentConfigsNil: true,
		},
		{
			name: "invalid CEL expression",
			embeddedTrait: v1alpha1.ComponentTypeTrait{
				Name:         "bad-trait",
				InstanceName: "bad-1",
				Parameters: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"value": "${invalid.nonexistent.path}",
					}),
				},
			},
			componentContext: map[string]any{
				"parameters": map[string]any{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedParams, resolvedEnvironmentConfigs, err := ResolveEmbeddedTraitBindings(
				engine,
				tt.embeddedTrait,
				tt.componentContext,
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveEmbeddedTraitBindings() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Check params
			if tt.wantParamsNil {
				if resolvedParams != nil {
					t.Errorf("expected nil params, got %v", resolvedParams)
				}
			} else {
				if diff := cmp.Diff(tt.wantParams, resolvedParams); diff != "" {
					t.Errorf("params mismatch (-want +got):\n%s", diff)
				}
			}

			// Check environmentConfigs
			if tt.wantEnvironmentConfigsNil {
				if resolvedEnvironmentConfigs != nil {
					t.Errorf("expected nil environmentConfigs, got %v", resolvedEnvironmentConfigs)
				}
			} else {
				if diff := cmp.Diff(tt.wantEnvironmentConfigs, resolvedEnvironmentConfigs); diff != "" {
					t.Errorf("environmentConfigs mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestBuildEmbeddedTraitContext(t *testing.T) {
	baseTrait := &v1alpha1.Trait{}
	baseTrait.Name = testTraitNameStorage
	baseTrait.Spec.Parameters = &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{
			Raw: mustMarshalJSON(t, map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mountPath": map[string]any{
						"type": "string",
					},
					"size": map[string]any{
						"type":    "string",
						"default": "5Gi",
					},
				},
			}),
		},
	}

	baseDataPlane := &v1alpha1.DataPlane{}

	baseEnvironment := &v1alpha1.Environment{}
	baseEnvironment.Spec.DataPlaneRef = &v1alpha1.DataPlaneRef{
		Kind: v1alpha1.DataPlaneRefKindDataPlane,
		Name: "test-dataplane",
	}

	baseMetadata := MetadataContext{
		Name: "test", Namespace: "ns", ComponentName: "app", ComponentUID: "uid1",
		ComponentNamespace: "test-namespace",
		ProjectName:        "proj", ProjectUID: "uid2", DataPlaneName: "dp", DataPlaneUID: "uid3",
		EnvironmentName: "dev", EnvironmentUID: "uid4",
		Labels: map[string]string{}, Annotations: map[string]string{},
		PodSelectors: map[string]string{"k": "v"},
	}

	tests := []struct {
		name                   string
		input                  *EmbeddedTraitContextInput
		wantParams             map[string]any
		wantEnvironmentConfigs map[string]any
		wantTraitName          string
		wantInstanceName       string
		wantErr                bool
	}{
		{
			name: "basic embedded trait context with resolved parameters",
			input: &EmbeddedTraitContextInput{
				Trait:        baseTrait,
				InstanceName: "app-storage",
				ResolvedParameters: map[string]any{
					"mountPath": "/var/data",
					"size":      "10Gi",
				},
				Component:   &v1alpha1.Component{},
				DataPlane:   baseDataPlane,
				Environment: baseEnvironment,
				Metadata:    baseMetadata,
			},
			wantParams: map[string]any{
				"mountPath": "/var/data",
				"size":      "10Gi",
			},
			wantEnvironmentConfigs: map[string]any{},
			wantTraitName:          testTraitNameStorage,
			wantInstanceName:       "app-storage",
		},
		{
			name: "embedded trait with resolved environmentConfigs",
			input: &EmbeddedTraitContextInput{
				Trait: func() *v1alpha1.Trait {
					t := &v1alpha1.Trait{}
					t.Name = "logging"
					t.Spec.Parameters = &v1alpha1.SchemaSection{
						OpenAPIV3Schema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"type": "object",
								"properties": map[string]any{
									"format": map[string]any{
										"type":    "string",
										"default": "json",
									},
								},
							}),
						},
					}
					t.Spec.EnvironmentConfigs = &v1alpha1.SchemaSection{
						OpenAPIV3Schema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"type": "object",
								"properties": map[string]any{
									"logLevel": map[string]any{
										"type":    "string",
										"default": "info",
									},
								},
							}),
						},
					}
					return t
				}(),
				InstanceName: "app-logging",
				ResolvedEnvironmentConfigs: map[string]any{
					"logLevel": "debug",
				},
				Component:   &v1alpha1.Component{},
				DataPlane:   baseDataPlane,
				Environment: baseEnvironment,
				Metadata:    baseMetadata,
			},
			wantParams: map[string]any{
				"format": "json",
			},
			wantEnvironmentConfigs: map[string]any{
				"logLevel": "debug",
			},
			wantTraitName:    "logging",
			wantInstanceName: "app-logging",
		},
		{
			name: "embedded trait with nil environmentConfigs uses schema defaults",
			input: &EmbeddedTraitContextInput{
				Trait: func() *v1alpha1.Trait {
					t := &v1alpha1.Trait{}
					t.Name = "logging"
					t.Spec.EnvironmentConfigs = &v1alpha1.SchemaSection{
						OpenAPIV3Schema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"type": "object",
								"properties": map[string]any{
									"logLevel": map[string]any{
										"type":    "string",
										"default": "info",
									},
								},
							}),
						},
					}
					return t
				}(),
				InstanceName: "app-logging",
				Component:    &v1alpha1.Component{},
				DataPlane:    baseDataPlane,
				Environment:  baseEnvironment,
				Metadata:     baseMetadata,
			},
			wantParams: map[string]any{},
			wantEnvironmentConfigs: map[string]any{
				"logLevel": "info", // Schema default applied
			},
			wantTraitName:    "logging",
			wantInstanceName: "app-logging",
		},
		{
			name: "embedded trait with empty map parameters applies schema defaults",
			input: &EmbeddedTraitContextInput{
				Trait: func() *v1alpha1.Trait {
					t := &v1alpha1.Trait{}
					t.Name = "optional-config"
					t.Spec.Parameters = &v1alpha1.SchemaSection{
						OpenAPIV3Schema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"type": "object",
								"properties": map[string]any{
									"timeout": map[string]any{
										"type":    "string",
										"default": "30s",
									},
									"retries": map[string]any{
										"type":    "number",
										"default": float64(3),
									},
								},
							}),
						},
					}
					return t
				}(),
				InstanceName:       "app-config",
				ResolvedParameters: map[string]any{}, // Empty map instead of nil
				Component:          &v1alpha1.Component{},
				DataPlane:          baseDataPlane,
				Environment:        baseEnvironment,
				Metadata:           baseMetadata,
			},
			wantParams: map[string]any{
				"timeout": "30s", // Schema defaults applied
				"retries": float64(3),
			},
			wantEnvironmentConfigs: map[string]any{},
			wantTraitName:          "optional-config",
			wantInstanceName:       "app-config",
		},
		{
			name: "missing instance name",
			input: &EmbeddedTraitContextInput{
				Trait:       baseTrait,
				Component:   &v1alpha1.Component{},
				DataPlane:   baseDataPlane,
				Environment: baseEnvironment,
				Metadata:    baseMetadata,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := BuildEmbeddedTraitContext(tt.input)

			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildEmbeddedTraitContext() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			ctxMap := ctx.ToMap()

			// Verify parameters
			params, _ := ctxMap["parameters"].(map[string]any)
			if diff := cmp.Diff(tt.wantParams, params); diff != "" {
				t.Errorf("parameters mismatch (-want +got):\n%s", diff)
			}

			// Verify environmentConfigs
			environmentConfigs, _ := ctxMap["environmentConfigs"].(map[string]any)
			if diff := cmp.Diff(tt.wantEnvironmentConfigs, environmentConfigs); diff != "" {
				t.Errorf("environmentConfigs mismatch (-want +got):\n%s", diff)
			}

			// Verify trait metadata
			traitMeta, _ := ctxMap["trait"].(map[string]any)
			if traitMeta["name"] != tt.wantTraitName {
				t.Errorf("trait.name = %v, want %v", traitMeta["name"], tt.wantTraitName)
			}
			if traitMeta["instanceName"] != tt.wantInstanceName {
				t.Errorf("trait.instanceName = %v, want %v", traitMeta["instanceName"], tt.wantInstanceName)
			}
		})
	}
}

// mustMarshalJSON marshals v to JSON bytes, failing the test on error.
// If t is nil, it panics on error (for use in struct initializers).
func mustMarshalJSON(t *testing.T, v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		if t != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}
		panic(err)
	}
	return data
}

// validEmbeddedTraitContextInput returns a minimal valid EmbeddedTraitContextInput for use in tests.
func validEmbeddedTraitContextInput() *EmbeddedTraitContextInput {
	trait := &v1alpha1.Trait{}
	trait.Name = "embedded-trait"

	return &EmbeddedTraitContextInput{
		Trait:          trait,
		InstanceName:   "embedded-instance",
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

func TestResolveEmbeddedTraitBindings_EnvConfigsError(t *testing.T) {
	engine := template.NewEngineWithOptions(
		template.WithCELExtensions(CELExtensions()...),
	)

	embeddedTrait := v1alpha1.ComponentTypeTrait{
		Name:         "bad-env-trait",
		InstanceName: "bad-env-1",
		EnvironmentConfigs: &runtime.RawExtension{
			Raw: mustMarshalJSON(t, map[string]any{
				"level": "${invalid.nonexistent.field}",
			}),
		},
	}

	componentContext := map[string]any{
		"parameters": map[string]any{},
	}

	_, _, err := ResolveEmbeddedTraitBindings(engine, embeddedTrait, componentContext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "environmentConfigs")
}

func TestResolveBindings_NilRaw(t *testing.T) {
	engine := template.NewEngineWithOptions(
		template.WithCELExtensions(CELExtensions()...),
	)

	// nil RawExtension
	result, err := resolveBindings(engine, nil, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, result)

	// Non-nil RawExtension but nil Raw bytes
	result, err = resolveBindings(engine, &runtime.RawExtension{Raw: nil}, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveBindings_UnmarshalError(t *testing.T) {
	engine := template.NewEngineWithOptions(
		template.WithCELExtensions(CELExtensions()...),
	)

	raw := &runtime.RawExtension{
		Raw: []byte(`{invalid json`),
	}

	result, err := resolveBindings(engine, raw, map[string]any{})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestBuildEmbeddedTraitContext_ValidationError(t *testing.T) {
	tests := []struct {
		name  string
		input *EmbeddedTraitContextInput
	}{
		{
			name: "nil Trait",
			input: func() *EmbeddedTraitContextInput {
				in := validEmbeddedTraitContextInput()
				in.Trait = nil
				return in
			}(),
		},
		{
			name: "empty InstanceName",
			input: func() *EmbeddedTraitContextInput {
				in := validEmbeddedTraitContextInput()
				in.InstanceName = ""
				return in
			}(),
		},
		{
			name: "nil Component",
			input: func() *EmbeddedTraitContextInput {
				in := validEmbeddedTraitContextInput()
				in.Component = nil
				return in
			}(),
		},
		{
			name: "nil DataPlane",
			input: func() *EmbeddedTraitContextInput {
				in := validEmbeddedTraitContextInput()
				in.DataPlane = nil
				return in
			}(),
		},
		{
			name: "nil Environment",
			input: func() *EmbeddedTraitContextInput {
				in := validEmbeddedTraitContextInput()
				in.Environment = nil
				return in
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := BuildEmbeddedTraitContext(tt.input)
			require.Error(t, err)
			assert.Nil(t, ctx)
			assert.Contains(t, err.Error(), "validation failed")
		})
	}
}

func TestBuildEmbeddedTraitContext_NilMetadataMaps(t *testing.T) {
	// Nil maps fail validation because of validate:"required" tags on MetadataContext.
	input := validEmbeddedTraitContextInput()
	input.Metadata.Labels = nil
	input.Metadata.Annotations = nil
	input.Metadata.PodSelectors = nil

	ctx, err := BuildEmbeddedTraitContext(input)
	require.Error(t, err, "nil metadata maps should be rejected by validator")
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "validation failed")

	// When empty maps are provided (passing validation), verify the init code
	// preserves them as initialized (not nil).
	input2 := validEmbeddedTraitContextInput()
	input2.Metadata.Labels = map[string]string{}
	input2.Metadata.Annotations = map[string]string{}
	input2.Metadata.PodSelectors = map[string]string{"app": "test"}

	ctx2, err := BuildEmbeddedTraitContext(input2)
	require.NoError(t, err)
	require.NotNil(t, ctx2)
	assert.NotNil(t, ctx2.Metadata.Labels)
	assert.NotNil(t, ctx2.Metadata.Annotations)
	assert.NotNil(t, ctx2.Metadata.PodSelectors)
}

func TestBuildEmbeddedTraitContext_GatewaySet(t *testing.T) {
	input := validEmbeddedTraitContextInput()
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

	ctx, err := BuildEmbeddedTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Gateway should be derived from Environment (which merges with DataPlane)
	require.NotNil(t, ctx.Gateway, "ctx.Gateway should not be nil when DataPlane has gateway config")
	assert.Equal(t, ctx.Environment.Gateway, ctx.Gateway,
		"ctx.Gateway should equal ctx.Environment.Gateway")

	// Verify the gateway data is populated correctly
	require.NotNil(t, ctx.Gateway.Ingress)
	require.NotNil(t, ctx.Gateway.Ingress.External)
	assert.Equal(t, "ext-gw", ctx.Gateway.Ingress.External.Name)
	require.NotNil(t, ctx.Gateway.Ingress.External.HTTPS)
	assert.Equal(t, "api.example.com", ctx.Gateway.Ingress.External.HTTPS.Host)
	assert.Equal(t, int32(443), ctx.Gateway.Ingress.External.HTTPS.Port)
}

func TestProcessEmbeddedTraitParameters_SchemaError(t *testing.T) {
	input := validEmbeddedTraitContextInput()
	input.Trait.Spec.Parameters = &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{
			Raw: []byte(`{"type": "object", "properties": "invalid"}`),
		},
	}

	ctx, err := BuildEmbeddedTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "failed to build trait schemas")
}

func TestProcessEmbeddedTraitParameters_ValidationFailure(t *testing.T) {
	input := validEmbeddedTraitContextInput()
	input.Trait.Spec.Parameters = openAPIV3Schema(objectSchema(map[string]any{
		"count": integerPropSchema(),
	}))
	// Provide a string value where integer is expected
	input.ResolvedParameters = map[string]any{
		"count": "not-a-number",
	}

	ctx, err := BuildEmbeddedTraitContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "parameters validation failed")
}

func TestProcessEmbeddedTraitParameters_PruneAndDefault(t *testing.T) {
	input := validEmbeddedTraitContextInput()
	input.Trait.Spec.Parameters = openAPIV3Schema(objectSchema(map[string]any{
		"name":    stringPropSchema(),
		"timeout": stringPropSchema("30s"),
	}))
	input.Trait.Spec.EnvironmentConfigs = openAPIV3Schema(objectSchema(map[string]any{
		"replicas": integerPropSchema(1),
	}))

	// Provide parameters: one valid key, one extra key to be pruned, and omit "timeout" for default
	input.ResolvedParameters = map[string]any{
		"name":     "my-app",
		"extraKey": "should-be-pruned",
	}
	// Provide empty environmentConfigs so defaults are applied
	input.ResolvedEnvironmentConfigs = map[string]any{}

	ctx, err := BuildEmbeddedTraitContext(input)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// extraKey should be pruned
	_, hasExtra := ctx.Parameters["extraKey"]
	assert.False(t, hasExtra, "extraKey should have been pruned from parameters")

	// name should remain
	assert.Equal(t, "my-app", ctx.Parameters["name"])

	// timeout should get schema default
	assert.Equal(t, "30s", ctx.Parameters["timeout"])

	// replicas should get schema default (schema defaults produce int64 for integers)
	assert.Equal(t, int64(1), ctx.EnvironmentConfigs["replicas"])
}
