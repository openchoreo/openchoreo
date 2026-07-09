// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
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
