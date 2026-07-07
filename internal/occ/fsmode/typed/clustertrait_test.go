// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func makeClusterTraitEntry(t *testing.T, ct *v1alpha1.ClusterTrait) *index.ResourceEntry {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ct)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ClusterTrait"))
	return &index.ResourceEntry{Resource: obj}
}

func TestNewClusterTrait(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
	}{
		{
			name:  "valid entry",
			entry: makeClusterTraitEntry(t, &v1alpha1.ClusterTrait{}),
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trait, err := NewClusterTrait(tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, trait)
		})
	}
}

func TestClusterTraitGetSpec(t *testing.T) {
	schemaJSON := []byte(`{"type":"object"}`)
	templateJSON := []byte(`{"apiVersion":"v1","kind":"ConfigMap"}`)

	tests := []struct {
		name        string
		trait       *ClusterTrait
		wantParams  bool
		wantEnv     bool
		wantCreates bool
		wantPatches bool
	}{
		{
			name: "parameters present",
			trait: &ClusterTrait{
				ClusterTrait: &v1alpha1.ClusterTrait{
					Spec: v1alpha1.ClusterTraitSpec{
						Parameters: &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: schemaJSON}},
					},
				},
			},
			wantParams: true,
		},
		{
			name: "environmentConfigs present",
			trait: &ClusterTrait{
				ClusterTrait: &v1alpha1.ClusterTrait{
					Spec: v1alpha1.ClusterTraitSpec{
						EnvironmentConfigs: &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: schemaJSON}},
					},
				},
			},
			wantEnv: true,
		},
		{
			name: "creates present",
			trait: &ClusterTrait{
				ClusterTrait: &v1alpha1.ClusterTrait{
					Spec: v1alpha1.ClusterTraitSpec{
						Creates: []v1alpha1.TraitCreate{
							{
								TargetPlane: "dataplane",
								Template:    &runtime.RawExtension{Raw: templateJSON},
							},
						},
					},
				},
			},
			wantCreates: true,
		},
		{
			name: "patches present",
			trait: &ClusterTrait{
				ClusterTrait: &v1alpha1.ClusterTrait{
					Spec: v1alpha1.ClusterTraitSpec{
						Patches: []v1alpha1.TraitPatch{
							{
								ForEach:     "${spec.endpoints}",
								Var:         "ep",
								TargetPlane: "dataplane",
								Target: v1alpha1.PatchTarget{
									Group:   "apps",
									Version: "v1",
									Kind:    "Deployment",
									Where:   "${metadata.name == 'my-deploy'}",
								},
								Operations: []v1alpha1.JSONPatchOperation{
									{
										Op:    "add",
										Path:  "/spec/replicas",
										Value: &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)},
									},
								},
							},
						},
					},
				},
			},
			wantPatches: true,
		},
		{
			name: "no schemas",
			trait: &ClusterTrait{
				ClusterTrait: &v1alpha1.ClusterTrait{
					Spec: v1alpha1.ClusterTraitSpec{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.trait.GetSpec()
			require.NotNil(t, spec)
			if tt.wantParams {
				require.Contains(t, spec, "parameters", "spec should contain 'parameters' key")
				paramsMap, ok := spec["parameters"].(map[string]interface{})
				require.True(t, ok, "spec[\"parameters\"] should be a map[string]interface{}")
				require.Contains(t, paramsMap, "openAPIV3Schema", "spec[\"parameters\"] should contain 'openAPIV3Schema' key")
				schema, ok := paramsMap["openAPIV3Schema"].(map[string]interface{})
				require.True(t, ok, "spec[\"parameters\"][\"openAPIV3Schema\"] should be a map[string]interface{}")
				assert.Equal(t, "object", schema["type"], "spec[\"parameters\"][\"openAPIV3Schema\"][\"type\"] should match input schema")
			} else {
				assert.NotContains(t, spec, "parameters")
			}
			if tt.wantEnv {
				require.Contains(t, spec, "environmentConfigs", "spec should contain 'environmentConfigs' key")
				envMap, ok := spec["environmentConfigs"].(map[string]interface{})
				require.True(t, ok, "spec[\"environmentConfigs\"] should be a map[string]interface{}")
				require.Contains(t, envMap, "openAPIV3Schema", "spec[\"environmentConfigs\"] should contain 'openAPIV3Schema' key")
				schema, ok := envMap["openAPIV3Schema"].(map[string]interface{})
				require.True(t, ok, "spec[\"environmentConfigs\"][\"openAPIV3Schema\"] should be a map[string]interface{}")
				assert.Equal(t, "object", schema["type"], "spec[\"environmentConfigs\"][\"openAPIV3Schema\"][\"type\"] should match input schema")
			} else {
				assert.NotContains(t, spec, "environmentConfigs")
			}
			if tt.wantCreates {
				assert.Contains(t, spec, "creates")
				creates := spec["creates"].([]interface{})
				require.Len(t, creates, 1)
				c := creates[0].(map[string]interface{})
				assert.Equal(t, "dataplane", c["targetPlane"])
				assert.NotNil(t, c["template"])
			} else {
				assert.NotContains(t, spec, "creates")
			}
			if tt.wantPatches {
				assert.Contains(t, spec, "patches")
				patches := spec["patches"].([]interface{})
				require.Len(t, patches, 1)
				p := patches[0].(map[string]interface{})
				assert.Equal(t, "${spec.endpoints}", p["forEach"])
				assert.Equal(t, "ep", p["var"])
				assert.Equal(t, "dataplane", p["targetPlane"])

				target := p["target"].(map[string]interface{})
				assert.Equal(t, "apps", target["group"])
				assert.Equal(t, "v1", target["version"])
				assert.Equal(t, "Deployment", target["kind"])
				assert.Equal(t, "${metadata.name == 'my-deploy'}", target["where"])

				ops := p["operations"].([]interface{})
				require.Len(t, ops, 1)
				op := ops[0].(map[string]interface{})
				assert.Equal(t, "add", op["op"])
				assert.Equal(t, "/spec/replicas", op["path"])
				assert.NotNil(t, op["value"])
			} else {
				assert.NotContains(t, spec, "patches")
			}
		})
	}
}

