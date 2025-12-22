// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceEntryName(t *testing.T) {
	tests := []struct {
		name     string
		entry    *ResourceEntry
		expected string
	}{
		{
			name: "normal resource",
			entry: &ResourceEntry{
				Resource: func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{}
					obj.SetName("test-resource")
					return obj
				}(),
			},
			expected: "test-resource",
		},
		{
			name: "nil resource",
			entry: &ResourceEntry{
				Resource: nil,
			},
			expected: "",
		},
		{
			name: "empty name",
			entry: &ResourceEntry{
				Resource: &unstructured.Unstructured{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.Name()
			if result != tt.expected {
				t.Errorf("Name() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResourceEntryNamespace(t *testing.T) {
	tests := []struct {
		name     string
		entry    *ResourceEntry
		expected string
	}{
		{
			name: "namespaced resource",
			entry: &ResourceEntry{
				Resource: func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{}
					obj.SetNamespace("my-namespace")
					return obj
				}(),
			},
			expected: "my-namespace",
		},
		{
			name: "cluster-scoped resource",
			entry: &ResourceEntry{
				Resource: func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{}
					obj.SetName("cluster-resource")
					return obj
				}(),
			},
			expected: "",
		},
		{
			name: "nil resource",
			entry: &ResourceEntry{
				Resource: nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.Namespace()
			if result != tt.expected {
				t.Errorf("Namespace() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResourceEntryNamespacedName(t *testing.T) {
	tests := []struct {
		name     string
		entry    *ResourceEntry
		expected string
	}{
		{
			name: "namespaced resource",
			entry: &ResourceEntry{
				Resource: func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{}
					obj.SetNamespace("my-namespace")
					obj.SetName("my-resource")
					return obj
				}(),
			},
			expected: "my-namespace/my-resource",
		},
		{
			name: "cluster-scoped resource",
			entry: &ResourceEntry{
				Resource: func() *unstructured.Unstructured {
					obj := &unstructured.Unstructured{}
					obj.SetName("cluster-resource")
					return obj
				}(),
			},
			expected: "cluster-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.NamespacedName()
			if result != tt.expected {
				t.Errorf("NamespacedName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResourceEntryGetNestedString(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "Component",
			"metadata": map[string]interface{}{
				"name": "test-component",
			},
			"spec": map[string]interface{}{
				"owner": map[string]interface{}{
					"projectName":   "my-project",
					"componentName": "my-component",
				},
				"componentType": "http-service",
			},
		},
	}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "openchoreo.dev",
		Version: "v1alpha1",
		Kind:    "Component",
	})

	entry := &ResourceEntry{
		Resource: obj,
		FilePath: "/test.yaml",
	}

	tests := []struct {
		name     string
		fields   []string
		expected string
	}{
		{"nested string", []string{"spec", "componentType"}, "http-service"},
		{"deeply nested", []string{"spec", "owner", "projectName"}, "my-project"},
		{"non-existent field", []string{"spec", "nonexistent"}, ""},
		{"wrong path", []string{"wrong", "path"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := entry.GetNestedString(tt.fields...)
			if result != tt.expected {
				t.Errorf("GetNestedString(%v) = %q, want %q", tt.fields, result, tt.expected)
			}
		})
	}

	// Test with nil resource
	nilEntry := &ResourceEntry{Resource: nil}
	if nilEntry.GetNestedString("any", "path") != "" {
		t.Error("GetNestedString should return empty string for nil resource")
	}
}

func TestResourceEntryGetNestedMap(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "Component",
			"spec": map[string]interface{}{
				"owner": map[string]interface{}{
					"projectName": "my-project",
				},
			},
		},
	}

	entry := &ResourceEntry{
		Resource: obj,
		FilePath: "/test.yaml",
	}

	// Get existing map
	ownerMap := entry.GetNestedMap("spec", "owner")
	if ownerMap == nil {
		t.Fatal("expected non-nil map for spec.owner")
	}
	if ownerMap["projectName"] != "my-project" {
		t.Errorf("expected projectName='my-project', got %v", ownerMap["projectName"])
	}

	// Get non-existent map
	nilMap := entry.GetNestedMap("spec", "nonexistent")
	if nilMap != nil {
		t.Error("expected nil for non-existent path")
	}

	// Test with nil resource
	nilEntry := &ResourceEntry{Resource: nil}
	if nilEntry.GetNestedMap("any", "path") != nil {
		t.Error("GetNestedMap should return nil for nil resource")
	}
}

func TestResourceEntryGetNestedSlice(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "openchoreo.dev/v1alpha1",
			"kind":       "Component",
			"spec": map[string]interface{}{
				"traits": []interface{}{
					map[string]interface{}{"name": "trait1"},
					map[string]interface{}{"name": "trait2"},
				},
			},
		},
	}

	entry := &ResourceEntry{
		Resource: obj,
		FilePath: "/test.yaml",
	}

	// Get existing slice
	traits := entry.GetNestedSlice("spec", "traits")
	if traits == nil {
		t.Fatal("expected non-nil slice for spec.traits")
	}
	if len(traits) != 2 {
		t.Errorf("expected 2 traits, got %d", len(traits))
	}

	// Get non-existent slice
	nilSlice := entry.GetNestedSlice("spec", "nonexistent")
	if nilSlice != nil {
		t.Error("expected nil for non-existent path")
	}

	// Test with nil resource
	nilEntry := &ResourceEntry{Resource: nil}
	if nilEntry.GetNestedSlice("any", "path") != nil {
		t.Error("GetNestedSlice should return nil for nil resource")
	}
}
