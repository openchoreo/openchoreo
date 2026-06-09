// Copyright 2025 The OpenChoreo Authors
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

func makeComponentEntry(t *testing.T, comp *v1alpha1.Component) *index.ResourceEntry {
	t.Helper()
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(comp)
	require.NoError(t, err)
	obj := &unstructured.Unstructured{Object: raw}
	obj.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("Component"))
	return &index.ResourceEntry{Resource: obj}
}

func TestNewComponent(t *testing.T) {
	tests := []struct {
		name    string
		entry   *index.ResourceEntry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: makeComponentEntry(t, &v1alpha1.Component{
				Spec: v1alpha1.ComponentSpec{
					Owner:         v1alpha1.ComponentOwner{ProjectName: "my-project"},
					ComponentType: v1alpha1.ComponentTypeRef{Name: "deployment/http-service"},
				},
			}),
		},
		{
			name:    "nil entry",
			entry:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := NewComponent(tt.entry)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, comp)
			assert.Equal(t, "my-project", comp.ProjectName())
		})
	}
}

func TestComponentProjectName(t *testing.T) {
	comp := &Component{
		Component: &v1alpha1.Component{
			Spec: v1alpha1.ComponentSpec{
				Owner: v1alpha1.ComponentOwner{ProjectName: "test-project"},
			},
		},
	}
	assert.Equal(t, "test-project", comp.ProjectName())
}

func TestComponentGetParameters(t *testing.T) {
	tests := []struct {
		name    string
		params  *runtime.RawExtension
		wantNil bool
		wantKey string
	}{
		{
			name:    "with parameters",
			params:  &runtime.RawExtension{Raw: []byte(`{"port":8080,"replicas":3}`)},
			wantKey: "port",
		},
		{
			name:    "nil parameters",
			params:  nil,
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &Component{
				Component: &v1alpha1.Component{
					Spec: v1alpha1.ComponentSpec{
						Parameters: tt.params,
					},
				},
			}
			result := comp.GetParameters()
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.Contains(t, result, tt.wantKey)
		})
	}
}

func TestComponentGetTraitRefs(t *testing.T) {
	tests := []struct {
		name     string
		traits   []v1alpha1.ComponentTrait
		wantNil  bool
		wantLen  int
		validate func(t *testing.T, refs []TraitRef)
	}{
		{
			name:    "no traits",
			traits:  nil,
			wantNil: true,
		},
		{
			name:    "empty traits",
			traits:  []v1alpha1.ComponentTrait{},
			wantNil: true,
		},
		{
			name: "single trait with explicit kind",
			traits: []v1alpha1.ComponentTrait{
				{
					Kind:         "ClusterTrait",
					Name:         "autoscaler",
					InstanceName: "my-autoscaler",
					Parameters:   &runtime.RawExtension{Raw: []byte(`{"minReplicas":2}`)},
				},
			},
			wantLen: 1,
			validate: func(t *testing.T, refs []TraitRef) {
				assert.Equal(t, "ClusterTrait", refs[0].Kind)
				assert.Equal(t, "autoscaler", refs[0].Name)
				assert.Equal(t, "my-autoscaler", refs[0].InstanceName)
				require.NotNil(t, refs[0].Parameters)
				assert.Equal(t, float64(2), refs[0].Parameters["minReplicas"])
			},
		},
		{
			name: "trait with empty kind defaults to Trait",
			traits: []v1alpha1.ComponentTrait{
				{
					Kind:         "",
					Name:         "logging",
					InstanceName: "my-logging",
				},
			},
			wantLen: 1,
			validate: func(t *testing.T, refs []TraitRef) {
				assert.Equal(t, "Trait", refs[0].Kind)
				assert.Equal(t, "logging", refs[0].Name)
				assert.Nil(t, refs[0].Parameters)
			},
		},
		{
			name: "multiple traits",
			traits: []v1alpha1.ComponentTrait{
				{Kind: "Trait", Name: "logging", InstanceName: "log1"},
				{Kind: "ClusterTrait", Name: "autoscaler", InstanceName: "as1"},
			},
			wantLen: 2,
			validate: func(t *testing.T, refs []TraitRef) {
				assert.Equal(t, "logging", refs[0].Name)
				assert.Equal(t, "autoscaler", refs[1].Name)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &Component{
				Component: &v1alpha1.Component{
					Spec: v1alpha1.ComponentSpec{
						Traits: tt.traits,
					},
				},
			}
			refs := comp.GetTraitRefs()
			if tt.wantNil {
				assert.Nil(t, refs)
				return
			}
			require.Len(t, refs, tt.wantLen)
			if tt.validate != nil {
				tt.validate(t, refs)
			}
		})
	}
}

func TestComponentTypeName(t *testing.T) {
	tests := []struct {
		name          string
		componentType v1alpha1.ComponentTypeRef
		want          string
	}{
		{
			name:          "Name with category extracts name after slash",
			componentType: v1alpha1.ComponentTypeRef{Name: "deployment/http-service"},
			want:          "http-service",
		},
		{
			name:          "Name without category returns full name",
			componentType: v1alpha1.ComponentTypeRef{Name: "http-service"},
			want:          "http-service",
		},
		{
			name:          "Empty name returns empty string",
			componentType: v1alpha1.ComponentTypeRef{Name: ""},
			want:          "",
		},
		{
			name:          "Multiple slashes extracts after last slash",
			componentType: v1alpha1.ComponentTypeRef{Name: "a/b/c"},
			want:          "c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &Component{
				Component: &v1alpha1.Component{
					Spec: v1alpha1.ComponentSpec{
						ComponentType: tt.componentType,
					},
				},
			}
			got := comp.ComponentTypeName()
			if got != tt.want {
				t.Errorf("ComponentTypeName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComponentTypeCategory(t *testing.T) {
	tests := []struct {
		name          string
		componentType v1alpha1.ComponentTypeRef
		want          string
	}{
		{
			name:          "Name with category extracts category before slash",
			componentType: v1alpha1.ComponentTypeRef{Name: "deployment/http-service"},
			want:          "deployment",
		},
		{
			name:          "Name without category returns empty string",
			componentType: v1alpha1.ComponentTypeRef{Name: "http-service"},
			want:          "",
		},
		{
			name:          "Empty name returns empty string",
			componentType: v1alpha1.ComponentTypeRef{Name: ""},
			want:          "",
		},
		{
			name:          "Multiple slashes extracts before first slash",
			componentType: v1alpha1.ComponentTypeRef{Name: "a/b/c"},
			want:          "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &Component{
				Component: &v1alpha1.Component{
					Spec: v1alpha1.ComponentSpec{
						ComponentType: tt.componentType,
					},
				},
			}
			got := comp.ComponentTypeCategory()
			if got != tt.want {
				t.Errorf("ComponentTypeCategory() = %q, want %q", got, tt.want)
			}
		})
	}
}
