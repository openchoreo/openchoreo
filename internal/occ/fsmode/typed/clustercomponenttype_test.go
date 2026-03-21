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

func makeClusterComponentTypeEntry(t *testing.T, cct *v1alpha1.ClusterComponentType) *index.ResourceEntry {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cct)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ClusterComponentType"))
	return &index.ResourceEntry{Resource: obj}
}

func TestNewClusterComponentType(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: makeClusterComponentTypeEntry(t, &v1alpha1.ClusterComponentType{
				Spec: v1alpha1.ClusterComponentTypeSpec{
					WorkloadType: "deployment",
				},
			}),
		},
		{
			name:    "nil resource entry",
			entry:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cct, err := NewClusterComponentType(tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cct)
		})
	}
}

func TestClusterComponentTypeWorkloadType(t *testing.T) {
	tests := []struct {
		name         string
		workloadType string
		want         string
	}{
		{
			name:         "present",
			workloadType: "deployment",
			want:         "deployment",
		},
		{
			name:         "empty",
			workloadType: "",
			want:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cct := &ClusterComponentType{
				ClusterComponentType: &v1alpha1.ClusterComponentType{
					Spec: v1alpha1.ClusterComponentTypeSpec{
						WorkloadType: tt.workloadType,
					},
				},
			}
			assert.Equal(t, tt.want, cct.WorkloadType())
		})
	}
}

func TestClusterComponentTypeGetSchema(t *testing.T) {
	schemaJSON := []byte(`{"type":"object","properties":{"port":{"type":"integer"}}}`)

	tests := []struct {
		name       string
		cct        *ClusterComponentType
		wantParams bool
		wantEnv    bool
	}{
		{
			name: "parameters and env configs present",
			cct: &ClusterComponentType{
				ClusterComponentType: &v1alpha1.ClusterComponentType{
					Spec: v1alpha1.ClusterComponentTypeSpec{
						Parameters:         &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: schemaJSON}},
						EnvironmentConfigs: &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: schemaJSON}},
					},
				},
			},
			wantParams: true,
			wantEnv:    true,
		},
		{
			name: "no schemas",
			cct: &ClusterComponentType{
				ClusterComponentType: &v1alpha1.ClusterComponentType{
					Spec: v1alpha1.ClusterComponentTypeSpec{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.cct.GetSchema()
			require.NotNil(t, schema)
			if tt.wantParams {
				assert.Contains(t, schema, "parameters")
				params := schema["parameters"].(map[string]any)
				assert.Contains(t, params, "openAPIV3Schema")
			} else {
				assert.NotContains(t, schema, "parameters")
			}
			if tt.wantEnv {
				assert.Contains(t, schema, "environmentConfigs")
			} else {
				assert.NotContains(t, schema, "environmentConfigs")
			}
		})
	}
}

func TestClusterComponentTypeGetResources(t *testing.T) {
	templateJSON, _ := json.Marshal(map[string]any{"kind": "Deployment"})

	tests := []struct {
		name      string
		resources []v1alpha1.ResourceTemplate
		wantLen   int
		wantNil   bool
	}{
		{
			name: "resources present",
			resources: []v1alpha1.ResourceTemplate{
				{
					ID:          "deployment",
					TargetPlane: "dataplane",
					Template:    &runtime.RawExtension{Raw: templateJSON},
				},
				{
					ID:          "service",
					IncludeWhen: "${spec.autoscaling.enabled}",
					ForEach:     "${spec.endpoints}",
					Var:         "ep",
				},
			},
			wantLen: 2,
		},
		{
			name:    "empty resources",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cct := &ClusterComponentType{
				ClusterComponentType: &v1alpha1.ClusterComponentType{
					Spec: v1alpha1.ClusterComponentTypeSpec{
						Resources: tt.resources,
					},
				},
			}
			result := cct.GetResources()
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.Len(t, result, tt.wantLen)
			first := result[0].(map[string]any)
			assert.Equal(t, "deployment", first["id"])
			assert.Equal(t, "dataplane", first["targetPlane"])
			assert.Contains(t, first, "template")

			second := result[1].(map[string]any)
			assert.Equal(t, "service", second["id"])
			assert.Equal(t, "${spec.autoscaling.enabled}", second["includeWhen"])
			assert.Equal(t, "${spec.endpoints}", second["forEach"])
			assert.Equal(t, "ep", second["var"])
		})
	}
}
