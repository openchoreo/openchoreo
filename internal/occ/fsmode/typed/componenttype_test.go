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

func makeComponentTypeEntry(t *testing.T, ct *v1alpha1.ComponentType) *index.ResourceEntry {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ct)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ComponentType"))
	return &index.ResourceEntry{Resource: obj}
}

func TestNewComponentType(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: makeComponentTypeEntry(t, &v1alpha1.ComponentType{
				Spec: v1alpha1.ComponentTypeSpec{
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
			ct, err := NewComponentType(tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, ct)
		})
	}
}

func TestComponentTypeWorkloadType(t *testing.T) {
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
			ct := &ComponentType{
				ComponentType: &v1alpha1.ComponentType{
					Spec: v1alpha1.ComponentTypeSpec{
						WorkloadType: tt.workloadType,
					},
				},
			}
			assert.Equal(t, tt.want, ct.WorkloadType())
		})
	}
}

func TestComponentTypeGetSchema(t *testing.T) {
	schemaJSON := []byte(`{"type":"object","properties":{"port":{"type":"integer"}}}`)

	tests := []struct {
		name       string
		ct         *ComponentType
		wantParams bool
		wantEnv    bool
	}{
		{
			name: "parameters and env configs present",
			ct: &ComponentType{
				ComponentType: &v1alpha1.ComponentType{
					Spec: v1alpha1.ComponentTypeSpec{
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
			ct: &ComponentType{
				ComponentType: &v1alpha1.ComponentType{
					Spec: v1alpha1.ComponentTypeSpec{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.ct.GetSchema()
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

func TestComponentTypeGetResources(t *testing.T) {
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
				},
			},
			wantLen: 2,
		},
		{
			name:    "empty resources",
			wantNil: true,
		},
		{
			name: "resource with forEach and var",
			resources: []v1alpha1.ResourceTemplate{
				{
					ID:      "pvc",
					ForEach: "${parameters.volumes}",
					Var:     "volume",
				},
			},
			wantLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := &ComponentType{
				ComponentType: &v1alpha1.ComponentType{
					Spec: v1alpha1.ComponentTypeSpec{
						Resources: tt.resources,
					},
				},
			}
			result := ct.GetResources()
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.Len(t, result, tt.wantLen)
			first := result[0].(map[string]any)
			if tt.name == "resource with forEach and var" {
				assert.Equal(t, "pvc", first["id"])
				assert.Equal(t, "${parameters.volumes}", first["forEach"])
				assert.Equal(t, "volume", first["var"])
				return
			}
			assert.Equal(t, "deployment", first["id"])
			assert.Equal(t, "dataplane", first["targetPlane"])
			assert.Contains(t, first, "template")
		})
	}
}
