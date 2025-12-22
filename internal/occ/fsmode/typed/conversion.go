// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// FromEntry converts a ResourceEntry to a typed v1alpha1 object using runtime.DefaultUnstructuredConverter
func FromEntry[T any](entry *index.ResourceEntry) (*T, error) {
	if entry == nil {
		return nil, fmt.Errorf("resource entry is nil")
	}
	if entry.Resource == nil {
		return nil, fmt.Errorf("resource is nil")
	}

	var obj T
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(entry.Resource.Object, &obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to typed object: %w", err)
	}

	return &obj, nil
}

// rawExtensionToMap converts a runtime.RawExtension to map[string]interface{} for template processing
func rawExtensionToMap(raw *runtime.RawExtension) map[string]interface{} {
	if raw == nil || raw.Raw == nil {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &result); err != nil {
		return nil
	}

	return result
}

// TraitRef is a convenience type for trait references
// This mirrors the v1alpha1.ComponentTrait structure but uses map for Parameters
type TraitRef struct {
	Name         string
	InstanceName string
	Parameters   map[string]interface{}
}
