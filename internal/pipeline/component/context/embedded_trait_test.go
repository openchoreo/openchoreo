// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

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
				Name:         "storage",
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
				Name:         "storage",
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
	baseTrait.Name = "storage"
	baseTrait.Spec.Parameters = &v1alpha1.SchemaSection{
		OCSchema: &runtime.RawExtension{
			Raw: mustMarshalJSON(t, map[string]any{
				"mountPath": "string",
				"size":      "string | default=\"5Gi\"",
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
			wantTraitName:          "storage",
			wantInstanceName:       "app-storage",
		},
		{
			name: "embedded trait with resolved environmentConfigs",
			input: &EmbeddedTraitContextInput{
				Trait: func() *v1alpha1.Trait {
					t := &v1alpha1.Trait{}
					t.Name = "logging"
					t.Spec.Parameters = &v1alpha1.SchemaSection{
						OCSchema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"format": "string | default=\"json\"",
							}),
						},
					}
					t.Spec.EnvironmentConfigs = &v1alpha1.SchemaSection{
						OCSchema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"logLevel": "string | default=\"info\"",
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
						OCSchema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"logLevel": "string | default=\"info\"",
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
						OCSchema: &runtime.RawExtension{
							Raw: mustMarshalJSON(nil, map[string]any{
								"timeout": "string | default=\"30s\"",
								"retries": "number | default=3",
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