// TestClusterTraitGetSpecValidationsAndRemoves verifies that GetSpec carries the cluster
// trait's validation rules (deprecated validations, preRenderValidations,
// postRenderValidations) and removes into the emitted spec, mirroring the namespace-scoped
// Trait. Dropping any of them silently disables the corresponding trait behavior in the
// generated ComponentRelease.
func TestClusterTraitGetSpecValidationsAndRemoves(t *testing.T) {
	mustMatch := false
	ct := &ClusterTrait{
		ClusterTrait: &v1alpha1.ClusterTrait{
			Spec: v1alpha1.ClusterTraitSpec{
				Validations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.a > 0}", Message: "a must be positive"},
				},
				PreRenderValidations: []v1alpha1.ValidationRule{
					{Rule: "${parameters.b != ''}", Message: "b is required"},
				},
				PostRenderValidations: []v1alpha1.PostRenderValidation{
					{
						When:    "${parameters.enabled}",
						ForEach: "${parameters.items}",
						Var:     "item",
						Target: v1alpha1.PostRenderTarget{
							PatchTarget: v1alpha1.PatchTarget{
								Group:   "apps",
								Version: "v1",
								Kind:    "Deployment",
								Where:   "${resource.metadata.name == item}",
							},
							MustMatch: &mustMatch,
						},
						TargetPlane: "dataplane",
						Rule:        "${resource.spec.replicas > 0}",
						Message:     "replicas must be positive",
					},
				},
				Removes: []v1alpha1.TraitRemove{
					{
						ForEach:     "${parameters.routesToDrop}",
						Var:         "route",
						TargetPlane: "dataplane",
						Target: v1alpha1.PatchTarget{
							Group:   "",
							Version: "v1",
							Kind:    "Service",
							Where:   "${resource.metadata.name == route}",
						},
					},
				},
			},
		},
	}

	spec := ct.GetSpec()
	require.NotNil(t, spec)

	raw, err := json.Marshal(spec)
	require.NoError(t, err)
	var decoded v1alpha1.ClusterTraitSpec
	require.NoError(t, json.Unmarshal(raw, &decoded))

	require.Len(t, decoded.Validations, 1)
	assert.Equal(t, "${parameters.a > 0}", decoded.Validations[0].Rule)

	require.Len(t, decoded.PreRenderValidations, 1)
	assert.Equal(t, "${parameters.b != ''}", decoded.PreRenderValidations[0].Rule)

	require.Len(t, decoded.PostRenderValidations, 1)
	prv := decoded.PostRenderValidations[0]
	assert.Equal(t, "${parameters.enabled}", prv.When)
	assert.Equal(t, "${parameters.items}", prv.ForEach)
	assert.Equal(t, "item", prv.Var)
	assert.Equal(t, "Deployment", prv.Target.Kind)
	assert.Equal(t, "${resource.metadata.name == item}", prv.Target.Where)
	require.NotNil(t, prv.Target.MustMatch)
	assert.False(t, *prv.Target.MustMatch)
	assert.Equal(t, "dataplane", prv.TargetPlane)
	assert.Equal(t, "${resource.spec.replicas > 0}", prv.Rule)
	assert.Equal(t, "replicas must be positive", prv.Message)

	require.Len(t, decoded.Removes, 1)
	rm := decoded.Removes[0]
	assert.Equal(t, "${parameters.routesToDrop}", rm.ForEach)
	assert.Equal(t, "route", rm.Var)
	assert.Equal(t, "Service", rm.Target.Kind)
}

// TestClusterTraitGetSpecOmitsEmptyValidationsAndRemoves verifies validation and removes
// keys are absent when the cluster trait defines none.
func TestClusterTraitGetSpecOmitsEmptyValidationsAndRemoves(t *testing.T) {
	ct := &ClusterTrait{ClusterTrait: &v1alpha1.ClusterTrait{Spec: v1alpha1.ClusterTraitSpec{}}}
	spec := ct.GetSpec()
	require.NotNil(t, spec)
	assert.NotContains(t, spec, "validations")
	assert.NotContains(t, spec, "preRenderValidations")
	assert.NotContains(t, spec, "postRenderValidations")
	assert.NotContains(t, spec, "removes")
}
