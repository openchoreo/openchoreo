// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"testing"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

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
