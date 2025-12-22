// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceEntry holds a resource with its source file path
type ResourceEntry struct {
	Resource *unstructured.Unstructured
	FilePath string
}

// Name returns the resource name
func (r *ResourceEntry) Name() string {
	if r.Resource == nil {
		return ""
	}
	return r.Resource.GetName()
}

// Namespace returns the resource namespace
func (r *ResourceEntry) Namespace() string {
	if r.Resource == nil {
		return ""
	}
	return r.Resource.GetNamespace()
}

// NamespacedName returns the namespaced name in the format "namespace/name"
func (r *ResourceEntry) NamespacedName() string {
	ns := r.Namespace()
	if ns == "" {
		return r.Name()
	}
	return ns + "/" + r.Name()
}

// GetNestedString safely extracts a nested string value from the resource
func (r *ResourceEntry) GetNestedString(fields ...string) string {
	if r.Resource == nil {
		return ""
	}
	val, found, err := unstructured.NestedString(r.Resource.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return val
}

// GetNestedMap safely extracts a nested map from the resource
func (r *ResourceEntry) GetNestedMap(fields ...string) map[string]interface{} {
	if r.Resource == nil {
		return nil
	}
	val, found, err := unstructured.NestedMap(r.Resource.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return val
}

// GetNestedSlice safely extracts a nested slice from the resource
func (r *ResourceEntry) GetNestedSlice(fields ...string) []interface{} {
	if r.Resource == nil {
		return nil
	}
	val, found, err := unstructured.NestedSlice(r.Resource.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return val
}
