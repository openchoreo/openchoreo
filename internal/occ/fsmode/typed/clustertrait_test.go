// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
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
				assert.Contains(t, spec, "parameters")
			} else {
				assert.NotContains(t, spec, "parameters")
			}
			if tt.wantEnv {
				assert.Contains(t, spec, "environmentConfigs")
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
