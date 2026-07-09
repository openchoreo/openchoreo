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
