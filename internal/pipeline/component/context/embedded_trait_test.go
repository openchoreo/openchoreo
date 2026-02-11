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
		name                string
		embeddedTrait       v1alpha1.ComponentTypeTrait
		componentContext    map[string]any
		wantParams          map[string]any
		wantEnvOverrides    map[string]any
		wantParamsNil       bool
		wantEnvOverridesNil bool
		wantErr             bool
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
			wantEnvOverridesNil: true,
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
			wantEnvOverridesNil: true,
		},
		{
			name: "mixed concrete and CEL values in both params and envOverrides",
			embeddedTrait: v1alpha1.ComponentTypeTrait{
				Name:         "logging",
				InstanceName: "app-logging",
				Parameters: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"format":  "json",
						"appName": "${parameters.appName}",
					}),
				},
				EnvOverrides: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"logLevel": "${envOverrides.logLevel}",
						"output":   "stdout",
					}),
				},
			},
			componentContext: map[string]any{
				"parameters": map[string]any{
					"appName": "my-service",
				},
				"envOverrides": map[string]any{
					"logLevel": "debug",
				},
			},
			wantParams: map[string]any{
				"format":  "json",
				"appName": "my-service",
			},
			wantEnvOverrides: map[string]any{
				"logLevel": "debug",
				"output":   "stdout",
			},
		},
		{
			name: "nil parameters and envOverrides",
			embeddedTrait: v1alpha1.ComponentTypeTrait{
				Name:         "simple",
				InstanceName: "simple-1",
			},
			componentContext:    map[string]any{},
			wantParamsNil:       true,
			wantEnvOverridesNil: true,
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
			resolvedParams, resolvedEnvOverrides, err := ResolveEmbeddedTraitBindings(
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
				gotParams := unmarshalRawExtension(t, resolvedParams)
				if diff := cmp.Diff(tt.wantParams, gotParams); diff != "" {
					t.Errorf("params mismatch (-want +got):\n%s", diff)
				}
			}

			// Check envOverrides
			if tt.wantEnvOverridesNil {
				if resolvedEnvOverrides != nil {
					t.Errorf("expected nil envOverrides, got %v", resolvedEnvOverrides)
				}
			} else {
				gotEnvOverrides := unmarshalRawExtension(t, resolvedEnvOverrides)
				if diff := cmp.Diff(tt.wantEnvOverrides, gotEnvOverrides); diff != "" {
					t.Errorf("envOverrides mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestBuildEmbeddedTraitContext(t *testing.T) {
	baseTrait := &v1alpha1.Trait{}
	baseTrait.Name = "storage"
	baseTrait.Spec.Schema.Parameters = &runtime.RawExtension{
		Raw: mustMarshalJSON(t, map[string]any{
			"mountPath": "string",
			"size":      "string | default=\"5Gi\"",
		}),
	}

	baseDataPlane := &v1alpha1.DataPlane{}
	baseDataPlane.Spec.Gateway.PublicVirtualHost = "api.example.com"

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
		name             string
		input            *EmbeddedTraitContextInput
		wantParams       map[string]any
		wantEnvOverrides map[string]any
		wantTraitName    string
		wantInstanceName string
		wantErr          bool
	}{
		{
			name: "basic embedded trait context with resolved parameters",
			input: &EmbeddedTraitContextInput{
				Trait: baseTrait,
				Instance: v1alpha1.ComponentTrait{
					Name:         "storage",
					InstanceName: "app-storage",
					Parameters: &runtime.RawExtension{
						Raw: mustMarshalJSON(t, map[string]any{
							"mountPath": "/var/data",
							"size":      "10Gi",
						}),
					},
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
			wantEnvOverrides: map[string]any{},
			wantTraitName:    "storage",
			wantInstanceName: "app-storage",
		},
		{
			name: "embedded trait with envOverrides merged from ReleaseBinding",
			input: &EmbeddedTraitContextInput{
				Trait: func() *v1alpha1.Trait {
					t := &v1alpha1.Trait{}
					t.Name = "logging"
					t.Spec.Schema.Parameters = &runtime.RawExtension{
						Raw: mustMarshalJSON(nil, map[string]any{
							"format": "string | default=\"json\"",
						}),
					}
					t.Spec.Schema.EnvOverrides = &runtime.RawExtension{
						Raw: mustMarshalJSON(nil, map[string]any{
							"logLevel": "string | default=\"info\"",
						}),
					}
					return t
				}(),
				Instance: v1alpha1.ComponentTrait{
					Name:         "logging",
					InstanceName: "app-logging",
				},
				ResolvedEnvOverrides: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"logLevel": "debug",
					}),
				},
				ReleaseBinding: &v1alpha1.ReleaseBinding{
					Spec: v1alpha1.ReleaseBindingSpec{
						TraitOverrides: map[string]runtime.RawExtension{
							"app-logging": {
								Raw: mustMarshalJSON(t, map[string]any{
									"logLevel": "warn",
								}),
							},
						},
					},
				},
				Component:   &v1alpha1.Component{},
				DataPlane:   baseDataPlane,
				Environment: baseEnvironment,
				Metadata:    baseMetadata,
			},
			wantParams: map[string]any{
				"format": "json",
			},
			wantEnvOverrides: map[string]any{
				"logLevel": "warn", // ReleaseBinding wins over resolved defaults
			},
			wantTraitName:    "logging",
			wantInstanceName: "app-logging",
		},
		{
			name: "embedded trait with resolved envOverrides defaults (no ReleaseBinding override)",
			input: &EmbeddedTraitContextInput{
				Trait: func() *v1alpha1.Trait {
					t := &v1alpha1.Trait{}
					t.Name = "logging"
					t.Spec.Schema.EnvOverrides = &runtime.RawExtension{
						Raw: mustMarshalJSON(nil, map[string]any{
							"logLevel": "string | default=\"info\"",
						}),
					}
					return t
				}(),
				Instance: v1alpha1.ComponentTrait{
					Name:         "logging",
					InstanceName: "app-logging",
				},
				ResolvedEnvOverrides: &runtime.RawExtension{
					Raw: mustMarshalJSON(t, map[string]any{
						"logLevel": "debug",
					}),
				},
				Component:   &v1alpha1.Component{},
				DataPlane:   baseDataPlane,
				Environment: baseEnvironment,
				Metadata:    baseMetadata,
			},
			wantParams: map[string]any{},
			wantEnvOverrides: map[string]any{
				"logLevel": "debug", // Resolved defaults used when no ReleaseBinding override
			},
			wantTraitName:    "logging",
			wantInstanceName: "app-logging",
		},
		{
			name: "missing instance name",
			input: &EmbeddedTraitContextInput{
				Trait: baseTrait,
				Instance: v1alpha1.ComponentTrait{
					Name: "storage",
				},
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

			// Verify envOverrides
			envOverrides, _ := ctxMap["envOverrides"].(map[string]any)
			if diff := cmp.Diff(tt.wantEnvOverrides, envOverrides); diff != "" {
				t.Errorf("envOverrides mismatch (-want +got):\n%s", diff)
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

// unmarshalRawExtension unmarshals a RawExtension into a map.
func unmarshalRawExtension(t *testing.T, raw *runtime.RawExtension) map[string]any {
	t.Helper()
	if raw == nil || raw.Raw == nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw.Raw, &result); err != nil {
		t.Fatalf("Failed to unmarshal RawExtension: %v", err)
	}
	return result
}
