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

			if tt.wantParamsNil {
				if resolvedParams != nil {
					t.Errorf("expected nil params, got %v", resolvedParams)
				}
			} else {
				if diff := cmp.Diff(tt.wantParams, resolvedParams); diff != "" {
					t.Errorf("params mismatch (-want +got):\n%s", diff)
				}
			}

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
