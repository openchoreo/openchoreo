// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SerializableIndex is the JSON-friendly version of Index
type SerializableIndex struct {
	Resources []SerializableResource `json:"resources"`
	RepoPath  string                 `json:"repoPath"`
	CommitSHA string                 `json:"commitSHA"`
}

// SerializableResource is a JSON-friendly representation of a resource entry
type SerializableResource struct {
	GVK       string                 `json:"gvk"` // "openchoreo.dev/v1alpha1/Component"
	Namespace string                 `json:"namespace"`
	Name      string                 `json:"name"`
	FilePath  string                 `json:"filePath"`
	Object    map[string]interface{} `json:"object"` // Full resource
}

// ToSerializable converts the Index to a serializable format
func (idx *Index) ToSerializable() *SerializableIndex {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var resources []SerializableResource

	// Iterate through all resources in the index
	for gvk, gvkMap := range idx.byGVK {
		for _, entry := range gvkMap {
			sr := SerializableResource{
				GVK:       gvkToString(gvk),
				Namespace: entry.Namespace(),
				Name:      entry.Name(),
				FilePath:  entry.FilePath,
				Object:    entry.Resource.Object,
			}

			resources = append(resources, sr)
		}
	}

	return &SerializableIndex{
		Resources: resources,
		RepoPath:  idx.repoPath,
		CommitSHA: idx.commitSHA,
	}
}

// ToIndex converts a SerializableIndex back to an Index
func (si *SerializableIndex) ToIndex(repoPath string) *Index {
	idx := New(repoPath)
	idx.commitSHA = si.CommitSHA

	for _, sr := range si.Resources {
		// Parse GVK
		gvk, err := stringToGVK(sr.GVK)
		if err != nil {
			continue
		}

		// Reconstruct the unstructured resource
		obj := &unstructured.Unstructured{
			Object: sr.Object,
		}
		obj.SetGroupVersionKind(gvk)

		// Create resource entry
		entry := &ResourceEntry{
			Resource: obj,
			FilePath: sr.FilePath,
		}

		// Add to index (ignore errors during deserialization)
		_ = idx.Add(entry)
	}

	return idx
}

// gvkToString converts a GVK to a string representation
func gvkToString(gvk schema.GroupVersionKind) string {
	return gvk.Group + "/" + gvk.Version + "/" + gvk.Kind
}

// stringToGVK converts a string back to a GVK
func stringToGVK(s string) (schema.GroupVersionKind, error) {
	gv, err := schema.ParseGroupVersion(s[:len(s)-len("/"+kindFromString(s))])
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return gv.WithKind(kindFromString(s)), nil
}

// kindFromString extracts the kind from a GVK string
func kindFromString(s string) string {
	// Find last slash
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return s[i+1:]
		}
	}
	return s
}
